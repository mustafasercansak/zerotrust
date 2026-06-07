import { describe, expect, it, vi } from "vitest";
import { requiredValidator } from "./validation";

describe("validation helper", () => {
  it("sets empty validity when value is present", () => {
    const el = { setCustomValidity: vi.fn() } as unknown as HTMLInputElement;
    const validator = requiredValidator("some text", "Field is required");
    validator(el);
    expect(el.setCustomValidity).toHaveBeenCalledWith("");
  });

  it("sets error validity when value is empty or whitespace", () => {
    const el = { setCustomValidity: vi.fn() } as unknown as HTMLInputElement;
    const validator = requiredValidator("  ", "Field is required");
    validator(el);
    expect(el.setCustomValidity).toHaveBeenCalledWith("Field is required");
  });
});
