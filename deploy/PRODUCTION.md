# Production deployment

## Layout

- Root: `/Volumes/SSD_ZHITAI/my-cloudflared-app`
- Server source: `share_clipboard/server`
- PostgreSQL data: `share_clipboard/postgres`
- Secrets and runtime environment: `share_clipboard/.env` (mode `0600`)
- Nginx path route: `conf/conf.d/default.conf`
- Reserved subdomain route: `conf/conf.d/share_clipboard.conf`

The active public base URL is `https://zhy.hair/fastcopy`. The Nginx path proxy
removes `/fastcopy` before forwarding requests, including the WebSocket route.
`clip.zhy.hair` is already configured in Nginx but still needs a Cloudflare
Tunnel Public Hostname before it can be used.

## Containers

- `share_clipboard_postgres`: PostgreSQL 17, internal Docker network only
- `share_clipboard_server`: read-only filesystem, all capabilities dropped

The production environment sets `FASTCOPY_MAX_USERS=1`. The first account can
be created through the unified authentication endpoint; subsequent requests
with that account log in automatically.

The APNs token API and `device_push_tokens` table are deployed. APNs sending
currently remains disabled until an Apple token-signing key is added. Fresh
databases are created from one complete baseline schema; no legacy data
migration is retained.

## Enable APNs

1. Put the Apple `.p8` key outside the Git repository, for example at
   `share_clipboard/secrets/apns-auth-key.p8`, and restrict its permissions.
2. Mount it read-only into `share_clipboard_server` as
   `/run/secrets/apns-auth-key.p8`.
3. Add the following values to `share_clipboard/.env`:

```dotenv
FASTCOPY_APNS_ENABLED=true
FASTCOPY_APNS_KEY_ID=<Apple Key ID>
FASTCOPY_APNS_TEAM_ID=<Apple Team ID>
FASTCOPY_APNS_BUNDLE_ID=hair.zhy.fastcopy.ios
FASTCOPY_APNS_PRIVATE_KEY_PATH=/run/secrets/apns-auth-key.p8
```

Then recreate the service and check that its startup log says
`APNs notifications enabled`. Never commit the `.p8` key or production `.env`.

## Operations

Run these commands from `/Volumes/SSD_ZHITAI/my-cloudflared-app`:

```bash
docker compose up -d --build share_clipboard_server
docker compose ps share_clipboard_server share_clipboard_postgres
docker compose logs --tail=100 share_clipboard_server
curl -fsS https://zhy.hair/fastcopy/healthz
```

Before deployment, server source is synchronized from this repository:

```bash
rsync -a --delete --exclude integration server/ \
  /Volumes/SSD_ZHITAI/my-cloudflared-app/share_clipboard/server/
```

Backups created before the initial deployment are named
`docker-compose.yml.pre-fastcopy-20260818`,
`conf/nginx.conf.pre-fastcopy-20260818`, and
`conf/conf.d/default.conf.pre-fastcopy-20260818`.
