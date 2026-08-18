# Production deployment

## Layout

- Root: `/Volumes/SSD_ZHITAI/my-cloudflared-app`
- Server source: `fastcopy/server`
- PostgreSQL data: `fastcopy/postgres`
- Secrets and runtime environment: `fastcopy/.env` (mode `0600`)
- Nginx path route: `conf/conf.d/default.conf`
- Reserved subdomain route: `conf/conf.d/fastcopy.conf`

The active public base URL is `https://zhy.hair/fastcopy`. The Nginx path proxy
removes `/fastcopy` before forwarding requests, including the WebSocket route.
`clip.zhy.hair` is already configured in Nginx but still needs a Cloudflare
Tunnel Public Hostname before it can be used.

## Containers

- `fastcopy-postgres`: PostgreSQL 17, internal Docker network only
- `fastcopy-server`: read-only filesystem, all capabilities dropped

The production environment sets `FASTCOPY_MAX_USERS=1`. The first account can
be created through the unified authentication endpoint; subsequent requests
with that account log in automatically.

## Operations

Run these commands from `/Volumes/SSD_ZHITAI/my-cloudflared-app`:

```bash
docker compose up -d --build fastcopy-server
docker compose ps fastcopy-server fastcopy-postgres
docker compose logs --tail=100 fastcopy-server
curl -fsS https://zhy.hair/fastcopy/healthz
```

Before deployment, server source is synchronized from this repository:

```bash
rsync -a --delete --exclude integration server/ \
  /Volumes/SSD_ZHITAI/my-cloudflared-app/fastcopy/server/
```

Backups created before the initial deployment are named
`docker-compose.yml.pre-fastcopy-20260818`,
`conf/nginx.conf.pre-fastcopy-20260818`, and
`conf/conf.d/default.conf.pre-fastcopy-20260818`.
