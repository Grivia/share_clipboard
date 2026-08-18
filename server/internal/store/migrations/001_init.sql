CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    email TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS users_email_unique ON users (lower(email));

CREATE TABLE IF NOT EXISTS devices (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    installation_id UUID NOT NULL,
    reported_name TEXT NOT NULL,
    custom_name TEXT NOT NULL DEFAULT '',
    platform TEXT NOT NULL,
    os_version TEXT NOT NULL DEFAULT '',
    app_version TEXT NOT NULL DEFAULT '',
    first_login_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    UNIQUE (user_id, installation_id),
    UNIQUE (user_id, id)
);

CREATE TABLE IF NOT EXISTS auth_sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    device_id UUID NOT NULL,
    access_token_hash BYTEA NOT NULL UNIQUE,
    refresh_token_hash BYTEA NOT NULL UNIQUE,
    access_expires_at TIMESTAMPTZ NOT NULL,
    refresh_expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    FOREIGN KEY (user_id, device_id) REFERENCES devices(user_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS auth_sessions_device_active_idx
    ON auth_sessions (device_id) WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS login_events (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
    email_hash BYTEA NOT NULL,
    success BOOLEAN NOT NULL,
    remote_ip TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE SEQUENCE IF NOT EXISTS clipboard_event_seq;

CREATE TABLE IF NOT EXISTS clip_idempotency (
    origin_device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    client_event_id UUID NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_id UUID NOT NULL UNIQUE,
    seq BIGINT NOT NULL UNIQUE,
    request_digest BYTEA NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (origin_device_id, client_event_id)
);

CREATE TABLE IF NOT EXISTS clipboard_events (
    event_id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    origin_device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    client_event_id UUID NOT NULL,
    seq BIGINT NOT NULL UNIQUE,
    content_type TEXT NOT NULL CHECK (content_type = 'text/plain'),
    algorithm TEXT NOT NULL CHECK (algorithm = 'AES-256-GCM'),
    nonce BYTEA NOT NULL CHECK (octet_length(nonce) = 12),
    ciphertext BYTEA NOT NULL CHECK (octet_length(ciphertext) >= 16),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    UNIQUE (origin_device_id, client_event_id)
);

CREATE INDEX IF NOT EXISTS clipboard_events_user_seq_idx
    ON clipboard_events (user_id, seq);
CREATE INDEX IF NOT EXISTS clipboard_events_expiry_idx
    ON clipboard_events (expires_at);

CREATE TABLE IF NOT EXISTS device_cursors (
    device_id UUID PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
    last_acked_seq BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
