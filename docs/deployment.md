# Deployment

HubScope ships as a single Linux binary with the frontend and SQLite
migrations embedded. A Dockerfile and a `docker-compose.yml` are provided as
an alternative.

## Recommended: one-command install script

The recommended path is `scripts/install.sh`, which automates every manual
step described below — **no Go/pnpm toolchain required**:

```sh
curl -fsSL https://raw.githubusercontent.com/taliove/hubscope/main/scripts/install.sh | sudo bash
```

The script resolves the latest release (pin one with `HUBSCOPE_VERSION=vX.Y.Z`),
downloads the prebuilt binary for the host's OS/arch from GitHub Releases,
verifies its sha256 against the release checksums, installs it to
`/usr/local/bin/hubscope`, creates a `hubscope` system user and the
`/var/lib/hubscope` data directory, renders and enables the systemd unit,
waits for the health check to pass, and prints the next steps (the
`hubscope admin create` bootstrap command and the URL to open). Re-running
the script is safe: it upgrades to the requested release version (download,
replace the binary, restart the service) without touching existing data or
configuration.

For development checkouts, `sudo scripts/install.sh --build-from-source`
builds the binary from source instead (this path does require Go + pnpm).

The manual steps below are kept as a reference — for understanding what the
script does, for hosts where it cannot run, and for packaging.

## Binary deployment (manual reference)

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

| Variable         | Default          | Purpose                                   |
|------------------|------------------|-------------------------------------------|
| `ADDR`           | `:8080`          | Listen address                            |
| `DATA_PATH`      | `./data/app.db`  | SQLite file location                      |
| `LOG_LEVEL`      | `info`           | Log verbosity: `debug` / `info` / `warn` / `error` |
| `TRUST_PROXY`    | `false`          | Set `true` only behind a reverse proxy that **replaces** `X-Forwarded-For` |
| `HTTPS_PROXY` / `HTTP_PROXY` / `NO_PROXY` | *(empty)* | Outbound proxy for hub traffic (standard Go behavior). **Required on machines running fake-ip local proxies** (e.g. Clash enhanced mode): direct DNS answers like `198.18.x.x` are unroutable and probes fail with `can't assign requested address`. Example: `HTTPS_PROXY=http://127.0.0.1:7890`. The effective proxy is logged at startup (credentials masked). |

The first admin user is **not** configured via an environment variable. After
the binary is in place and the database is reachable, bootstrap the initial
`super_admin` with the CLI (see "Bootstrap the first admin" below); the server
refuses login until at least one user exists.

### 3. systemd unit

The single source of truth for the unit file is the template embedded in
`scripts/install.sh` — do not copy a unit from this document; either run the
script or read the template out of it. The key properties of the unit:

- `Type=simple`, `ExecStart` pointing at the installed binary
  (`/usr/local/bin/hubscope` by default).
- `User=hubscope` / `Group=hubscope` — a dedicated system user, never root.
- `Environment=DATA_PATH=/var/lib/hubscope/app.db` and
  `Environment=ADDR=:8080`; `WorkingDirectory=/var/lib/hubscope`.
- `Restart=on-failure` with a short restart delay.
- Basic hardening: `NoNewPrivileges=true`, `ProtectSystem=strict` (with the
  data directory whitelisted via `ReadWritePaths`), `ProtectHome=true`.

After installing a unit, enable and start it:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now hubscope
```

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

The compose file at the repository root is the preferred way to run HubScope
in Docker — it builds the image from the included Dockerfile, maps port 8080,
and attaches a persistent named volume in one command:

```sh
docker compose up -d --build
```

The SQLite database lives on the named volume `hubscope-data`, mounted at
`/data` inside the container (matching the Dockerfile's
`DATA_PATH=/data/app.db`). Recreating the container — e.g. after
`docker compose up -d --build` on a newer checkout — keeps all monitoring
history.

To bootstrap the first admin against the containerized database:

```sh
docker compose exec hubscope hubscope admin create \
  --username admin --password 'a-strong-password'
```

If you prefer plain `docker` without compose:

```sh
docker build -t hubscope .
docker run -d --name hubscope \
  -p 8080:8080 \
  -v hubscope-data:/data \
  hubscope
```

## Bootstrap the first admin

The first user (a global `super_admin`) must be created out-of-band via the
CLI, because there is no way to reach the authenticated user-management API
before any user exists. Run the binary with the `admin create` subcommand
against the same database the server uses — **passing `DATA_PATH`
explicitly**: the CLI defaults to `./data/app.db` relative to your current
directory, and running it from another directory silently creates a fresh,
empty database there (the command succeeds, but logins against the real
service database then fail with "invalid credentials"):

```sh
sudo -u hubscope DATA_PATH=/var/lib/hubscope/app.db \
  /usr/local/bin/hubscope admin create \
  --username admin --password 'a-strong-password'
```

To create a hub-scoped user instead of a global super_admin, pass `--hub
<id>` and `--role admin|operator|viewer` together (both required):

```sh
sudo -u hubscope DATA_PATH=/var/lib/hubscope/app.db \
  /usr/local/bin/hubscope admin create \
  --username alice --password 'a-strong-password' \
  --hub 1 --role operator
```

The password is bcrypt-hashed (cost 10) before it touches the database; the
plaintext is never written to disk or logs.

## First-run checklist

1. Bootstrap the first `super_admin` via the CLI (see above).
2. Open the site → you are redirected to `/login`; sign in with the
   credentials you just created.
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

- Admin passwords are set via the `hubscope admin create` CLI and stored only
  as bcrypt hashes in SQLite. They are never read from the environment and
  never written into any file; do not put credentials in systemd unit files
  committed to the repo (use `EnvironmentFile=` with `0600` permissions for
  any non-HubScope secrets the unit references).
- Hub tokens are stored in SQLite. The API never returns them in plaintext;
  back up the database file accordingly and treat the backup as sensitive.
