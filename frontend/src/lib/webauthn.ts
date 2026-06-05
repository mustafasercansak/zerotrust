// WebAuthn browser glue: converts the base64url-encoded options the server
// sends into the ArrayBuffers navigator.credentials expects, and serializes the
// authenticator's response back into the JSON shape go-webauthn parses.

export interface PublicKeyCredentialDescriptorJSON {
  type: "public-key";
  id: string; // base64url
  transports?: string[];
}

export interface CredentialCreationOptionsJSON {
  publicKey: {
    challenge: string; // base64url
    rp: { id?: string; name: string };
    user: { id: string; name: string; displayName: string }; // id base64url
    pubKeyCredParams: Array<{ type: "public-key"; alg: number }>;
    timeout?: number;
    excludeCredentials?: PublicKeyCredentialDescriptorJSON[];
    authenticatorSelection?: Record<string, unknown>;
    attestation?: string;
    extensions?: Record<string, unknown>;
  };
}

export interface CredentialRequestOptionsJSON {
  publicKey: {
    challenge: string; // base64url
    timeout?: number;
    rpId?: string;
    allowCredentials?: PublicKeyCredentialDescriptorJSON[];
    userVerification?: string;
    extensions?: Record<string, unknown>;
  };
}

/** Decode a base64url string into an ArrayBuffer. */
export function base64urlToBuffer(value: string): ArrayBuffer {
  const padded = value.replace(/-/g, "+").replace(/_/g, "/");
  const pad = padded.length % 4 === 0 ? "" : "=".repeat(4 - (padded.length % 4));
  const binary = atob(padded + pad);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes.buffer;
}

/** Encode an ArrayBuffer as an unpadded base64url string. */
export function bufferToBase64url(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = "";
  for (let i = 0; i < bytes.length; i += 1) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function mapDescriptors(list?: PublicKeyCredentialDescriptorJSON[]): PublicKeyCredentialDescriptor[] | undefined {
  if (!list) return undefined;
  return list.map((d) => ({
    type: "public-key",
    id: base64urlToBuffer(d.id),
    transports: d.transports as AuthenticatorTransport[] | undefined,
  }));
}

/** True when this browser exposes the WebAuthn API. */
export function isWebAuthnSupported(): boolean {
  return typeof window !== "undefined" && !!window.PublicKeyCredential && !!navigator.credentials;
}

/**
 * Run a registration ceremony: prompt the authenticator and return the
 * attestation response serialized for the server.
 */
export async function performRegistration(options: CredentialCreationOptionsJSON): Promise<Record<string, unknown>> {
  const pk = options.publicKey;
  const publicKey: PublicKeyCredentialCreationOptions = {
    ...(pk as unknown as PublicKeyCredentialCreationOptions),
    challenge: base64urlToBuffer(pk.challenge),
    user: {
      ...pk.user,
      id: base64urlToBuffer(pk.user.id),
    },
    excludeCredentials: mapDescriptors(pk.excludeCredentials),
  };

  const credential = (await navigator.credentials.create({ publicKey })) as PublicKeyCredential | null;
  if (!credential) {
    throw new Error("registration_cancelled");
  }
  const response = credential.response as AuthenticatorAttestationResponse;
  return {
    id: credential.id,
    rawId: bufferToBase64url(credential.rawId),
    type: credential.type,
    response: {
      attestationObject: bufferToBase64url(response.attestationObject),
      clientDataJSON: bufferToBase64url(response.clientDataJSON),
    },
    clientExtensionResults: credential.getClientExtensionResults(),
  };
}

/**
 * Run an authentication (assertion) ceremony and return the signed response
 * serialized for the server.
 */
export async function performAssertion(options: CredentialRequestOptionsJSON): Promise<Record<string, unknown>> {
  const pk = options.publicKey;
  const publicKey: PublicKeyCredentialRequestOptions = {
    ...(pk as unknown as PublicKeyCredentialRequestOptions),
    challenge: base64urlToBuffer(pk.challenge),
    allowCredentials: mapDescriptors(pk.allowCredentials),
  };

  const credential = (await navigator.credentials.get({ publicKey })) as PublicKeyCredential | null;
  if (!credential) {
    throw new Error("assertion_cancelled");
  }
  const response = credential.response as AuthenticatorAssertionResponse;
  return {
    id: credential.id,
    rawId: bufferToBase64url(credential.rawId),
    type: credential.type,
    response: {
      authenticatorData: bufferToBase64url(response.authenticatorData),
      clientDataJSON: bufferToBase64url(response.clientDataJSON),
      signature: bufferToBase64url(response.signature),
      userHandle: response.userHandle ? bufferToBase64url(response.userHandle) : undefined,
    },
    clientExtensionResults: credential.getClientExtensionResults(),
  };
}
