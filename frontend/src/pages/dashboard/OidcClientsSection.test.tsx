import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import OidcClientsSection from "./OidcClientsSection";
import { api, ApiError } from "@/lib/api";
import { toast } from "sonner";
import { renderToString } from "react-dom/server";

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: any) => {
      if (options?.name) return `${key}:${options.name}`;
      return key;
    },
  }),
}));

// Mock MUI X DataGrid - it renders rows via virtualization so we bypass it
vi.mock("@mui/x-data-grid", () => ({
  DataGrid: (props: any) => {
    // Render a simplified table with all rows so we can test content
    return React.createElement(
      "div",
      { "data-testid": "datagrid" },
      props.rows?.map((row: any) =>
        React.createElement(
          "div",
          { key: row.id, "data-testid": "datagrid-row" },
          JSON.stringify(row)
        )
      )
    );
  },
  GridToolbar: () => React.createElement("div", null),
}));

const capturedSubmits: any[] = [];
const capturedButtonClicks: any[] = [];
const capturedTextFieldChanges: any[] = [];

let stateStore: any = {};
let stateSetters: any = {};
let callIdx = 0;

vi.mock("react", async (importOriginal) => {
  const original = await importOriginal<typeof import("react")>();
  return {
    ...original,
    useState: (init: any) => {
      const idx = callIdx;
      callIdx++;
      if (!(idx in stateStore)) {
        stateStore[idx] = init;
      }
      stateSetters[idx] = (newVal: any) => {
        if (typeof newVal === "function") {
          stateStore[idx] = newVal(stateStore[idx]);
        } else {
          stateStore[idx] = newVal;
        }
      };
      if (callIdx >= 13) {
        callIdx = 0;
      }
      return [stateStore[idx], stateSetters[idx]];
    },
    useEffect: (fn: any) => {
      fn();
    },
  };
});

vi.mock("@mui/material/Box", () => ({
  default: (props: any) => {
    if (props.component === "form" && props.onSubmit) {
      capturedSubmits.push(props.onSubmit);
    }
    return React.createElement("div", null, props.children);
  }
}));

vi.mock("@mui/material/Button", () => ({
  default: (props: any) => {
    if (props.onClick) capturedButtonClicks.push(props.onClick);
    return React.createElement("button", { onClick: props.onClick }, props.children);
  }
}));

vi.mock("@mui/material/IconButton", () => ({
  default: (props: any) => {
    if (props.onClick) capturedButtonClicks.push(props.onClick);
    return React.createElement("button", { onClick: props.onClick }, props.children);
  }
}));

vi.mock("@mui/material/TextField", () => ({
  default: (props: any) => {
    if (props.onChange) capturedTextFieldChanges.push(props.onChange);
    return React.createElement("input", { onChange: props.onChange, value: props.value });
  }
}));

vi.mock("@mui/material/Tooltip", () => ({
  default: (props: any) => React.createElement("span", null, props.children)
}));

vi.mock("@mui/material/Dialog", () => ({
  default: (props: any) => props.open ? React.createElement("div", null, props.children) : null
}));
vi.mock("@mui/material/DialogTitle", () => ({
  default: (props: any) => React.createElement("div", null, props.children)
}));
vi.mock("@mui/material/DialogContent", () => ({
  default: (props: any) => React.createElement("div", null, props.children)
}));
vi.mock("@mui/material/DialogActions", () => ({
  default: (props: any) => React.createElement("div", null, props.children)
}));

