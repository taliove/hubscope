# Deployment

HubScope ships as a single Linux binary with the frontend and SQLite
migrations embedded. A Dockerfile is provided as an alternative.

## Binary deployment (recommended)

### 1. Build

```sh
make build          # pnpm build + go build → bin/hubscope
# cross-compile for a Linux target from macOS:
cd web && pnpm build && cd ..
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/hubscope-linux ./cmd/hubscope
```

### 2. Ship and configure

Copy the binary to the target host. Configuration is entirely via
environment variables — **no Hub credentials in config files** (Hub base
URLs and tokens live in the database, managed from the admin UI; see
ADR-0001).

| Variable          | Default          | Purpose                                   |
|-------------------|------------------|-------------------------------------------|
| `ADDR`            | `:8080`          | Listen address                            |
| `DATA_PATH`       | `./data/app.db`  | SQLite file location                      |
| `SESSION_SECRET`  | *(auto-generated)* | HMAC signing key for session cookies. Set explicitly for multi-instance deployments or to force rotation (rotation invalidates all sessions). If unset, a 32-byte hex secret is generated into the `settings` table on first start and reused across restarts. See ADR-0011. |
| `LOG_LEVEL`       | `info`           | Log verbosity: `debug` / `info` / `warn` / `error` |
| `TRUST_PROXY`     | `false`          | Set `true` only behind a reverse proxy that **replaces** `X-Forwarded-For` |
| `HTTPS_PROXY` / `HTTP_PROXY` / `NO_PROXY` | *(empty)* | Outbound proxy for hub traffic (standard Go behavior). **Required on machines running fake-ip local proxies** (e.g. Clash enhanced mode): direct DNS answers like `198.18.x.x` are unroutable and probes fail with `can't assign requested address`. Example: `HTTPS_PROXY=http://127.0.0.1:7890`. The effective proxy is logged at startup (credentials masked). |

> `ADMIN_PASSWORD` is **deprecated** (ADR-0011). It no longer gates startup; if still set during the transition window, a deprecation warning is logged. Bootstrap the first `super_admin` via the CLI instead (see step 2 below). The hard-delete of the env-var read path is tracked by ticket 69.

### 3. systemd unit

```ini
[Unit]
Description=HubScope
After=network.target

[Service]
Type=simple
User=ahc
WorkingDirectory=/opt/hubscope
Environment=ADDR=:8080
Environment=DATA_PATH=/var/lib/hubscope/app.db
ExecStart=/opt/hubscope/hubscope
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Bootstrap the first `super_admin` **before** starting the service the first time (ADR-0011):

```sh
sudo -u ahc /opt/hubscope/hubscope admin create --username admin --password '<new-password>'
# no --hub → super_admin (global, unbound to a hub)
# optionally: export SESSION_SECRET=<32-byte-hex> for multi-instance / forced rotation
sudo systemctl enable --now hubscope
```

> The `hubscope admin create` subcommand is delivered by ticket 69. Until 69 lands, the deprecation warning on `ADMIN_PASSWORD` is cosmetic-only; the CLI itself does not yet exist.

### 4. nginx reverse proxy

```nginx
server {
    listen 80;
    server_name hubscope.internal.example.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## Docker (alternative)

```sh
docker build -t hubscope .
docker run -d --name hubscope \
  -p 8080:8080 \
  -e SESSION_SECRET=<32-byte-hex-or-omit-to-auto-generate> \
  -v ahc-data:/data \
  hubscope
# bootstrap the first super_admin (one-shot, after the volume is initialized):
docker exec -it hubscope /hubscope admin create --username admin --password '<new-password>'
```

The SQLite database lives on the `ahc-data` volume (`/data/app.db`).

## First-run checklist

1. Bootstrap the first `super_admin`: `hubscope admin create --username admin --password '<new-password>'` (no `--hub` → super_admin; see ADR-0011). Optionally set `SESSION_SECRET` first for multi-instance or forced rotation.
2. Start the service, open the site → you are redirected to `/login`; sign in with the `admin` account.
3. Go to the admin page → add a Hub (name, base URL, API token). The token is stored in the database and never returned by the API (only a `token_hint` with the last 4 characters is shown).
4. Within a minute, model auto-discovery runs and populates models and endpoints (or add a model manually, or trigger `POST /api/discovery/run`).
5. Check the dashboard: endpoints flip green as the first probe rounds complete.
6. In settings, paste the Lark group-bot webhook URL to enable down/recovered alerts.
7. Trigger a first evaluation run manually from the evaluation center to establish score baselines; scheduled weekly runs follow automatically.

> Steps 1–2 depend on the CLI `hubscope admin create`, delivered by ticket 69. Until 69 lands the service still starts and the `ADMIN_PASSWORD` deprecation warning is cosmetic-only; the CLI itself does not yet exist.

## Credential safety

- `ADMIN_PASSWORD` is **deprecated** (ADR-0011) and will be hard-removed once the CLI bootstrap lands (ticket 69). Do not rely on it for new deployments; bootstrap the first `super_admin` via `hubscope admin create` instead. During the transition window it is still read from the environment only to emit a deprecation warning — never write a real password into any file committed to git (systemd unit files with real credentials should live outside the repo, or use `EnvironmentFile=` with `0600` permissions).
- `SESSION_SECRET` is the session-cookie HMAC key. Set it via env or let it auto-generate into the `settings` table. Treat it as a credential: rotation invalidates all sessions.
- Hub tokens are stored in SQLite. The API never returns them in plaintext; back up the database file accordingly and treat the backup as sensitive.
