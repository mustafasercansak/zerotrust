import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { render, screen, fireEvent, cleanup, waitFor } from "@testing-library/react";
import OidcClientsSection from "./OidcClientsSection";
import { api } from "@/lib/api";
import { toast } from "sonner";

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));

const mockT = (key: string, options?: any) => {
  if (options?.name) return `${key}:${options.name}`;
  return key;
};

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: mockT,
    i18n: { language: "en" },
  }),
}));

// Mock MUI X DataGrid to render a simple table so we can test it under JSDOM without row virtualization
vi.mock("@mui/x-data-grid", () => ({
  DataGrid: (props: any) => {
    return (
      <div data-testid="datagrid">
        {props.rows?.map((row: any) => (
          <div key={row.id} data-testid="datagrid-row">
            {props.columns?.map((col: any) => {
              if (col.renderCell) {
                return (
                  <div key={col.field} data-testid={`cell-${col.field}`}>
                    {col.renderCell({ row, value: row[col.field], rowId: row.id, rowNode: row })}
                  </div>
                );
              }
              return (
                <div key={col.field} data-testid={`cell-${col.field}`}>
                  {row[col.field]}
                </div>
              );
            })}
          </div>
        ))}
      </div>
    );
  },
  GridToolbar: () => <div />,
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

vi.mock("@/components/SecretDisplayCard", () => ({
  SecretDisplayCard: (props: any) => {
    return (
      <div data-testid="secret-card">
        <span>{props.label}</span>
        <span>{props.value}</span>
        <button
          data-testid={`copy-${props.label}`}
          onClick={() => {
            navigator.clipboard.writeText(props.value);
            toast.success(props.successMessage || "Copied");
          }}
        >
          Copy
        </button>
      </div>
    );
  },
}));

describe("OidcClientsSection", () => {
  const confirmMock = vi.fn().mockReturnValue(true);
  const writeTextMock = vi.fn();

  beforeEach(() => {
    confirmMock.mockClear();
    confirmMock.mockReturnValue(true);
    writeTextMock.mockClear();
    mockRunWithStepUp.mockClear();
    mockRunWithStepUp.mockImplementation(async (action: any) => action());

    vi.spyOn(window, "confirm").mockImplementation(confirmMock);
    vi.stubGlobal("navigator", { clipboard: { writeText: writeTextMock } });
  });

  afterEach(() => {
    cleanup();
    document.body.innerHTML = "";
    document.body.removeAttribute("style");
    document.body.removeAttribute("class");
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("renders loader during loading state", async () => {
    vi.spyOn(api.admin, "listOidcClients").mockReturnValue(new Promise(() => {}));
    render(<OidcClientsSection />);
    expect(screen.getByText("loading")).toBeDefined();
  });

  it("renders DataGrid when clients are loaded", async () => {
    const listSpy = vi.spyOn(api.admin, "listOidcClients").mockResolvedValue([
      {
        id: "c1",
        client_id: "test-client-id",
        name: "Test Client",
        redirect_uris: ["http://localhost/callback"],
        allowed_scopes: ["openid", "profile"],
        created_at: "2026-06-06T12:00:00Z",
      },
    ]);

    render(<OidcClientsSection />);

    await waitFor(() => {
      expect(screen.getByText("Test Client")).toBeDefined();
      expect(screen.getByText("test-client-id")).toBeDefined();
    });
    expect(listSpy).toHaveBeenCalled();
  });

  it("handles client edit update flow", async () => {
    vi.spyOn(api.admin, "listOidcClients").mockResolvedValue([
      {
        id: "c1",
        client_id: "test-client-id",
        name: "Old Client Name",
        redirect_uris: ["http://old/callback"],
        allowed_scopes: ["openid"],
        created_at: "2026-06-06T12:00:00Z",
      },
    ]);

    const updateSpy = vi.spyOn(api.admin, "updateOidcClient").mockResolvedValue({} as any);

    render(<OidcClientsSection />);

    await waitFor(() => {
      expect(screen.getByText("Old Client Name")).toBeDefined();
    });

    const editBtn = screen.getByTestId("EditIcon");
    fireEvent.click(editBtn);

    const nameInput = screen.getByLabelText(/oidc.name/i);
    const redirectInput = screen.getByLabelText(/oidc.redirectUris/i);
    const scopeInput = screen.getByLabelText(/oidc.allowedScopes/i);

    fireEvent.change(nameInput, { target: { value: "Updated Client Name" } });
    fireEvent.change(redirectInput, { target: { value: "http://new/callback" } });
    fireEvent.change(scopeInput, { target: { value: "openid profile" } });

    const saveBtn = screen.getByRole("button", { name: "save" });
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(updateSpy).toHaveBeenCalledWith("c1", {
        name: "Updated Client Name",
        redirect_uris: ["http://new/callback"],
        allowed_scopes: ["openid", "profile"],
      });
      expect(toast.success).toHaveBeenCalledWith("saved");
    });
  });

  it("handles create client form submission", async () => {
    vi.spyOn(api.admin, "listOidcClients").mockResolvedValue([]);
    const createSpy = vi.spyOn(api.admin, "createOidcClient").mockResolvedValue({
      id: "new-c",
      client_id: "new-app-id",
      client_secret: "generated-secret",
      name: "New App",
      redirect_uris: ["http://localhost/callback"],
      allowed_scopes: ["openid", "profile"],
      created_at: "2026-06-06T12:00:00Z",
    });

    render(<OidcClientsSection />);

    await waitFor(() => {
      expect(screen.getByText("oidc.title")).toBeDefined();
    });

    const createBtn = screen.getByRole("button", { name: "oidc.create" });
    fireEvent.click(createBtn);

    const nameInput = screen.getByLabelText(/oidc.name/i);
    const clientIdInput = screen.getByLabelText(/oidc.clientId/i);
    const redirectInput = screen.getByLabelText(/oidc.redirectUris/i);
    const scopeInput = screen.getByLabelText(/oidc.allowedScopes/i);

    fireEvent.change(nameInput, { target: { value: "New App" } });
    fireEvent.change(clientIdInput, { target: { value: "new-app-id" } });
    fireEvent.change(redirectInput, { target: { value: "http://localhost/callback" } });
    fireEvent.change(scopeInput, { target: { value: "openid profile" } });

    const submitBtn = screen.getByRole("button", { name: "oidc.create" });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(createSpy).toHaveBeenCalledWith({
        client_id: "new-app-id",
        name: "New App",
        redirect_uris: ["http://localhost/callback"],
        allowed_scopes: ["openid", "profile"],
      });
    });

    const doneBtn = screen.getByRole("button", { name: "oidc.done" });
    fireEvent.click(doneBtn);
  });

  it("shows rotated secret after rotating secret", async () => {
    vi.spyOn(api.admin, "listOidcClients").mockResolvedValue([
      {
        id: "c1",
        client_id: "rotate-client",
        name: "Rotate Test",
        redirect_uris: [],
        allowed_scopes: ["openid"],
        created_at: "2026-06-06T12:00:00Z",
      },
    ]);

    const rotateSpy = vi.spyOn(api.admin, "rotateOidcClientSecret").mockResolvedValue({
      client_secret: "new-rotated-secret-xyz",
    });

    render(<OidcClientsSection />);

    await waitFor(() => {
      expect(screen.getByText("Rotate Test")).toBeDefined();
    });

    const rotateBtn = screen.getByTestId("AutorenewIcon");
    fireEvent.click(rotateBtn);

    expect(confirmMock).toHaveBeenCalled();
    await waitFor(() => {
      expect(rotateSpy).toHaveBeenCalledWith("c1");
    });

    await waitFor(() => {
      expect(screen.getByText(/new-rotated-secret-xyz/)).toBeDefined();
      expect(screen.getAllByText(/rotate-client/).length).toBeGreaterThan(0);
    });

    const doneBtn = screen.getByRole("button", { name: "oidc.done" });
    fireEvent.click(doneBtn);
  });

  it("handles copy to clipboard", async () => {
    vi.spyOn(api.admin, "listOidcClients").mockResolvedValue([
      {
        id: "c1",
        client_id: "copied-client-id",
        name: "Test",
        redirect_uris: [],
        allowed_scopes: [],
        created_at: "2026-06-06T12:00:00Z",
      },
    ]);

    render(<OidcClientsSection />);

    await waitFor(() => {
      expect(screen.getByText("copied-client-id")).toBeDefined();
    });

    const copyBtn = screen.getByTestId("ContentCopyIcon");
    fireEvent.click(copyBtn);

    expect(writeTextMock).toHaveBeenCalledWith("copied-client-id");
    expect(toast.success).toHaveBeenCalledWith("oidc.copied");
  });
});
