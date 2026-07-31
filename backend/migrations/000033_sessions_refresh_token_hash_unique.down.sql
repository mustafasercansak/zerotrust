DROP INDEX idx_sessions_token_hash;
CREATE INDEX idx_sessions_token_hash ON sessions(refresh_token_hash);
