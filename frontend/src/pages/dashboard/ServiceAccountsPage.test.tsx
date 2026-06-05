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
const capturedDialogCloses: any[] = [];

vi.mock("@mui/material/Button", () => ({
  default: (props: any) => {
    if (props.onClick) capturedButtonClicks.push(props.onClick);
    return React.createElement("button", { onClick: props.onClick, type: props.type, disabled: props.disabled }, props.children);
  }
}));

vi.mock("@mui/material/Chip", () => ({
  default: (props: any) => {
    if (props.onClick) capturedChipClicks.push(props.onClick);
    return React.createElement("div", { onClick: props.onClick }, props.children ?? props.label);
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
    return React.createElement("input", {
      type: props.type,
      value: props.value,
      onChange: props.onChange,
      disabled: props.disabled,
      readOnly: props.slotProps?.htmlInput?.readOnly,
    });
  }
}));

vi.mock("@mui/material/Switch", () => ({
  default: (props: any) => {
    if (props.onChange) capturedSwitches.push(props.onChange);
    return React.createElement("input", { type: "checkbox", checked: props.checked, onChange: props.onChange });
  }
}));

vi.mock("@mui/material/Dialog", () => ({
  default: (props: any) => {
    if (props.open && props.onClose) capturedDialogCloses.push(props.onClose);
    return props.open ? React.createElement("div", null, props.children) : null;
  }
}));

vi.mock("@mui/material/Tooltip", () => ({
  default: (props: any) => props.children
}));

