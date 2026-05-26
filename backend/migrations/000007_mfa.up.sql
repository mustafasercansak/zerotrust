CREATE TABLE user_mfa (
    user_id          UUID        PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    totp_secret_enc  TEXT        NOT NULL,
    enabled_at       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
