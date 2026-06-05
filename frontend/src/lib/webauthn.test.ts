import { afterEach, describe, expect, it, vi } from "vitest";
import {
  base64urlToBuffer,
  bufferToBase64url,
  isWebAuthnSupported,
  performAssertion,
  performRegistration,
} from "./webauthn";

function bufFrom(bytes: number[]): ArrayBuffer {
  return new Uint8Array(bytes).buffer;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("base64url helpers", () => {
  it("round-trips arbitrary bytes", () => {
    const original = bufFrom([0, 1, 2, 250, 251, 255, 62, 63]);
    const encoded = bufferToBase64url(original);
    // base64url must not contain +, /, or =.
    expect(encoded).not.toMatch(/[+/=]/);
    const decoded = new Uint8Array(base64urlToBuffer(encoded));
    expect(Array.from(decoded)).toEqual([0, 1, 2, 250, 251, 255, 62, 63]);
  });

  it("decodes a known base64url value", () => {
    // "AQID" is base64 for [1,2,3].
    const decoded = new Uint8Array(base64urlToBuffer("AQID"));
    expect(Array.from(decoded)).toEqual([1, 2, 3]);
  });
});

describe("isWebAuthnSupported", () => {
  it("is false without PublicKeyCredential", () => {
    vi.stubGlobal("window", {});
    vi.stubGlobal("navigator", { credentials: {} });
    expect(isWebAuthnSupported()).toBe(false);
  });

  it("is true when the API is present", () => {
    vi.stubGlobal("window", { PublicKeyCredential: function () {} });
    vi.stubGlobal("navigator", { credentials: { create: vi.fn(), get: vi.fn() } });
    expect(isWebAuthnSupported()).toBe(true);
  });
});

describe("performRegistration", () => {
  it("decodes options and serializes the attestation response", async () => {
    const create = vi.fn().mockResolvedValue({
      id: "credid",
      rawId: bufFrom([1, 2, 3]),
      type: "public-key",
      response: {
        attestationObject: bufFrom([4, 5, 6]),
        clientDataJSON: bufFrom([7, 8, 9]),
      },
      getClientExtensionResults: () => ({}),
    });
    vi.stubGlobal("navigator", { credentials: { create } });

    const out = await performRegistration({
      publicKey: {
        challenge: "AQID", // [1,2,3]
        rp: { name: "ZeroTrust" },
        user: { id: "AQID", name: "u", displayName: "u" },
        pubKeyCredParams: [{ type: "public-key", alg: -7 }],
        excludeCredentials: [{ type: "public-key", id: "AQID" }],
      },
    });

    // The challenge and user.id passed to create() must be ArrayBuffers.
    const passedPublicKey = create.mock.calls[0][0].publicKey;
    expect(passedPublicKey.challenge).toBeInstanceOf(ArrayBuffer);
    expect(passedPublicKey.user.id).toBeInstanceOf(ArrayBuffer);
    expect(passedPublicKey.excludeCredentials[0].id).toBeInstanceOf(ArrayBuffer);

    // The serialized response uses base64url strings.
    expect(out.id).toBe("credid");
    expect(out.rawId).toBe("AQID");
    expect((out.response as any).attestationObject).toBe("BAUG"); // [4,5,6]
    expect((out.response as any).clientDataJSON).toBe("BwgJ"); // [7,8,9]
  });

  it("throws when the ceremony is cancelled", async () => {
    vi.stubGlobal("navigator", { credentials: { create: vi.fn().mockResolvedValue(null) } });
    await expect(
      performRegistration({
        publicKey: {
          challenge: "AQID",
          rp: { name: "x" },
          user: { id: "AQID", name: "u", displayName: "u" },
          pubKeyCredParams: [],
        },
      }),
    ).rejects.toThrow("registration_cancelled");
  });
});

describe("performAssertion", () => {
  it("decodes options and serializes the assertion response", async () => {
    const get = vi.fn().mockResolvedValue({
      id: "credid",
      rawId: bufFrom([1, 2, 3]),
      type: "public-key",
      response: {
        authenticatorData: bufFrom([10]),
        clientDataJSON: bufFrom([11]),
        signature: bufFrom([12]),
        userHandle: bufFrom([13]),
      },
      getClientExtensionResults: () => ({}),
    });
    vi.stubGlobal("navigator", { credentials: { get } });

    const out = await performAssertion({
      publicKey: {
        challenge: "AQID",
        allowCredentials: [{ type: "public-key", id: "AQID" }],
      },
    });

    const passedPublicKey = get.mock.calls[0][0].publicKey;
    expect(passedPublicKey.challenge).toBeInstanceOf(ArrayBuffer);
    expect(passedPublicKey.allowCredentials[0].id).toBeInstanceOf(ArrayBuffer);

    const resp = out.response as any;
    expect(resp.authenticatorData).toBe(bufferToBase64url(bufFrom([10])));
    expect(resp.signature).toBe(bufferToBase64url(bufFrom([12])));
    expect(resp.userHandle).toBe(bufferToBase64url(bufFrom([13])));
  });

  it("omits userHandle when absent", async () => {
    const get = vi.fn().mockResolvedValue({
      id: "c",
      rawId: bufFrom([1]),
      type: "public-key",
      response: {
        authenticatorData: bufFrom([1]),
        clientDataJSON: bufFrom([1]),
        signature: bufFrom([1]),
        userHandle: null,
      },
      getClientExtensionResults: () => ({}),
    });
    vi.stubGlobal("navigator", { credentials: { get } });

    const out = await performAssertion({ publicKey: { challenge: "AQID" } });
    expect((out.response as any).userHandle).toBeUndefined();
  });
});
