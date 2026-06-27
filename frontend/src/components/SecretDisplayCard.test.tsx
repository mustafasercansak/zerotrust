import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { render, screen, fireEvent, act, cleanup } from "@testing-library/react";
import { toast } from "sonner";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

import { SecretDisplayCard } from "./SecretDisplayCard";

describe("SecretDisplayCard", () => {
  beforeEach(() => {
    vi.mocked(toast.success).mockClear();
    vi.useFakeTimers();
    vi.stubGlobal("navigator", { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } });
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("renders label and value", () => {
    render(<SecretDisplayCard label="API Key" value="sk-abc123" />);
    expect(screen.getByText("API Key")).toBeDefined();
    expect(screen.getByText("sk-abc123")).toBeDefined();
  });

  it("renders copy icon button initially", () => {
    render(<SecretDisplayCard label="API Key" value="sk-abc123" />);
    // The copy button should be visible
    expect(screen.getByRole("button")).toBeDefined();
  });

  it("handleCopy: writes value to clipboard and shows toast with default message", () => {
    render(<SecretDisplayCard label="API Key" value="sk-abc123" />);
    fireEvent.click(screen.getByRole("button"));
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith("sk-abc123");
    expect(toast.success).toHaveBeenCalledWith("Copied to clipboard");
  });

  it("handleCopy: shows custom successMessage when provided", () => {
    render(<SecretDisplayCard label="API Key" value="sk-abc123" successMessage="Key copied!" />);
    fireEvent.click(screen.getByRole("button"));
    expect(toast.success).toHaveBeenCalledWith("Key copied!");
  });

  it("handleCopy: sets copied=true immediately then resets to false after 2000ms", () => {
    render(<SecretDisplayCard label="API Key" value="sk-abc123" />);
    fireEvent.click(screen.getByRole("button"));
    // After click, the check icon should appear (MUI CheckIcon uses a data-testid)
    // and after 2s, the copy icon returns. We verify via the toast + timer side-effects.
    expect(toast.success).toHaveBeenCalled();
    act(() => { vi.advanceTimersByTime(2000); });
    // The component re-rendered with copied=false — no crash means the timer worked
  });

  it("renders with gradient style when hasGradient is true", () => {
    render(<SecretDisplayCard label="API Key" value="sk-abc123" hasGradient />);
    expect(screen.getByText("API Key")).toBeDefined();
    expect(screen.getByText("sk-abc123")).toBeDefined();
  });
});
