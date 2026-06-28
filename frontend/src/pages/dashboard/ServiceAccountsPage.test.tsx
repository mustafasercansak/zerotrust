import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { render, screen, fireEvent, cleanup, waitFor } from "@testing-library/react";
import ServiceAccountsPage from "./ServiceAccountsPage";
import { api, ApiError, type ServiceAccount } from "@/lib/api";
import { useMeContext } from "@/contexts/MeContext";
import { toast } from "sonner";

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));

vi.mock("@/contexts/MeContext", () => ({
  useMeContext: vi.fn(),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: any) => {
      if (options?.name) return `${key}:${options.name}`;
      return key;
    },
    tCommon: (key: string) => key,
    i18n: { language: "en" },
  }),
}));

const mockRunWithStepUp = vi.fn().mockImplementation(async (action: any) => action());

vi.mock("@/hooks/useStepUp", () => ({
  useStepUp: () => ({
    runWithStepUp: mockRunWithStepUp,
    stepUpOpen: false,
    stepUpError: "",
    stepUpSubmitting: false,
    handleStepUpSubmit: vi.fn(),
    handleStepUpClose: vi.fn(),
  }),
}));

vi.mock("@/components/StepUpMfaDialog", () => ({
  StepUpMfaDialog: () => null,
}));

