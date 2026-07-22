# Deployment

AI Hub Checker ships as a single Linux binary with the frontend and SQLite
migrations embedded. A Dockerfile is provided as an alternative.

## Binary deployment (recommended)

### 1. Build

```sh
make build          # pnpm build + go build → bin/ai-hub-checker
# cross-compile for a Linux target from macOS:
cd web && pnpm build && cd ..
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/ai-hub-checker-linux ./cmd/ai-hub-checker
```

### 2. Ship and configure

Copy the binary to the target host. Configuration is entirely via
environment variables — **no Hub credentials in config files** (Hub base
URLs and tokens live in the database, managed from the admin UI; see
ADR-0001).

| Variable         | Default          | Purpose                                   |
|------------------|------------------|-------------------------------------------|
| `ADDR`           | `:8080`          | Listen address                            |
| `DATA_PATH`      | `./data/app.db`  | SQLite file location                      |
| `ADMIN_PASSWORD` | *(required)*     | Admin login password; the service refuses to start without it |
| `LOG_LEVEL`      | `info`           | Log verbosity: `debug` / `info` / `warn` / `error` |
| `TRUST_PROXY`    | `false`          | Set `true` only behind a reverse proxy that **replaces** `X-Forwarded-For` |
| `HTTPS_PROXY` / `HTTP_PROXY` / `NO_PROXY` | *(empty)* | Outbound proxy for hub traffic (standard Go behavior). **Required on machines running fake-ip local proxies** (e.g. Clash enhanced mode): direct DNS answers like `198.18.x.x` are unroutable and probes fail with `can't assign requested address`. Example: `HTTPS_PROXY=http://127.0.0.1:7890`. The effective proxy is logged at startup (credentials masked). |

### 3. systemd unit

```ini
[Unit]
Description=AI Hub Checker
After=network.target

[Service]
Type=simple
User=ahc
WorkingDirectory=/opt/ai-hub-checker
Environment=ADDR=:8080
Environment=DATA_PATH=/var/lib/ai-hub-checker/app.db
Environment=ADMIN_PASSWORD=change-me-in-production
ExecStart=/opt/ai-hub-checker/ai-hub-checker
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```sh
sudo systemctl enable --now ai-hub-checker
```

### 4. nginx reverse proxy

```nginx
server {
    listen 80;
    server_name ai-hub-checker.internal.example.com;

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
docker build -t ai-hub-checker .
docker run -d --name ai-hub-checker \
  -p 8080:8080 \
  -e ADMIN_PASSWORD=change-me-in-production \
  -v ahc-data:/data \
  ai-hub-checker
```

The SQLite database lives on the `ahc-data` volume (`/data/app.db`).

## First-run checklist

1. Open the site → you are redirected to `/login`; sign in with
   `ADMIN_PASSWORD`.
2. Go to the admin page → add a Hub (name, base URL, API token). The token
   is stored in the database and never returned by the API (only a
   `token_hint` with the last 4 characters is shown).
3. Within a minute, model auto-discovery runs and populates models and
   endpoints (or add a model manually, or trigger `POST /api/discovery/run`).
4. Check the dashboard: endpoints flip green as the first probe rounds
   complete.
5. In settings, paste the Lark group-bot webhook URL to enable down/recovered
   alerts.
6. Trigger a first evaluation run manually from the evaluation center to
   establish score baselines; scheduled weekly runs follow automatically.

## Credential safety

- `ADMIN_PASSWORD` is only ever read from the environment. Do not write it
  into any file that is committed to git (systemd unit files with real
  passwords should live outside the repo, or use `EnvironmentFile=` with
  `0600` permissions).
- Hub tokens are stored in SQLite. The API never returns them in plaintext;
  back up the database file accordingly and treat the backup as sensitive.
