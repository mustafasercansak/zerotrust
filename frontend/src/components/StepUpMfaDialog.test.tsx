import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, opts?: any) => (opts?.defaultValue ? `${key}||${opts.defaultValue}` : key),
  }),
}));

import { StepUpMfaDialog } from "./StepUpMfaDialog";

describe("StepUpMfaDialog", () => {
  const onSubmit = vi.fn();
  const onClose = vi.fn();

  beforeEach(() => {
    onSubmit.mockClear();
    onClose.mockClear();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("renders title, description and action buttons", () => {
    render(<StepUpMfaDialog open={true} onSubmit={onSubmit} onClose={onClose} />);
    expect(screen.getByText("stepUpMfa.title")).toBeDefined();
    expect(screen.getByText("stepUpMfa.description")).toBeDefined();
    expect(screen.getByText("stepUpMfa.verify")).toBeDefined();
    expect(screen.getByText("cancel")).toBeDefined();
  });

  it("renders nothing when open=false", () => {
    render(<StepUpMfaDialog open={false} onSubmit={onSubmit} onClose={onClose} />);
    expect(screen.queryByText("stepUpMfa.title")).toBeNull();
  });

  it("handleSubmit: does not call onSubmit when code is empty", () => {
    render(<StepUpMfaDialog open={true} onSubmit={onSubmit} onClose={onClose} />);
    // Submit button is disabled when code is empty, so submit the form directly
    const form = document.querySelector("form")!;
    fireEvent.submit(form);
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("handleSubmit: does not call onSubmit when code is whitespace only", () => {
    render(<StepUpMfaDialog open={true} onSubmit={onSubmit} onClose={onClose} />);
    const input = screen.getByRole("textbox");
    fireEvent.change(input, { target: { value: "   " } });
    const form = document.querySelector("form")!;
    fireEvent.submit(form);
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("handleSubmit: calls onSubmit with trimmed code and clears code", () => {
    render(<StepUpMfaDialog open={true} onSubmit={onSubmit} onClose={onClose} />);
    const input = screen.getByRole("textbox");
    fireEvent.change(input, { target: { value: " 123456 " } });
    const form = document.querySelector("form")!;
    fireEvent.submit(form);
    expect(onSubmit).toHaveBeenCalledWith("123456");
    expect((input as HTMLInputElement).value).toBe("");
  });

  it("handleClose via cancel button: clears code and calls onClose", () => {
    render(<StepUpMfaDialog open={true} onSubmit={onSubmit} onClose={onClose} />);
    const input = screen.getByRole("textbox");
    fireEvent.change(input, { target: { value: "555" } });
    fireEvent.click(screen.getByText("cancel"));
    expect(onClose).toHaveBeenCalled();
    expect((input as HTMLInputElement).value).toBe("");
  });

  it("shows error helperText when error prop is provided", () => {
    render(<StepUpMfaDialog open={true} onSubmit={onSubmit} onClose={onClose} error="invalid_code" />);
    expect(screen.getByText("errors.invalid_code||stepUpMfa.invalidCode")).toBeDefined();
  });

  it("disables input and buttons when loading=true", () => {
    render(<StepUpMfaDialog open={true} onSubmit={onSubmit} onClose={onClose} loading={true} />);
    const input = screen.getByRole("textbox");
    expect((input as HTMLInputElement).disabled).toBe(true);
    // Verify button should be disabled
    const buttons = screen.getAllByRole("button");
    const verifyButton = buttons.find(b => b.textContent === "stepUpMfa.verify");
    expect(verifyButton?.hasAttribute("disabled")).toBe(true);
  });
});