vi.mock("@mui/x-data-grid", () => ({
  DataGrid: (props: any) => {
    const renderedRows = (props.rows ?? []).map((row: any) => {
      props.getRowId?.(row);
      return React.createElement("div", { key: row.id, className: "mock-row", "data-testid": `row-${row.id}` },
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
  let confirmMock: any;
  let promptMock: any;
  let alertMock: any;

  beforeEach(() => {
    vi.useRealTimers();
    confirmMock = vi.spyOn(window, "confirm").mockReturnValue(true);
    promptMock = vi.spyOn(window, "prompt").mockReturnValue("123456");
    alertMock = vi.spyOn(window, "alert").mockImplementation(() => {});

    vi.spyOn(window, "setInterval");
    vi.spyOn(window, "clearInterval");
    vi.spyOn(window, "addEventListener");
    vi.spyOn(window, "removeEventListener");

    vi.mocked(toast.error).mockClear();
    vi.mocked(toast.success).mockClear();
    mockRunWithStepUp.mockClear();
    mockRunWithStepUp.mockImplementation(async (action: any) => action());

    vi.stubGlobal("navigator", {
      clipboard: {
        writeText: vi.fn().mockResolvedValue({} as any),
      },
    });

    class MockEventSource {
      close = vi.fn();
      addEventListener = vi.fn();
      removeEventListener = vi.fn();
    }
    vi.stubGlobal("EventSource", MockEventSource);
  });

  afterEach(() => {
    cleanup();
    document.body.innerHTML = "";
    document.body.removeAttribute("style");
    document.body.removeAttribute("class");
    vi.restoreAllMocks();
  });

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
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    const listSpy = vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());

    render(<ServiceAccountsPage />);

    await waitFor(() => {
      expect(screen.getByText("Service 1")).toBeDefined();
    });
    expect(listSpy).toHaveBeenCalled();
  });

  it("handles creating a service account successfully", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const createSpy = vi.spyOn(api.admin, "createServiceAccount").mockResolvedValue({
      client_id: "client-new",
      client_secret: "secret-new",
    } as any);

    render(<ServiceAccountsPage />);

    // Open create dialog
    const createBtn = await screen.findByRole("button", { name: /\+ create/i });
    fireEvent.click(createBtn);

    // Input fields
    const nameInput = screen.getByLabelText(/name/i);
    fireEvent.change(nameInput, { target: { value: "New SA" } });

    const expiresInput = screen.getByLabelText(/expiresAt/i);
    fireEvent.change(expiresInput, { target: { value: "2026-12-31" } });

    // Toggle scope chip (pick the first read chip)
    const scopeChip = screen.getAllByText("read")[0];
    fireEvent.click(scopeChip);

    // Submit
    const submitBtn = document.querySelector('button[type="submit"]')!;
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(createSpy).toHaveBeenCalledWith({
        name: "New SA",
        scopes: ["users:read"],
        expires_at: "2026-12-31",
      });
    });
  });

  it("handles toggling status with MFA challenge", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const stepUpSpy = vi.spyOn(api, "mfaStepUp").mockResolvedValue({} as any);
    const statusSpy = vi.spyOn(api.admin, "setServiceAccountStatus")
      .mockRejectedValueOnce(new ApiError("mfa_required"))
      .mockResolvedValueOnce({} as any);

    // Retrying with MFA challenge simulated
    mockRunWithStepUp.mockImplementationOnce(async (action: any) => {
      try { return await action(); }
      catch { await api.mfaStepUp("123456"); return action(); }
    });

    render(<ServiceAccountsPage />);

    const statusChip = await screen.findByText("active");
    fireEvent.click(statusChip);

    await waitFor(() => {
      expect(statusSpy).toHaveBeenCalledWith("sa1", false);
      expect(mockRunWithStepUp).toHaveBeenCalled();
      expect(stepUpSpy).toHaveBeenCalledWith("123456");
    });
  });

  it("handles revoking service account", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const revokeSpy = vi.spyOn(api.admin, "revokeServiceAccount").mockResolvedValue({} as any);

    render(<ServiceAccountsPage />);

    const revokeBtn = (await screen.findAllByTestId("DeleteOutlinedIcon"))[0].closest("button")!;
    fireEvent.click(revokeBtn);

    await waitFor(() => {
      expect(revokeSpy).toHaveBeenCalledWith("sa1");
    });
  });

  it("handles rotating service account secret", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const rotateSpy = vi.spyOn(api.admin, "rotateServiceAccountSecret").mockResolvedValue({
      client_id: "client-1",
      client_secret: "secret-rotated",
    } as any);

    render(<ServiceAccountsPage />);

    const rotateBtn = (await screen.findAllByTestId("KeyIcon"))[0].closest("button")!;
    fireEvent.click(rotateBtn);

    await waitFor(() => {
      expect(rotateSpy).toHaveBeenCalledWith("sa1");
    });
  });

  it("handles editing and updating service account details", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const updateSpy = vi.spyOn(api.admin, "updateServiceAccount").mockResolvedValue({} as any);

    render(<ServiceAccountsPage />);

    const editBtn = (await screen.findAllByTestId("EditIcon"))[0].closest("button")!;
    fireEvent.click(editBtn);

    const nameInput = screen.getByLabelText(/name/i);
    fireEvent.change(nameInput, { target: { value: "Service 1 Updated" } });

    const activeSwitch = screen.getByRole("switch");
    fireEvent.click(activeSwitch);

    const saveBtn = document.querySelector('button[type="submit"]')!;
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(updateSpy).toHaveBeenCalledWith("sa1", {
        name: "Service 1 Updated",
        scopes: ["users:read"],
        expires_at: "2026-06-10",
        is_active: false,
      });
    });
  });

  it("handles testing and token probe flows", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());

    const tokenResult = { access_token: "token.payload.signature", token_type: "Bearer", expires_in: 3600 };
    const createTokenSpy = vi.spyOn(api.admin, "createServiceToken").mockResolvedValue(tokenResult);
    const probeResult = { ok: true, status: 200, statusText: "OK", body: { message: "success" } };
    const probeSpy = vi.spyOn(api.admin, "probeWithServiceToken").mockResolvedValue(probeResult);

    const payload = JSON.stringify({ sub: "sa1", scopes: ["service_accounts:read"] });
    vi.stubGlobal("atob", vi.fn().mockReturnValue(payload));

    render(<ServiceAccountsPage />);

    const testBtn = (await screen.findAllByTestId("ScienceIcon"))[0].closest("button")!;
    fireEvent.click(testBtn);

    const secretInput = screen.getByLabelText(/secretLabel/i);
    fireEvent.change(secretInput, { target: { value: "my-secret" } });

    const getTokenBtn = screen.getByRole("button", { name: "getToken" });
    fireEvent.click(getTokenBtn);

    await waitFor(() => {
      expect(createTokenSpy).toHaveBeenCalledWith({
        client_id: "client-1",
        client_secret: "my-secret",
      });
    });

    const runProbeBtn = screen.getByRole("button", { name: "runProbe" });
    fireEvent.click(runProbeBtn);

    await waitFor(() => {
      expect(probeSpy).toHaveBeenCalledWith("/api/v1/admin/service-accounts?limit=1&offset=0", "token.payload.signature");
    });
  });

  it("handles copy secret and done button clicks inside newSecret dialog", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    vi.spyOn(api.admin, "rotateServiceAccountSecret").mockResolvedValue({
      client_id: "client-1",
      client_secret: "secret-rotated",
    } as any);

    render(<ServiceAccountsPage />);

    const rotateBtn = (await screen.findAllByTestId("KeyIcon"))[0].closest("button")!;
    fireEvent.click(rotateBtn);

    const copyBtn = (await screen.findByTestId("ContentCopyIcon")).closest("button")!;
    fireEvent.click(copyBtn);

    expect(navigator.clipboard.writeText).toHaveBeenCalledWith("secret-rotated");

    const doneBtn = screen.getByRole("button", { name: "done" });
    fireEvent.click(doneBtn);
  });

  it("handles API error conditions for create, update and probe", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const createSpy = vi.spyOn(api.admin, "createServiceAccount").mockRejectedValue(new Error("invalid_name"));
    const updateSpy = vi.spyOn(api.admin, "updateServiceAccount").mockRejectedValue(new Error("db_error"));
    const probeSpy = vi.spyOn(api.admin, "probeWithServiceToken").mockRejectedValue(new Error("Blocked"));
    vi.spyOn(api.admin, "createServiceToken").mockResolvedValue({ access_token: "tok.payload.sig", token_type: "Bearer", expires_in: 60 });
    const payload = JSON.stringify({ sub: "sa1", scopes: ["service_accounts:read"] });
    vi.stubGlobal("atob", vi.fn().mockReturnValue(payload));

    render(<ServiceAccountsPage />);

    // Create Error
    const createBtn = await screen.findByRole("button", { name: /\+ create/i });
    fireEvent.click(createBtn);
    const createName = screen.getByLabelText(/name/i);
    fireEvent.change(createName, { target: { value: "Bad SA" } });
    fireEvent.click(document.querySelector('button[type="submit"]')!);
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.invalid_name");
    });
    // Close Create Dialog and wait for transition
    fireEvent.click(screen.getByRole("button", { name: "cancel" }));
    await waitFor(() => {
      expect(screen.queryByText("createTitle")).toBeNull();
    });

    // Update Error
    const editBtn = (await screen.findAllByTestId("EditIcon"))[0].closest("button")!;
    fireEvent.click(editBtn);
    const editName = screen.getByLabelText(/name/i);
    fireEvent.change(editName, { target: { value: "Edit Bad SA" } });
    fireEvent.click(document.querySelector('button[type="submit"]')!);
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.db_error");
    });
    // Close Edit Dialog and wait for transition
    fireEvent.click(screen.getByRole("button", { name: "cancel" }));
    await waitFor(() => {
      expect(screen.queryByText("editTitle")).toBeNull();
    });

    // Probe Error
    const testBtn = (await screen.findAllByTestId("ScienceIcon"))[0].closest("button")!;
    fireEvent.click(testBtn);
    const secretInput = screen.getByLabelText(/secretLabel/i);
    fireEvent.change(secretInput, { target: { value: "secret" } });
    fireEvent.click(screen.getByRole("button", { name: "getToken" }));
    await waitFor(() => {
      expect(screen.getByText("tokenReady")).toBeDefined();
    });
    fireEvent.click(screen.getByRole("button", { name: "runProbe" }));
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.internal_error");
    });
    // Close Test Dialog and wait for transition
    fireEvent.click(screen.getByRole("button", { name: "done" }));
    await waitFor(() => {
      expect(screen.queryByText("testSubtitle")).toBeNull();
    });
  });

  it("does not run destructive service-account actions when confirmation is cancelled", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const rotateSpy = vi.spyOn(api.admin, "rotateServiceAccountSecret");
    const revokeSpy = vi.spyOn(api.admin, "revokeServiceAccount");
    confirmMock.mockReturnValue(false);

    render(<ServiceAccountsPage />);

    const rotateBtn = (await screen.findAllByTestId("KeyIcon"))[0].closest("button")!;
    fireEvent.click(rotateBtn);

    const revokeBtn = (await screen.findAllByTestId("DeleteOutlinedIcon"))[0].closest("button")!;
    fireEvent.click(revokeBtn);

    expect(confirmMock).toHaveBeenCalledTimes(2);
    expect(rotateSpy).not.toHaveBeenCalled();
    expect(revokeSpy).not.toHaveBeenCalled();
  });

  it("does not complete step-up protected actions when MFA prompt is empty", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const stepUpSpy = vi.spyOn(api, "mfaStepUp");
    const statusSpy = vi.spyOn(api.admin, "setServiceAccountStatus").mockRejectedValue(new ApiError("mfa_required"));

    // User cancels MFA challenge (runWithStepUp propagates it)
    mockRunWithStepUp.mockImplementationOnce(async (action: any) => action());

    render(<ServiceAccountsPage />);

    const statusChip = await screen.findByText("active");
    fireEvent.click(statusChip);

    await waitFor(() => {
      expect(statusSpy).toHaveBeenCalledTimes(1);
    });
    expect(stepUpSpy).not.toHaveBeenCalled();
  });

  it("covers token test guard and token issuance failure paths", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const createTokenSpy = vi.spyOn(api.admin, "createServiceToken").mockRejectedValue(new ApiError("invalid_client"));

    render(<ServiceAccountsPage />);

    const testBtn = (await screen.findAllByTestId("ScienceIcon"))[0].closest("button")!;
    fireEvent.click(testBtn);

    // click getToken with empty secret (should be disabled or ignored)
    const getTokenBtn = screen.getByRole("button", { name: "getToken" });
    fireEvent.click(getTokenBtn);

    // click runProbe with no token (should be disabled or ignored)
    const runProbeBtn = screen.getByRole("button", { name: "runProbe" });
    fireEvent.click(runProbeBtn);

    expect(createTokenSpy).not.toHaveBeenCalled();

    const secretInput = screen.getByLabelText(/secretLabel/i);
    fireEvent.change(secretInput, { target: { value: "wrong-secret" } });
    fireEvent.click(getTokenBtn);

    await waitFor(() => {
      expect(createTokenSpy).toHaveBeenCalledWith({
        client_id: "client-1",
        client_secret: "wrong-secret",
      });
      expect(toast.error).toHaveBeenCalledWith("errors.invalid_client");
    });
  });

  it("renders the locked client id field as disabled and read-only", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());

    render(<ServiceAccountsPage />);

    const editBtn = (await screen.findAllByTestId("EditIcon"))[0].closest("button")!;
    fireEvent.click(editBtn);

    const clientIdInput = screen.getByLabelText(/clientId/i) as HTMLInputElement;
    expect(clientIdInput.disabled).toBe(true);
    expect(clientIdInput.readOnly).toBe(true);
    expect(clientIdInput.value).toBe("client-1");
  });

  it("handles destructive action generic errors and repeated scope toggles", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    vi.spyOn(api.admin, "setServiceAccountStatus").mockRejectedValue(new Error("status failed"));
    vi.spyOn(api.admin, "rotateServiceAccountSecret").mockRejectedValue(new Error("rotate failed"));
    vi.spyOn(api.admin, "revokeServiceAccount").mockRejectedValue(new Error("revoke failed"));

    render(<ServiceAccountsPage />);

    // Click status chip, rotate, revoke
    const statusChip = await screen.findByText("active");
    fireEvent.click(statusChip);

    const rotateBtn = (await screen.findAllByTestId("KeyIcon"))[0].closest("button")!;
    fireEvent.click(rotateBtn);

    const revokeBtn = (await screen.findAllByTestId("DeleteOutlinedIcon"))[0].closest("button")!;
    fireEvent.click(revokeBtn);

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.internal_error");
    });

    // Create scopes repeated toggles
    const createBtn = screen.getByRole("button", { name: /\+ create/i });
    fireEvent.click(createBtn);
    const scopeChip = screen.getAllByText("read")[0];
    fireEvent.click(scopeChip); // Toggle off
    fireEvent.click(scopeChip); // Toggle on
  });

  it("handles mfa_required responses from rotate and revoke without alerting", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    vi.spyOn(api.admin, "rotateServiceAccountSecret").mockRejectedValue(new ApiError("mfa_required"));
    vi.spyOn(api.admin, "revokeServiceAccount").mockRejectedValue(new ApiError("mfa_required"));

    render(<ServiceAccountsPage />);

    const rotateBtn = (await screen.findAllByTestId("KeyIcon"))[0].closest("button")!;
    fireEvent.click(rotateBtn);

    const revokeBtn = (await screen.findAllByTestId("DeleteOutlinedIcon"))[0].closest("button")!;
    fireEvent.click(revokeBtn);

    await waitFor(() => {
      expect(toast.error).not.toHaveBeenCalled();
    });
  });

  it("renders invalid and minimal service token payload states plus blocked probe output", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
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

    render(<ServiceAccountsPage />);

    const testBtn = (await screen.findAllByTestId("ScienceIcon"))[0].closest("button")!;
    fireEvent.click(testBtn);

    const secretInput = screen.getByLabelText(/secretLabel/i);
    fireEvent.change(secretInput, { target: { value: "secret" } });

    // 1. badtoken (no payload segment)
    fireEvent.click(screen.getByRole("button", { name: "getToken" }));
    await waitFor(() => {
      expect(screen.queryByText("tokenReady")).toBeNull();
    });

    // 2. bad.payload.signature (not-json)
    fireEvent.click(screen.getByRole("button", { name: "getToken" }));
    await waitFor(() => {
      expect(screen.queryByText("tokenReady")).toBeNull();
    });

    // 3. minimal.payload.signature (no scopes/sub/exp)
    fireEvent.click(screen.getByRole("button", { name: "getToken" }));
    await waitFor(() => {
      expect(screen.getByText("noScopes")).toBeDefined();
    });

    // Run probe
    fireEvent.click(screen.getByRole("button", { name: "runProbe" }));
    await waitFor(() => {
      expect(screen.getByText("probeBlocked")).toBeDefined();
      expect(screen.getByText(/forbidden/)).toBeDefined();
    });
  });

  it("updates probe target selection and closes the testing dialog", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());

    render(<ServiceAccountsPage />);

    const testBtn = (await screen.findAllByTestId("ScienceIcon"))[0].closest("button")!;
    fireEvent.click(testBtn);

    // target Select key
    const targetSelect = screen.getByRole("combobox", { name: /probeTarget/i });
    fireEvent.mouseDown(targetSelect);

    const usersOption = await screen.findByRole("option", { name: /GET Users/i });
    fireEvent.click(usersOption);

    // Done closes
    const doneBtn = screen.getByRole("button", { name: "done" });
    fireEvent.click(doneBtn);

    await waitFor(() => {
      expect(screen.queryByText("testSubtitle")).toBeNull();
    });
  });

  it("toggles edit scopes and skips update when the edit name is blank", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const updateSpy = vi.spyOn(api.admin, "updateServiceAccount").mockResolvedValue({} as any);

    render(<ServiceAccountsPage />);

    const editBtn = (await screen.findAllByTestId("EditIcon"))[0].closest("button")!;
    fireEvent.click(editBtn);

    const nameInput = screen.getByLabelText(/name/i);
    fireEvent.change(nameInput, { target: { value: "" } }); // Blank name

    const scopeChip = screen.getAllByText("read")[0];
    fireEvent.click(scopeChip); // toggle off
    fireEvent.click(scopeChip); // toggle on

    const saveBtn = document.querySelector('button[type="submit"]')!;
    fireEvent.click(saveBtn);

    expect(updateSpy).not.toHaveBeenCalled();
  });

  it("handles edit date change, cancel, and non-Error update failures", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const updateSpy = vi.spyOn(api.admin, "updateServiceAccount").mockRejectedValue("plain failure");

    render(<ServiceAccountsPage />);

    const editBtn = (await screen.findAllByTestId("EditIcon"))[0].closest("button")!;
    fireEvent.click(editBtn);

    const expiresInput = screen.getByLabelText(/expiresAt/i);
    fireEvent.change(expiresInput, { target: { value: "2026-12-24" } });

    const saveBtn = document.querySelector('button[type="submit"]')!;
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.internal_error");
    });

    // Cancel closes dialog
    const cancelBtn = screen.getByRole("button", { name: "cancel" });
    fireEvent.click(cancelBtn);
  });

  it("clears probe result and shows an internal error when probing fails", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    vi.spyOn(api.admin, "createServiceToken").mockResolvedValue({
      access_token: "minimal.payload.signature",
      token_type: "Bearer",
      expires_in: 60,
    });
    vi.spyOn(api.admin, "probeWithServiceToken").mockRejectedValue(new Error("probe failed"));
    vi.stubGlobal("atob", vi.fn().mockReturnValue(JSON.stringify({ cid: "client-1" })));

    render(<ServiceAccountsPage />);

    const testBtn = (await screen.findAllByTestId("ScienceIcon"))[0].closest("button")!;
    fireEvent.click(testBtn);

    const secretInput = screen.getByLabelText(/secretLabel/i);
    fireEvent.change(secretInput, { target: { value: "secret" } });

    fireEvent.click(screen.getByRole("button", { name: "getToken" }));
    await waitFor(() => {
      expect(screen.getByText("tokenReady")).toBeDefined();
    });

    fireEvent.click(screen.getByRole("button", { name: "runProbe" }));

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.internal_error");
    });
  });

  it("handles copy without a secret and dialog close callbacks", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());

    render(<ServiceAccountsPage />);

    // Trigger dialog closes by clicking backdrop / Close button
    // 1. Create dialog Close
    fireEvent.click(screen.getByRole("button", { name: /\+ create/i }));
    fireEvent.click(screen.getByRole("button", { name: "cancel" }));
    await waitFor(() => expect(screen.queryByText("createTitle")).toBeNull());

    // 2. Edit dialog Close
    const editBtn = (await screen.findAllByTestId("EditIcon"))[0].closest("button")!;
    fireEvent.click(editBtn);
    fireEvent.click(screen.getByRole("button", { name: "cancel" }));
    await waitFor(() => expect(screen.queryByText("editTitle")).toBeNull());

    // 3. Test dialog Close
    const testBtn = (await screen.findAllByTestId("ScienceIcon"))[0].closest("button")!;
    fireEvent.click(testBtn);
    fireEvent.click(screen.getByRole("button", { name: "done" }));
    await waitFor(() => expect(screen.queryByText("testSubtitle")).toBeNull());
  });

  it("renders access denied, expired accounts, and scopes without action suffixes", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["user"] } as any);
    const listSpy = vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());

    render(<ServiceAccountsPage />);
    await waitFor(() => {
      expect(screen.getByText("accessDenied")).toBeDefined();
    });
    expect(listSpy).not.toHaveBeenCalled();

    // Reset user back to admin and load expired accounts
    cleanup();
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
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

    render(<ServiceAccountsPage />);
    await waitFor(() => {
      expect(screen.getByText("Expired Service")).toBeDefined();
      expect(screen.getByText("expired")).toBeDefined();
      expect(screen.getByText("custom")).toBeDefined();
    });
  });

  it("renders loading, saving, copied, inactive, warning, expiring, and passing result states", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const createSpy = vi.spyOn(api.admin, "createServiceAccount").mockReturnValue(new Promise(() => {}));
    const updateSpy = vi.spyOn(api.admin, "updateServiceAccount").mockReturnValue(new Promise(() => {}));
    const tokenSpy = vi.spyOn(api.admin, "createServiceToken").mockResolvedValue({
      access_token: "minimal.payload.signature",
      token_type: "Bearer",
      expires_in: 60,
    });
    const probeSpy = vi.spyOn(api.admin, "probeWithServiceToken").mockResolvedValue({
      ok: true,
      status: 200,
      statusText: "OK",
      body: { status: "ok" },
    });
    vi.stubGlobal("atob", vi.fn().mockReturnValue(JSON.stringify({ sub: "sa1", scopes: ["users:read"] })));

    render(<ServiceAccountsPage />);

    // 1. Loading/Creating State: submit button shows 'creating'
    fireEvent.click(screen.getByRole("button", { name: /\+ create/i }));
    fireEvent.change(screen.getByLabelText(/name/i), { target: { value: "Creating State" } });
    fireEvent.click(document.querySelector('button[type="submit"]')!);
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "creating" })).toBeDefined();
    });
    // Close Create Dialog
    fireEvent.click(screen.getByRole("button", { name: "cancel" }));
    await waitFor(() => expect(screen.queryByText("createTitle")).toBeNull());

    // 2. Saving/Updating State: edit submit shows 'saving'
    const editBtn = (await screen.findAllByTestId("EditIcon"))[0].closest("button")!;
    fireEvent.click(editBtn);
    fireEvent.change(screen.getByLabelText(/name/i), { target: { value: "Saving State" } });
    fireEvent.click(document.querySelector('button[type="submit"]')!);
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "saving" })).toBeDefined();
    });
    // Close Edit Dialog
    fireEvent.click(screen.getByRole("button", { name: "cancel" }));
    await waitFor(() => expect(screen.queryByText("editTitle")).toBeNull());

    // 3. Inactive/Copied/Expiring/Passing Result states
    // Inactive chip for row 2 (which is inactive)
    const inactiveChip = await screen.findByText("inactive");
    expect(inactiveChip).toBeDefined();

    // Probe passing state:
    const testBtn = (await screen.findAllByTestId("ScienceIcon"))[0].closest("button")!;
    fireEvent.click(testBtn);
    fireEvent.change(screen.getByLabelText(/secretLabel/i), { target: { value: "secret" } });
    fireEvent.click(screen.getByRole("button", { name: "getToken" }));
    await waitFor(() => {
      expect(screen.getByText("tokenReady")).toBeDefined();
    });
    fireEvent.click(screen.getByRole("button", { name: "runProbe" }));
    await waitFor(() => {
      expect(screen.getByText("probePassed")).toBeDefined();
      expect(screen.getByText(/200 OK/)).toBeDefined();
    });
    // Close Test Dialog
    fireEvent.click(screen.getByRole("button", { name: "done" }));
    await waitFor(() => expect(screen.queryByText("testSubtitle")).toBeNull());
  });

  it("runs the copy success timeout callback", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    vi.spyOn(api.admin, "rotateServiceAccountSecret").mockResolvedValue({
      client_id: "client-1",
      client_secret: "secret-rotated",
    } as any);
    vi.spyOn(global, "setTimeout");

    render(<ServiceAccountsPage />);

    const rotateBtn = (await screen.findAllByTestId("KeyIcon"))[0].closest("button")!;
    fireEvent.click(rotateBtn);

    const copyBtn = (await screen.findByTestId("ContentCopyIcon")).closest("button")!;
    fireEvent.click(copyBtn);

    expect(setTimeout).toHaveBeenCalled();
  });

  it("covers null viewer, no-expiry edit, fallback probe target, and empty test-error rendering", async () => {
    vi.mocked(useMeContext).mockReturnValue(null);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());

    render(<ServiceAccountsPage />);
    await waitFor(() => {
      expect(screen.getByText("accessDenied")).toBeDefined();
    });
  });

  it("covers non-Error create and token failures plus Error update failures", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    vi.spyOn(api.admin, "createServiceAccount").mockRejectedValue("plain create failure");
    vi.spyOn(api.admin, "updateServiceAccount").mockRejectedValue(new Error("edit_error"));
    vi.spyOn(api.admin, "createServiceToken").mockRejectedValue("plain token failure");

    render(<ServiceAccountsPage />);

    // Create failure
    fireEvent.click(screen.getByRole("button", { name: /\+ create/i }));
    fireEvent.change(screen.getByLabelText(/name/i), { target: { value: "Plain Failure" } });
    fireEvent.click(document.querySelector('button[type="submit"]')!);
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.internal_error");
    });
    // Close Create Dialog
    fireEvent.click(screen.getByRole("button", { name: "cancel" }));
    await waitFor(() => expect(screen.queryByText("createTitle")).toBeNull());

    // Edit failure
    const editBtn = (await screen.findAllByTestId("EditIcon"))[0].closest("button")!;
    fireEvent.click(editBtn);
    fireEvent.change(screen.getByLabelText(/name/i), { target: { value: "Edit Failure" } });
    fireEvent.click(document.querySelector('button[type="submit"]')!);
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.edit_error");
    });
    // Close Edit Dialog
    fireEvent.click(screen.getByRole("button", { name: "cancel" }));
    await waitFor(() => expect(screen.queryByText("editTitle")).toBeNull());

    // Token failure
    const testBtn = (await screen.findAllByTestId("ScienceIcon"))[0].closest("button")!;
    fireEvent.click(testBtn);
    fireEvent.change(screen.getByLabelText(/secretLabel/i), { target: { value: "secret" } });
    fireEvent.click(screen.getByRole("button", { name: "getToken" }));
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.invalid_client");
    });
    // Close Test Dialog
    fireEvent.click(screen.getByRole("button", { name: "done" }));
    await waitFor(() => expect(screen.queryByText("testSubtitle")).toBeNull());
  });

  it("covers undefined MFA prompt returns", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    const statusSpy = vi.spyOn(api.admin, "setServiceAccountStatus").mockRejectedValue(new ApiError("mfa_required"));
    const stepUpSpy = vi.spyOn(api, "mfaStepUp");

    mockRunWithStepUp.mockImplementationOnce(async (action: any) => action());

    render(<ServiceAccountsPage />);

    const statusChip = await screen.findByText("active");
    fireEvent.click(statusChip);

    await waitFor(() => {
      expect(statusSpy).toHaveBeenCalledTimes(1);
    });
    expect(stepUpSpy).not.toHaveBeenCalled();
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("uses fallback probe target when probing with an unknown key and no test error", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listServiceAccounts").mockResolvedValue(getMockAccounts());
    vi.spyOn(api.admin, "createServiceToken").mockResolvedValue({
      access_token: "fallback.payload.signature",
      token_type: "Bearer",
      expires_in: 60,
    });
    const probeSpy = vi.spyOn(api.admin, "probeWithServiceToken").mockResolvedValue({
      ok: true,
      status: 200,
      statusText: "OK",
      body: {},
    });
    vi.stubGlobal("atob", vi.fn().mockReturnValue(JSON.stringify({ sub: "sa1", scopes: [] })));

    render(<ServiceAccountsPage />);

    const testBtn = (await screen.findAllByTestId("ScienceIcon"))[0].closest("button")!;
    fireEvent.click(testBtn);

    fireEvent.change(screen.getByLabelText(/secretLabel/i), { target: { value: "secret" } });
    fireEvent.click(screen.getByRole("button", { name: "getToken" }));
    await waitFor(() => {
      expect(screen.getByText("tokenReady")).toBeDefined();
    });

    // Override target Select value manually to target invalid option
    const selectEl = document.querySelector('input[value="serviceAccounts"]') as HTMLInputElement;
    fireEvent.change(selectEl, { target: { value: "missing" } });

    fireEvent.click(screen.getByRole("button", { name: "runProbe" }));

    await waitFor(() => {
      expect(probeSpy).toHaveBeenCalledWith("/api/v1/admin/service-accounts?limit=1&offset=0", "fallback.payload.signature");
    });
  });
});
