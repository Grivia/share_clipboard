# 粘贴板助手协议 v1

All API requests use JSON over HTTPS. Native clients send the access token in
`Authorization: Bearer <token>`. Clipboard plaintext is encrypted by clients
with AES-256-GCM before it leaves a device.

The current production base URL is `https://zhy.hair/fastcopy`.

## Device identity

Each installation creates a random UUID named `installation_id` and keeps it
in secure local storage. The server creates its own `device_id` after login.
Device names are informational and never identify a device.

## Authentication

- `POST /v1/auth/session`
- `POST /v1/auth/refresh`
- `POST /v1/auth/logout`

The session endpoint accepts an account, password, and device object:

```json
{
  "account": "rivia",
  "password": "correct horse battery staple",
  "device": {
    "installation_id": "UUID",
    "reported_name": "Rivia's Mac mini",
    "platform": "macos",
    "os_version": "15.6.1",
    "app_version": "0.2.0"
  }
}
```

If the exact account exists, the endpoint verifies the password and returns
HTTP 200. Otherwise it creates the account and returns HTTP 201. Accounts are
case-sensitive, have surrounding whitespace removed, and may contain 1 to 128
Unicode characters except control characters. Passwords may contain 4 to 256
Unicode characters and are otherwise unrestricted.

The production deployment allows one account. Registration remains serialized
so simultaneous first requests for the same account safely converge.

## Local key derivation

After a successful session response, every client derives the clipboard key
locally from `response.user.account` and the submitted password:

```text
salt = SHA-256(UTF-8("fastcopy:key-salt:v1|" + canonical_account))
key  = PBKDF2-HMAC-SHA256(UTF-8(password), salt, 210000, 32 bytes)
```

The 32-byte result is the AES-256-GCM key. It is stored in Keychain on macOS or
the private `0600` runtime file in the KernelSU module. The derived key is never
sent to the server or exposed in client settings. A future password-change
protocol must re-encrypt or explicitly rotate clipboard state because changing
the password changes this key.

## Devices

- `GET /v1/devices`
- `PATCH /v1/devices/<device_id>` with `{"name":"..."}`
- `PATCH /v1/devices/<device_id>/role` with `{"role":"admin"}` or `{"role":"member"}`
- `POST /v1/devices/<device_id>/revoke`

The list includes historical devices plus current `logged_in` and `online`
flags. Each item also contains `role`, `can_revoke`, and `can_change_role`.
Clients use the capability fields to render actions, while the server always
enforces the permission again when processing a request.

The first device of a newly registered account is the unique `super_admin`.
Later devices start as `member`. The super admin may promote a member to
`admin`, demote an admin to member, and revoke any other active device. An
admin may revoke a member or another admin, but cannot revoke the super admin
or change roles. A member cannot manage other devices. No role can revoke its
own current device through the device endpoint; every device can still use the
normal logout endpoint for itself.

Revoking a device invalidates all of its sessions and closes its active
WebSocket connections. This is a forced sign-out, so the device can sign in
again with the account password; its assigned role is retained.

## Encryption envelope

For each logical clipboard change, the client:

1. Creates a random UUID `client_event_id`.
2. Creates a random 12-byte nonce.
3. Encrypts UTF-8 bytes with AES-256-GCM.
4. Uses the UTF-8 string below as authenticated additional data:

```text
fastcopy:v1|<client_event_id>|text/plain
```

5. Persists the complete encrypted request locally before sending it.

Retries reuse the same event ID, nonce, and ciphertext.

## Clipboard API

- `POST /v1/clips`
- `GET /v1/clips?after_seq=0&limit=100`
- `POST /v1/sync/ack`

Upload body:

```json
{
  "client_event_id": "UUID",
  "content_type": "text/plain",
  "algorithm": "AES-256-GCM",
  "nonce": "base64",
  "ciphertext": "base64"
}
```

The idempotency scope is `(origin_device_id, client_event_id)`. Reusing this
pair with a different encrypted payload returns HTTP 409.

Logging in again with the same `installation_id` keeps the same server-side
device and therefore the same idempotency scope. Reinstalling normally creates
a new `installation_id`; the same accidental client event UUID on that new
device is a different pair and can be accepted as a new event.

## WebSocket

Connect to `GET /v1/events/ws` with the access token header. Events are JSON:

```json
{
  "type": "clip.created",
  "data": {
    "event_id": "server-event-uuid",
    "client_event_id": "client-event-uuid",
    "seq": 42,
    "origin_device_id": "device-uuid",
    "origin_name": "MacBook Pro",
    "content_type": "text/plain",
    "algorithm": "AES-256-GCM",
    "nonce": "base64",
    "ciphertext": "base64",
    "created_at": "2026-08-19T10:00:00Z",
    "expires_at": "2026-08-26T10:00:00Z"
  }
}
{"type":"device.presence_changed","data":{}}
{"type":"device.logged_in","data":{}}
{"type":"device.revoked","data":{}}
{"type":"device.updated","data":{}}
```

For `clip.created`, `data` is the same encrypted event envelope returned by the
clipboard REST API. A client may decrypt and apply it directly only when `seq`
equals its persisted cursor plus one. Missing or malformed data, a sequence gap,
and every reconnect trigger REST reconciliation with the persisted `after_seq`
cursor. WebSocket delivery remains opportunistic; REST is the recovery path.

## APNs device tokens

Only authenticated devices whose platform is `ios` may register an APNs token:

- `PUT /v1/push-tokens/apns`
- `DELETE /v1/push-tokens/apns`

Registration body:

```json
{
  "token": "lowercase-hex-device-token",
  "environment": "sandbox"
}
```

`environment` is `sandbox` for development builds and `production` for App
Store, TestFlight, Ad Hoc, and other distribution builds. A new token replaces
the previous token for that device. Clients register with APNs on every launch
and upload the token returned by the operating system instead of treating a
locally cached token as authoritative.

The server sends an alert notification with `content-available: 1`. Its custom
payload includes only `type`, `seq`, and `origin_device_id`; it never includes
clipboard plaintext, ciphertext, account names, or credentials. Delivery is a
best-effort wake-up signal. The persisted event cursor remains the source of
truth.
