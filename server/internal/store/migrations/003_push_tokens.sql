CREATE TABLE IF NOT EXISTS device_push_tokens (
    device_id UUID NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider TEXT NOT NULL CHECK (provider = 'apns'),
    environment TEXT NOT NULL CHECK (environment IN ('sandbox', 'production')),
    token TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, provider),
    UNIQUE (provider, environment, token),
    FOREIGN KEY (user_id, device_id)
        REFERENCES devices(user_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS device_push_tokens_user_idx
    ON device_push_tokens (user_id);