vi.mock("@mui/x-data-grid", () => ({
  DataGrid: (props: any) => {
    const renderedRows = (props.rows ?? []).map((row: any) => {
      props.getRowId?.(row);
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
    capturedDialogCloses.length = 0;
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
    capturedDialogCloses.length = 0;
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
    const tokenResult = { access_token: "token.payload.signature", token_type: "Bearer", expires_in: 3600 };
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
    vi.spyOn(api.admin, "createServiceToken").mockResolvedValue({ access_token: "tok", token_type: "Bearer", expires_in: 60 });
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

  it("does not run destructive service-account actions when confirmation is cancelled", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const rotateSpy = vi.spyOn(api.admin, "rotateServiceAccountSecret").mockResolvedValue({} as any);
    const revokeSpy = vi.spyOn(api.admin, "revokeServiceAccount").mockResolvedValue({} as any);
    confirmMock.mockReturnValue(false);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    await capturedIconButtonClicks[2](); // rotate
    await capturedIconButtonClicks[3](); // revoke

    expect(confirmMock).toHaveBeenCalledTimes(2);
    expect(rotateSpy).not.toHaveBeenCalled();
    expect(revokeSpy).not.toHaveBeenCalled();
  });

  it("does not complete step-up protected actions when MFA prompt is empty", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const stepUpSpy = vi.spyOn(api, "mfaStepUp").mockResolvedValue({} as any);
    const statusSpy = vi.spyOn(api.admin, "setServiceAccountStatus").mockRejectedValue(new ApiError("mfa_required"));
    promptMock.mockReturnValue("");

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    await capturedChipClicks[0]();

    expect(statusSpy).toHaveBeenCalledTimes(1);
    expect(promptMock).toHaveBeenCalled();
    expect(stepUpSpy).not.toHaveBeenCalled();
    expect(alertMock).not.toHaveBeenCalled();
  });

  it("covers token test guard and token issuance failure paths", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const createTokenSpy = vi.spyOn(api.admin, "createServiceToken").mockRejectedValue(new ApiError("invalid_client"));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedIconButtonClicks[0](); // open test dialog
    runRender();

    await capturedButtonClicks[1](); // get token with no secret returns early
    await capturedButtonClicks[2](); // run probe with no token returns early
    expect(createTokenSpy).not.toHaveBeenCalled();

    capturedInputs[0]({ target: { value: "wrong-secret" } });
    runRender();

    await capturedButtonClicks[1]();

    expect(createTokenSpy).toHaveBeenCalledWith({
      client_id: "client-1",
      client_secret: "wrong-secret",
    });
  });

  it("renders the locked client id field as disabled and read-only", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedIconButtonClicks[1](); // open edit dialog
    const html = runRender();

    expect(html).toContain("value=\"client-1\"");
    expect(html).toContain("disabled=\"\"");
    expect(html).toContain("readOnly=\"\"");
  });

  it("handles destructive action generic errors and repeated scope toggles", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    vi.spyOn(api.admin, "setServiceAccountStatus").mockRejectedValue(new Error("status failed"));
    vi.spyOn(api.admin, "rotateServiceAccountSecret").mockRejectedValue(new Error("rotate failed"));
    vi.spyOn(api.admin, "revokeServiceAccount").mockRejectedValue(new Error("revoke failed"));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    await capturedChipClicks[0](); // status chip
    await capturedIconButtonClicks[2](); // rotate
    await capturedIconButtonClicks[3](); // revoke

    expect(alertMock).toHaveBeenCalledWith("errors.internal_error");

    capturedButtonClicks[0](); // open create
    runRender();
    capturedChipClicks[2]();
    capturedChipClicks[2]();

    capturedIconButtonClicks[1](); // open edit
    runRender();
    capturedInputs[0]({ target: { value: "" } });
    capturedChipClicks[2]();
    capturedChipClicks[2]();
    await capturedSubmits[0]({ preventDefault: vi.fn() });
  });

  it("handles mfa_required responses from rotate and revoke without alerting", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    vi.spyOn(api.admin, "rotateServiceAccountSecret").mockRejectedValue(new ApiError("mfa_required"));
    vi.spyOn(api.admin, "revokeServiceAccount").mockRejectedValue(new ApiError("mfa_required"));
    vi.spyOn(api, "mfaStepUp").mockResolvedValue({} as any);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    await capturedIconButtonClicks[2]();
    await capturedIconButtonClicks[3]();

    expect(alertMock).not.toHaveBeenCalled();
  });

  it("renders invalid and minimal service token payload states plus blocked probe output", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const createTokenSpy = vi.spyOn(api.admin, "createServiceToken")
      .mockResolvedValueOnce({ access_token: "badtoken", token_type: "Bearer", expires_in: 60 })
      .mockResolvedValueOnce({ access_token: "bad.payload.signature", token_type: "Bearer", expires_in: 60 })
      .mockResolvedValueOnce({ access_token: "minimal.payload.signature", token_type: "Bearer", expires_in: 60 });
    const probeSpy = vi.spyOn(api.admin, "probeWithServiceToken").mockResolvedValue({
      ok: false,
      status: 403,
      statusText: "Forbidden",
      body: { error: "forbidden" },
    });

    vi.stubGlobal("atob", vi.fn()
      .mockImplementationOnce(() => "{not-json")
      .mockImplementationOnce(() => JSON.stringify({ cid: "client-1" })));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedIconButtonClicks[0](); // open test dialog
    runRender();
    capturedInputs[0]({ target: { value: "secret" } });
    runRender();

    await capturedButtonClicks[1](); // no JWT payload segment
    runRender();
    expect(createTokenSpy).toHaveBeenCalledTimes(1);

    await capturedButtonClicks[1](); // invalid JWT decode path
    runRender();

    await capturedButtonClicks[1](); // minimal payload: no scopes/sub/exp
    runRender();
    await Promise.resolve();
    runRender();

    const html = runRender();
    expect(html).toContain("noScopes");

    await capturedButtonClicks[2](); // blocked probe branch + prettyJson
    runRender();

    expect(probeSpy).toHaveBeenCalledWith("/api/v1/admin/service-accounts?limit=1&offset=0", "minimal.payload.signature");
  });

  it("updates probe target selection and closes the testing dialog", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedIconButtonClicks[0](); // open test dialog
    runRender();

    expect(capturedInputs[1]).toBeDefined();
    capturedInputs[1]({ target: { value: "users" } });
    expect(stateStore[17]).toBe("users");

    const done = capturedButtonClicks[capturedButtonClicks.length - 1];
    expect(done).toBeDefined();
    done();
    expect(stateStore[15]).toBeNull();
  });

  it("toggles edit scopes and skips update when the edit name is blank", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const updateSpy = vi.spyOn(api.admin, "updateServiceAccount").mockResolvedValue({} as any);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedIconButtonClicks[1](); // open edit
    runRender();

    expect(capturedChipClicks[2]).toBeDefined();
    capturedChipClicks[2](); // remove users:read
    capturedChipClicks[2](); // add users:read again
    capturedInputs[0]({ target: { value: "" } });
    runRender();

    await capturedSubmits[0]({ preventDefault: vi.fn() });

    expect(updateSpy).not.toHaveBeenCalled();
  });

  it("handles edit date change, cancel, and non-Error update failures", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    vi.spyOn(api.admin, "updateServiceAccount").mockRejectedValue("plain failure");

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedIconButtonClicks[1](); // open edit
    runRender();

    expect(capturedInputs[1]).toBeDefined();
    capturedInputs[1]({ target: { value: "2026-12-24" } });
    expect(stateStore[11]).toBe("2026-12-24");

    await capturedSubmits[0]({ preventDefault: vi.fn() });
    expect(stateStore[14]).toBe("errors.internal_error");

    runRender();
    expect(capturedButtonClicks[1]).toBeDefined();
    capturedButtonClicks[1](); // cancel edit dialog
    expect(stateStore[8]).toBeNull();
  });

  it("clears probe result and shows an internal error when probing fails", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    vi.spyOn(api.admin, "createServiceToken").mockResolvedValue({
      access_token: "minimal.payload.signature",
      token_type: "Bearer",
      expires_in: 60,
    });
    vi.spyOn(api.admin, "probeWithServiceToken").mockRejectedValue(new Error("probe failed"));
    vi.stubGlobal("atob", vi.fn().mockReturnValue(JSON.stringify({ cid: "client-1" })));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedIconButtonClicks[0]();
    runRender();
    capturedInputs[0]({ target: { value: "secret" } });
    runRender();
    await capturedButtonClicks[1]();
    runRender();
    await Promise.resolve();
    runRender();

    await capturedButtonClicks[2]();

    expect(stateStore[20]).toBeNull();
    expect(stateStore[21]).toBe("errors.internal_error");
  });

  it("handles copy without a secret and dialog close callbacks", async () => {
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

    // No new secret exists yet; first row action copy path must no-op if reached
    stateStore[1] = null;
    runRender();

    capturedButtonClicks[0](); // open create
    runRender();
    expect(capturedDialogCloses[0]).toBeDefined();
    capturedDialogCloses[0]();
    expect(stateStore[0]).toBe(false);
    runRender();

    capturedIconButtonClicks[1](); // open edit
    runRender();
    expect(capturedDialogCloses[0]).toBeDefined();
    capturedDialogCloses[0]();
    expect(stateStore[8]).toBeNull();
    runRender();

    capturedIconButtonClicks[0](); // open test
    runRender();
    expect(capturedDialogCloses[0]).toBeDefined();
    capturedDialogCloses[0]();
    expect(stateStore[15]).toBeNull();
    runRender();

    await capturedIconButtonClicks[2](); // rotate to open secret dialog
    runRender();
    expect(capturedDialogCloses[0]).toBeDefined();
    capturedDialogCloses[0]();
    expect(stateStore[1]).toBeNull();
  });

  it("renders access denied, expired accounts, and scopes without action suffixes", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["user"] } as any);
    const listSpy = vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());

    let html = runRender();
    await Promise.resolve();
    await Promise.resolve();

    expect(html).toContain("accessDenied");
    expect(listSpy).not.toHaveBeenCalled();

    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    listSpy.mockResolvedValue({
      data: [
        {
          id: "expired",
          name: "Expired Service",
          client_id: "client-expired",
          is_active: true,
          scopes: ["custom"],
          created_at: "2026-06-04T12:00:00Z",
          expires_at: "2020-01-01T00:00:00Z",
        },
      ],
      total: 1,
    });

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    html = runRender();

    expect(html).toContain("expired");
    expect(html).toContain("custom");
  });

  it("renders loading, saving, copied, inactive, warning, expiring, and passing result states", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();

    stateStore[0] = true;
    stateStore[6] = true;
    expect(runRender()).toContain("creating");

    stateStore[0] = false;
    stateStore[8] = getMockAccounts().data[0];
    stateStore[9] = "Service 1";
    stateStore[10] = ["users:read"];
    stateStore[11] = "2020-01-01";
    stateStore[12] = false;
    stateStore[13] = true;
    expect(runRender()).toContain("saving");

    stateStore[8] = null;
    stateStore[13] = false;
    stateStore[15] = getMockAccounts().data[1];
    stateStore[16] = "secret";
    stateStore[18] = { access_token: "token", token_type: "Bearer", expires_in: 60 };
    stateStore[19] = { sub: "service", scopes: ["users:read"], exp: 1780000000 };
    stateStore[20] = { ok: true, status: 200, statusText: "OK", body: { ok: true } };
    stateStore[22] = true;
    stateStore[23] = true;
    let html = runRender();
    expect(html).toContain("inactive");
    expect(html).toContain("users:read");
    expect(html).toContain("probePassed");
    expect(html).toContain("200 OK");

    stateStore[1] = {
      id: "sa1",
      name: "Service 1",
      client_id: "client-1",
      client_secret: "secret-rotated",
      is_active: true,
      scopes: ["users:read"],
      created_at: "2026-06-04T12:00:00Z",
      expires_at: null,
    };
    stateStore[2] = true;
    html = runRender();
    expect(html).toContain("secret-rotated");
  });

  it("runs the copy success timeout callback", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    vi.spyOn(api.admin, "rotateServiceAccountSecret").mockResolvedValue({
      client_id: "client-1",
      client_secret: "secret-rotated",
    } as any);
    vi.stubGlobal("setTimeout", vi.fn((fn: () => void) => {
      fn();
      return 1;
    }));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    await capturedIconButtonClicks[2]();
    runRender();

    const copy = capturedIconButtonClicks[capturedIconButtonClicks.length - 1];
    expect(copy).toBeDefined();
    await copy();
    await Promise.resolve();

    expect(navigator.clipboard.writeText).toHaveBeenCalledWith("secret-rotated");
    expect(setTimeout).toHaveBeenCalled();
  });

  it("covers null viewer, no-expiry edit, fallback probe target, and empty test-error rendering", async () => {
    vi.mocked(useMeContext).mockReturnValue(null);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());

    let html = runRender();
    expect(html).toContain("accessDenied");

    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedIconButtonClicks[5](); // row 2 edit
    expect(stateStore[11]).toBe("");
    stateStore[17] = "unknown";
    stateStore[21] = "";
    html = runRender();
    expect(html).toContain("client-2");
    expect(html).not.toContain("errors.internal_error");
  });

  it("covers non-Error create and token failures plus Error update failures", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    vi.spyOn(api.admin, "createServiceAccount").mockRejectedValue("plain create failure");
    vi.spyOn(api.admin, "updateServiceAccount").mockRejectedValue(new Error("edit_error"));
    vi.spyOn(api.admin, "createServiceToken").mockRejectedValue("plain token failure");

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedButtonClicks[0](); // open create
    runRender();
    capturedInputs[0]({ target: { value: "Plain Failure" } });
    runRender();
    await capturedSubmits[0]({ preventDefault: vi.fn() });
    expect(stateStore[7]).toBe("errors.internal_error");

    stateStore[0] = false;
    runRender();
    capturedIconButtonClicks[1](); // open edit
    runRender();
    capturedInputs[0]({ target: { value: "Edit Failure" } });
    stateStore[11] = "";
    runRender();
    await capturedSubmits[0]({ preventDefault: vi.fn() });
    expect(stateStore[14]).toBe("errors.edit_error");

    stateStore[8] = null;
    runRender();
    capturedIconButtonClicks[0](); // open test
    runRender();
    capturedInputs[0]({ target: { value: "secret" } });
    runRender();
    await capturedButtonClicks[1]();
    expect(stateStore[21]).toBe("errors.invalid_client");
  });

  it("covers undefined MFA prompt returns", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const statusSpy = vi.spyOn(api.admin, "setServiceAccountStatus").mockRejectedValue(new ApiError("mfa_required"));
    const stepUpSpy = vi.spyOn(api, "mfaStepUp").mockResolvedValue({} as any);
    promptMock.mockReturnValue(undefined);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    await capturedChipClicks[0]();

    expect(statusSpy).toHaveBeenCalledTimes(1);
    expect(stepUpSpy).not.toHaveBeenCalled();
    expect(alertMock).not.toHaveBeenCalled();
  });

  it("uses fallback probe target when probing with an unknown key and no test error", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const probeSpy = vi.spyOn(api.admin, "probeWithServiceToken").mockResolvedValue({
      ok: true,
      status: 200,
      statusText: "OK",
      body: {},
    });

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    stateStore[15] = getMockAccounts().data[0];
    stateStore[17] = "missing";
    stateStore[18] = { access_token: "fallback-token", token_type: "Bearer", expires_in: 60 };
    stateStore[19] = { scopes: [] };
    stateStore[21] = "";
    const html = runRender();
    expect(html).not.toContain("errors.internal_error");

    await capturedButtonClicks[2]();

    expect(probeSpy).toHaveBeenCalledWith("/api/v1/admin/service-accounts?limit=1&offset=0", "fallback-token");

    stateStore[21] = "errors.internal_error";
    expect(runRender()).toContain("errors.internal_error");
  });
  });
