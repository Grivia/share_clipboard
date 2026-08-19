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

## Device roles

The first device registered for an account is its unique `super_admin`; later
devices are `member` by default. The super admin can promote or demote other
devices and force them offline. An `admin` can force members and peer admins
offline, but cannot manage roles or revoke the super admin. Members have no
remote device-management permission.

`GET /v1/devices` returns `role`, `can_revoke`, and `can_change_role` for each
target device. Role changes use `PATCH /v1/devices/<device_id>/role`. Capability
fields are only UI guidance: role checks and row locks are applied again inside
the database transaction. Migration `004_device_roles.sql` promotes the oldest
non-revoked device in every existing account, falling back to its oldest
historical device.

## iOS push notifications

The iOS client uploads its APNs device token with
`PUT /v1/push-tokens/apns`. Tokens are bound to the authenticated iOS device
and removed automatically when the device is revoked or APNs reports a
permanently invalid token.

APNs is disabled by default. To enable it, mount an Apple token-signing `.p8`
file into the container and configure all of the following variables:

```sh
FASTCOPY_APNS_ENABLED=true
FASTCOPY_APNS_KEY_ID=ABC123DEFG
FASTCOPY_APNS_TEAM_ID=0123456789
FASTCOPY_APNS_BUNDLE_ID=hair.zhy.fastcopy.ios
FASTCOPY_APNS_PRIVATE_KEY_PATH=/run/secrets/apns-auth-key.p8
```

Push payloads contain only generic UI text and event metadata. Clipboard
plaintext and ciphertext are not sent through APNs; the client uses the push
as a wake-up signal and fetches encrypted events through the normal cursor API.
