import { describe, expect, it } from "vitest";
import React from "react";
import { MeContext, useMeContext } from "./MeContext";
import { renderToString } from "react-dom/server";

describe("MeContext", () => {
  it("provides null by default", () => {
    let contextValue: any = undefined;
    function TestComponent() {
      contextValue = useMeContext();
      return null;
    }
    renderToString(React.createElement(TestComponent));
    expect(contextValue).toBeNull();
  });

  it("provides the value from MeContext.Provider", () => {
    let contextValue: any = undefined;
    const mockMeData = {
      user: {
        id: "123",
        email: "test@example.com",
        first_name: "John",
        last_name: "Doe",
        is_active: true,
        locale: "en",
        roles: ["user"],
        created_at: "2026-06-04T12:00:00Z",
        updated_at: "2026-06-04T12:00:00Z",
      },
      loading: false,
      refetch: async () => {},
    } as any;

    function TestComponent() {
      contextValue = useMeContext();
      return null;
    }

    renderToString(
      React.createElement(
        MeContext.Provider,
        { value: mockMeData },
        React.createElement(TestComponent)
      )
    );
    expect(contextValue).toBe(mockMeData);
  });
});
