import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import ServiceAccountsPage from "./ServiceAccountsPage";
import { api, ApiError } from "@/lib/api";
import { useMeContext } from "@/contexts/MeContext";
import { renderToString } from "react-dom/server";

// State Mocking System
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
      if (callIdx >= 60) {
        callIdx = 0;
      }
      return [stateStore[idx], stateSetters[idx]];
    },
    useEffect: (fn: any) => {
      fn();
    },
  };
});

vi.mock("@/contexts/MeContext", () => ({
  useMeContext: vi.fn(),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: "en" },
  }),
}));

const capturedButtonClicks: any[] = [];
const capturedChipClicks: any[] = [];
const capturedIconButtonClicks: any[] = [];
const capturedSubmits: any[] = [];
const capturedInputs: any[] = [];
const capturedSwitches: any[] = [];

vi.mock("@mui/material/Button", () => ({
  default: (props: any) => {
    if (props.onClick) capturedButtonClicks.push(props.onClick);
    return React.createElement("button", { onClick: props.onClick, type: props.type, disabled: props.disabled }, props.children);
  }
}));

vi.mock("@mui/material/Chip", () => ({
  default: (props: any) => {
    if (props.onClick) capturedChipClicks.push(props.onClick);
    return React.createElement("div", { onClick: props.onClick }, props.children);
  }
}));

vi.mock("@mui/material/IconButton", () => ({
  default: (props: any) => {
    if (props.onClick) capturedIconButtonClicks.push(props.onClick);
    return React.createElement("button", { onClick: props.onClick }, props.children);
  }
}));

vi.mock("@mui/material/Box", () => ({
  default: (props: any) => {
    if (props.onSubmit) capturedSubmits.push(props.onSubmit);
    return React.createElement("div", { onSubmit: props.onSubmit }, props.children);
  }
}));

vi.mock("@mui/material/TextField", () => ({
  default: (props: any) => {
    if (props.onChange) capturedInputs.push(props.onChange);
    return React.createElement("input", { type: props.type, value: props.value, onChange: props.onChange });
  }
}));

vi.mock("@mui/material/Switch", () => ({
  default: (props: any) => {
    if (props.onChange) capturedSwitches.push(props.onChange);
    return React.createElement("input", { type: "checkbox", checked: props.checked, onChange: props.onChange });
  }
}));

vi.mock("@mui/material/Dialog", () => ({
  default: (props: any) => props.open ? React.createElement("div", null, props.children) : null
}));

vi.mock("@mui/material/Tooltip", () => ({
  default: (props: any) => props.children
}));

vi.mock("@mui/x-data-grid", () => ({
  DataGrid: (props: any) => {
    const renderedRows = (props.rows ?? []).map((row: any) => {
      return React.createElement("div", { key: row.id, className: "mock-row" },
        (props.columns ?? []).map((col: any) => {
          const cellContent = col.renderCell ? col.renderCell({ row }) : row[col.field];
          return React.createElement("div", { key: col.field, className: "mock-cell" }, cellContent);
        })
      );
    });
    return React.createElement("div", { className: "mock-datagrid" }, renderedRows);
  },
}));

