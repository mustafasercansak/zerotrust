-- Distinguishes WHY a session was revoked, so refresh-token reuse detection
-- (session.CheckReuse) can tell a rotated-and-superseded token (real evidence
-- of replay/theft) apart from a token that was revoked by logout, an admin
-- action, or session cleanup — replaying any of those today falsely triggers
-- a mass revocation of the victim's other sessions. (ISSUE_LIST #98)
ALTER TABLE sessions ADD COLUMN revoked_reason TEXT NULL;
