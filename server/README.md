# 粘贴板助手服务端

The server is a Go HTTP/WebSocket service backed by PostgreSQL. It stores
authentication state, device history, presence timestamps, idempotency keys,
and encrypted clipboard envelopes.

## Run locally

```sh
cp .env.example .env
docker compose -f ../deploy/docker-compose.dev.yml up --build
```

Create a session with `POST /v1/auth/session`, then use the returned access
token for device and clipboard APIs. A missing account is registered
automatically; an existing account is signed in. The service runs migrations
at startup.

Clipboard payloads must already be AES-256-GCM encrypted. See
`../shared/API.md` for the protocol.

`FASTCOPY_MAX_USERS=0` allows unlimited accounts. Set it to `1` for a personal
deployment: registration is serialized with a PostgreSQL advisory lock, so
only the first account can be created even when requests arrive concurrently.
Concurrent first requests for the same account converge on the same account.