describe("ServiceAccountsPage page component", () => {
  let confirmMock = vi.fn().mockReturnValue(true);
  let promptMock = vi.fn().mockReturnValue("123456");
  let alertMock = vi.fn();

  beforeEach(() => {
    stateStore = {};
    stateSetters = {};
    callIdx = 0;
    capturedButtonClicks.length = 0;
    capturedChipClicks.length = 0;
    capturedIconButtonClicks.length = 0;
    capturedSubmits.length = 0;
    capturedInputs.length = 0;
    capturedSwitches.length = 0;
    confirmMock = vi.fn().mockReturnValue(true);
    promptMock = vi.fn().mockReturnValue("123456");
    alertMock = vi.fn();

    vi.stubGlobal("document", {
      visibilityState: "visible",
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    });
    vi.stubGlobal("window", {
      setInterval: vi.fn((fn: any, delay: number) => {
        fn();
        return 123;
      }),
      clearInterval: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      confirm: confirmMock,
      prompt: promptMock,
      alert: alertMock,
    });

    vi.stubGlobal("navigator", {
      clipboard: {
        writeText: vi.fn().mockResolvedValue({} as any),
      },
    });

    (global as any).confirm = confirmMock;
    (global as any).prompt = promptMock;
    (global as any).alert = alertMock;

    class MockEventSource {
      close = vi.fn();
      addEventListener = vi.fn();
      removeEventListener = vi.fn();
    }
    vi.stubGlobal("EventSource", MockEventSource);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    delete (global as any).confirm;
    delete (global as any).prompt;
    delete (global as any).alert;
  });

  const runRender = () => {
    callIdx = 0;
    capturedButtonClicks.length = 0;
    capturedChipClicks.length = 0;
    capturedIconButtonClicks.length = 0;
    capturedSubmits.length = 0;
    capturedInputs.length = 0;
    capturedSwitches.length = 0;
    return renderToString(React.createElement(ServiceAccountsPage));
  };

  const getMockAccounts = () => ({
    data: [
      {
        id: "sa1",
        name: "Service 1",
        client_id: "client-1",
        is_active: true,
        scopes: ["users:read"],
        created_at: "2026-06-04T12:00:00Z",
        expires_at: "2026-06-10T12:00:00Z",
      },
      {
        id: "sa2",
        name: "Service 2",
        client_id: "client-2",
        is_active: false,
        scopes: [],
        created_at: "2026-06-04T10:00:00Z",
        expires_at: "",
      }
    ],
    total: 2,
  });

  it("renders loader screen then loads service accounts lists", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    const listSpy = vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();

    const html = runRender();
    expect(html).toContain("Service 1");
    expect(listSpy).toHaveBeenCalled();
  });

  it("handles creating a service account successfully", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const createSpy = vi.spyOn(api.admin, "createServiceAccount").mockResolvedValue({
      client_id: "client-new",
      client_secret: "secret-new",
    } as any);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    // Trigger openCreate (index 0 button is create button)
    expect(capturedButtonClicks[0]).toBeDefined();
    capturedButtonClicks[0]();
    runRender();

    // Inputs:
    // [0] -> name
    // [1] -> expiresAt
    expect(capturedInputs[0]).toBeDefined();
    capturedInputs[0]({ target: { value: "New SA" } });
    capturedInputs[1]({ target: { value: "2026-12-31" } });

    // Scopes chips: PERMISSION_GROUPS has users, serviceAccounts, audit scopes.
    // Index 0 and 1 are row status chips. Index 2 is users:read.
    expect(capturedChipClicks[2]).toBeDefined();
    capturedChipClicks[2](); // toggle users:read

    runRender();
    await capturedSubmits[0]({ preventDefault: vi.fn() });

    expect(createSpy).toHaveBeenCalledWith({
      name: "New SA",
      scopes: ["users:read"],
      expires_at: "2026-12-31",
    });
  });

  it("handles toggling status with MFA challenge", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const stepUpSpy = vi.spyOn(api, "mfaStepUp").mockResolvedValue({} as any);
    const statusSpy = vi.spyOn(api.admin, "setServiceAccountStatus")
      .mockRejectedValueOnce(new ApiError("mfa_required"))
      .mockResolvedValueOnce({} as any);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    // Chip click for Status is in column row cells:
    // Alice (sa1) status chip is at index 0
    expect(capturedChipClicks[0]).toBeDefined();
    await capturedChipClicks[0]();

    expect(statusSpy).toHaveBeenCalledWith("sa1", false);
    expect(promptMock).toHaveBeenCalled();
    expect(stepUpSpy).toHaveBeenCalledWith("123456");
  });

  it("handles revoking service account", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const revokeSpy = vi.spyOn(api.admin, "revokeServiceAccount").mockResolvedValue({} as any);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    // row 1 actions: [0] -> test, [1] -> edit, [2] -> rotate, [3] -> revoke
    expect(capturedIconButtonClicks[3]).toBeDefined();
    await capturedIconButtonClicks[3]();

    expect(revokeSpy).toHaveBeenCalledWith("sa1");
  });

  it("handles rotating service account secret", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const rotateSpy = vi.spyOn(api.admin, "rotateServiceAccountSecret").mockResolvedValue({
      client_id: "client-1",
      client_secret: "secret-rotated",
    } as any);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    // row 1 actions: [0] -> test, [1] -> edit, [2] -> rotate
    expect(capturedIconButtonClicks[2]).toBeDefined();
    await capturedIconButtonClicks[2]();

    expect(rotateSpy).toHaveBeenCalledWith("sa1");
  });

  it("handles editing and updating service account details", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const updateSpy = vi.spyOn(api.admin, "updateServiceAccount").mockResolvedValue({} as any);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    // row 1 actions: [1] -> edit
    capturedIconButtonClicks[1]();
    runRender();

    // Edit dialog inputs:
    // [0] -> name (currently "Service 1")
    // [1] -> expiresAt
    expect(capturedInputs[0]).toBeDefined();
    capturedInputs[0]({ target: { value: "Service 1 Updated" } });

    // Switch check toggled:
    expect(capturedSwitches[0]).toBeDefined();
    capturedSwitches[0]({ target: { checked: false } });

    runRender(); // re-render to update closures
    await capturedSubmits[0]({ preventDefault: vi.fn() });

    expect(updateSpy).toHaveBeenCalledWith("sa1", {
      name: "Service 1 Updated",
      scopes: ["users:read"],
      expires_at: "2026-06-10",
      is_active: false,
    });
  });

  it("handles testing and token probe flows", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    
    // Mock token creation and probe
    const tokenResult = { access_token: "token.payload.signature", expires_in: 3600 };
    const createTokenSpy = vi.spyOn(api.admin, "createServiceToken").mockResolvedValue(tokenResult);
    const probeResult = { ok: true, status: 200, statusText: "OK", body: { message: "success" } };
    const probeSpy = vi.spyOn(api.admin, "probeWithServiceToken").mockResolvedValue(probeResult);

    // Mock global atob for JWT decoding: payload has scopes
    const payload = JSON.stringify({ sub: "sa1", scopes: ["service_accounts:read"] });
    vi.stubGlobal("atob", vi.fn().mockReturnValue(payload));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    // row 1 actions: [0] -> test
    capturedIconButtonClicks[0]();
    runRender();

    // Test Dialog inputs: [0] -> secret input
    expect(capturedInputs[0]).toBeDefined();
    capturedInputs[0]({ target: { value: "my-secret" } });

    runRender();

    // Click "Get Token" button (index 1 is Get Token)
    expect(capturedButtonClicks[1]).toBeDefined();
    await capturedButtonClicks[1]();

    expect(createTokenSpy).toHaveBeenCalledWith({
      client_id: "client-1",
      client_secret: "my-secret",
    });

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    // Click "Run Probe" button (index 2 in capturedButtonClicks)
    expect(capturedButtonClicks[2]).toBeDefined();
    await capturedButtonClicks[2]();

    expect(probeSpy).toHaveBeenCalledWith("/api/v1/admin/service-accounts?limit=1&offset=0", "token.payload.signature");
  });

  it("handles copy secret and done button clicks inside newSecret dialog", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    vi.spyOn(api.admin, "rotateServiceAccountSecret").mockResolvedValue({
      client_id: "client-1",
      client_secret: "secret-rotated",
    } as any);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    // Rotate button is capturedIconButtonClicks[2]
    await capturedIconButtonClicks[2]();
    runRender();

    // Now newSecret is set. It renders the newSecret Dialog.
    // In newSecret dialog, the button is:
    // [1] -> Done button
    // The copy icon button is capturedIconButtonClicks[8] (since we rendered row action buttons and dialog icon buttons)
    expect(capturedIconButtonClicks[8]).toBeDefined();
    await capturedIconButtonClicks[8](); // Trigger copy
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith("secret-rotated");

    expect(capturedButtonClicks[1]).toBeDefined();
    capturedButtonClicks[1](); // Click Done
  });

  it("handles API error conditions for create, update and probe", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    vi.spyOn(api.admin, "createServiceAccount").mockRejectedValue(new Error("invalid_name"));
    vi.spyOn(api.admin, "updateServiceAccount").mockRejectedValue(new Error("db_error"));
    vi.spyOn(api.admin, "probeWithServiceToken").mockRejectedValue(new Error("Blocked"));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    // Create error test
    capturedButtonClicks[0](); // open create
    runRender();
    capturedInputs[0]({ target: { value: "Bad SA" } });
    await capturedSubmits[0]({ preventDefault: vi.fn() });
    runRender();

    // Update error test
    capturedIconButtonClicks[1](); // open edit
    runRender();
    capturedInputs[0]({ target: { value: "Edit Bad SA" } });
    runRender();
    await capturedSubmits[0]({ preventDefault: vi.fn() });
    runRender();

    // Probe error test: first open test and click get token
    vi.spyOn(api.admin, "createServiceToken").mockResolvedValue({ access_token: "tok", expires_in: 60 });
    capturedIconButtonClicks[0](); // open test
    runRender();
    capturedInputs[0]({ target: { value: "secret" } });
    runRender();
    await capturedButtonClicks[1](); // get token
    runRender();
    await Promise.resolve();
    runRender();
    await capturedButtonClicks[2](); // run probe (fails)
    runRender();
  });
});
