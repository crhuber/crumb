CREATE TABLE vaults (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    current_version   INTEGER NOT NULL DEFAULT 0,
    blob              BLOB,
    updated_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    created_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE devices (
    id            TEXT PRIMARY KEY,
    vault_id      TEXT NOT NULL REFERENCES vaults(id) ON DELETE CASCADE,
    public_key    TEXT NOT NULL,
    fingerprint   TEXT NOT NULL,
    label         TEXT,
    status        TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','revoked')),
    role          TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('owner','member')),
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    approved_at   TEXT,
    revoked_at    TEXT,
    UNIQUE (vault_id, fingerprint)
);
CREATE INDEX idx_devices_vault ON devices(vault_id);

CREATE TABLE invites (
    id                TEXT PRIMARY KEY,
    vault_id          TEXT NOT NULL REFERENCES vaults(id) ON DELETE CASCADE,
    code_hash         TEXT NOT NULL,
    max_uses          INTEGER NOT NULL DEFAULT 1,
    use_count         INTEGER NOT NULL DEFAULT 0,
    expires_at        TEXT NOT NULL,
    created_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    revoked_at        TEXT
);
CREATE INDEX idx_invites_vault ON invites(vault_id);

CREATE TABLE auth_challenges (
    id           TEXT PRIMARY KEY,
    vault_id     TEXT NOT NULL REFERENCES vaults(id) ON DELETE CASCADE,
    device_id    TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    nonce        TEXT NOT NULL,
    expires_at   TEXT NOT NULL,
    used_at      TEXT
);
CREATE INDEX idx_challenges_device ON auth_challenges(device_id);

CREATE TABLE sessions (
    token_hash    TEXT PRIMARY KEY,
    vault_id      TEXT NOT NULL REFERENCES vaults(id) ON DELETE CASCADE,
    device_id     TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    expires_at    TEXT NOT NULL
);
CREATE INDEX idx_sessions_device ON sessions(device_id);
