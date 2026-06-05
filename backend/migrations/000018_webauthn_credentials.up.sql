-- WebAuthn / FIDO2 passkey credentials, used as a phishing-resistant second
-- factor alongside TOTP. One row per registered authenticator. (ISSUE_LIST: WebAuthn)
CREATE TABLE user_webauthn_credentials (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- Base64url-encoded raw credential ID; unique across all users.
    credential_id  TEXT        NOT NULL UNIQUE,
    -- Full go-webauthn Credential serialized as JSON for lossless reconstruction.
    data           JSONB       NOT NULL,
    -- Signature counter, mirrored from data for clone-detection visibility.
    sign_count     BIGINT      NOT NULL DEFAULT 0,
    -- User-friendly label (e.g. "YubiKey 5", "iPhone").
    name           VARCHAR(100) NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at   TIMESTAMPTZ
);

CREATE INDEX idx_webauthn_credentials_user_id ON user_webauthn_credentials(user_id);