describe("OidcClientsSection", () => {
  const confirmMock = vi.fn().mockReturnValue(true);
  const writeTextMock = vi.fn();

  beforeEach(() => {
    stateStore = {};
    stateSetters = {};
    callIdx = 0;
    capturedSubmits.length = 0;
    capturedButtonClicks.length = 0;
    capturedTextFieldChanges.length = 0;
    confirmMock.mockClear();
    writeTextMock.mockClear();
    vi.mocked(toast.error).mockClear();
    vi.mocked(toast.success).mockClear();

    vi.stubGlobal("window", { confirm: confirmMock, prompt: vi.fn() });
    vi.stubGlobal("navigator", { clipboard: { writeText: writeTextMock } });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  const runRender = () => {
    callIdx = 0;
    return renderToString(React.createElement(OidcClientsSection));
  };

  it("renders loader during loading state", () => {
    stateStore[1] = true;
    const html = runRender();
    expect(html).toContain("CircularProgress");
    expect(html).toContain("loading");
  });

  it("renders DataGrid when clients are loaded", async () => {
    const listSpy = vi.spyOn(api.admin, "listOidcClients").mockResolvedValue([]);
    stateStore[1] = false;
    stateStore[0] = [
      {
        id: "c1",
        client_id: "test-client-id",
        name: "Test Client",
        redirect_uris: ["http://localhost/callback"],
        allowed_scopes: ["openid", "profile"],
        created_at: "2026-06-06T12:00:00Z",
      }
    ];

    const html = runRender();
    expect(html).toContain("test-client-id");
    expect(html).toContain("Test Client");
    expect(listSpy).toHaveBeenCalled();
  });

  it("handles client edit update flow", async () => {
    stateStore[1] = false;
    stateStore[8] = {
      id: "c1",
      client_id: "test-client-id",
      name: "Old Client Name",
      redirect_uris: ["http://old/callback"],
      allowed_scopes: ["openid"],
      created_at: "2026-06-06T12:00:00Z",
    };
    stateStore[9] = "Updated Client Name";
    stateStore[10] = "http://new/callback";
    stateStore[11] = "openid profile";

    const updateSpy = vi.spyOn(api.admin, "updateOidcClient").mockResolvedValue({} as any);

    runRender();

    expect(capturedSubmits[0]).toBeDefined();
    await capturedSubmits[0]({ preventDefault: vi.fn() });

    expect(updateSpy).toHaveBeenCalledWith("c1", {
      name: "Updated Client Name",
      redirect_uris: ["http://new/callback"],
      allowed_scopes: ["openid", "profile"],
    });
    expect(toast.success).toHaveBeenCalledWith("saved");
  });

  it("handles create client form submission", async () => {
    stateStore[1] = false;
    stateStore[2] = true;
    stateStore[3] = "New App";
    stateStore[4] = "new-app-id";
    stateStore[5] = "http://localhost/callback";
    stateStore[6] = "openid profile";

    const createSpy = vi.spyOn(api.admin, "createOidcClient").mockResolvedValue({
      id: "new-c",
      client_id: "new-app-id",
      client_secret: "generated-secret",
      name: "New App",
      redirect_uris: ["http://localhost/callback"],
      allowed_scopes: ["openid", "profile"],
      created_at: "2026-06-06T12:00:00Z",
    });

    runRender();

    expect(capturedSubmits[0]).toBeDefined();
    await capturedSubmits[0]({ preventDefault: vi.fn() });

    expect(createSpy).toHaveBeenCalledWith({
      client_id: "new-app-id",
      name: "New App",
      redirect_uris: ["http://localhost/callback"],
      allowed_scopes: ["openid", "profile"],
    });
  });

  it("shows rotated secret in the existing secret dialog", () => {
    stateStore[1] = false;
    // Pre-populate createdClient (idx 12) as handleRotate would after a successful rotation
    stateStore[12] = {
      id: "c1",
      client_id: "rotate-client",
      client_secret: "new-rotated-secret-xyz",
      name: "Rotate Test",
      redirect_uris: [],
      allowed_scopes: ["openid"],
      created_at: "2026-06-06T12:00:00Z",
    };

    const html = runRender();
    expect(html).toContain("new-rotated-secret-xyz");
    expect(html).toContain("rotate-client");
  });

  it("handles copy to clipboard", () => {
    stateStore[1] = false;
    stateStore[12] = {
      id: "c1",
      client_id: "copied-client-id",
      client_secret: "copied-secret",
      name: "Test",
      redirect_uris: [],
      allowed_scopes: [],
      created_at: "2026-06-06T12:00:00Z",
    };

    runRender();

    const copyBtn = capturedButtonClicks.find((fn) => {
      writeTextMock.mockClear();
      try { fn(); } catch { return false; }
      return writeTextMock.mock.calls.some((c) => c[0] === "copied-client-id");
    });
    expect(copyBtn).toBeDefined();
    expect(toast.success).toHaveBeenCalled();
  });
});
