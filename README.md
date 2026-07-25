<div align="center">

<img src="docs/assets/logo.svg" alt="HubScope" width="56">

# HubScope

**Availability monitoring and quality evaluation for AI hubs (model gateways).**

One Go binary. Embedded Vue dashboard with light and dark themes. Embedded SQLite. No runtime dependencies.

[![CI](https://github.com/taliove/hubscope/actions/workflows/ci.yml/badge.svg)](https://github.com/taliove/hubscope/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/taliove/hubscope)](go.mod)
[![Go Report Card](https://goreportcard.com/badge/github.com/taliove/hubscope)](https://goreportcard.com/report/github.com/taliove/hubscope)
[![Release](https://img.shields.io/github/v/release/taliove/hubscope)](https://github.com/taliove/hubscope/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**[简体中文](README.zh-CN.md)**

</div>

---

## What it does

- **Availability monitoring** — Probes every endpoint (model × protocol) every 5 minutes, over both non-streaming and streaming requests. Records success, latency, TTFT and token usage, and derives a green / yellow / red status with 24-hour availability trends.
- **Model evaluation** — 5 built-in capability suites (instruction following, reasoning, coding, knowledge Q&A, language understanding & generation) with hybrid rule-based + LLM-judge scoring, on a weekly schedule or on demand. Absolute scores stay comparable across time — silent vendor downgrades get caught.
- **Alerting** — Pushes to a Lark (Feishu) group bot when an endpoint keeps failing and when it recovers.
- **Admin console** — Hubs, models, webhooks and judge models are all configured online. Multi-user with per-hub roles and isolation. Light and dark themes included.

## Get started

**One-command install** (Linux — downloads the latest release binary, verifies its sha256, and installs a hardened systemd service; no Go/Node toolchain needed):

```sh
curl -fsSL https://raw.githubusercontent.com/taliove/hubscope/main/scripts/install.sh | sudo bash
sudo DATA_PATH=/var/lib/hubscope/app.db /usr/local/bin/hubscope admin create --username admin --password 'your-strong-password'
```

Pin a version with `HUBSCOPE_VERSION=v0.2.2`. Overridable via env: `HUBSCOPE_PREFIX`, `HUBSCOPE_DATA_DIR`, `HUBSCOPE_PORT` — see the script header.

**Docker** (no Go/Node toolchain needed — the image builds itself):

```sh
git clone https://github.com/taliove/hubscope.git && cd hubscope
docker compose up -d --build
docker compose exec hubscope hubscope admin create --username admin --password 'your-strong-password'
```

> **Behind an HTTP proxy?** The build pulls base images, Go modules and npm packages, so both the Docker daemon and the build itself need the proxy:
>
> ```sh
> # 1. daemon: /etc/systemd/system/docker.service.d/proxy.conf
> [Service]
> Environment="HTTP_PROXY=http://<proxy-host>:<port>"
> Environment="HTTPS_PROXY=http://<proxy-host>:<port>"
> Environment="NO_PROXY=localhost,127.0.0.1,::1"
> # then: sudo systemctl daemon-reload && sudo systemctl restart docker
>
> # 2. build containers:
> HTTP_PROXY=http://<proxy-host>:<port> HTTPS_PROXY=http://<proxy-host>:<port> \
>   docker compose up -d --build
> ```

**Prebuilt binary, no service** (Linux / macOS, amd64 / arm64 — from [Releases](https://github.com/taliove/hubscope/releases); good for a quick look):

```sh
curl -LO https://github.com/taliove/hubscope/releases/download/v0.2.2/hubscope_v0.2.2_linux_amd64.tar.gz
tar xzf hubscope_v0.2.2_linux_amd64.tar.gz
./hubscope_v0.2.2_linux_amd64
./hubscope_v0.2.2_linux_amd64 admin create --username admin --password 'your-strong-password'
```

Then open **http://localhost:8080**, log in, add a hub — models are discovered automatically.

Production deployment (systemd / nginx / reverse proxy): [docs/deployment.md](docs/deployment.md). Building from source or hacking on the code: `sudo scripts/install.sh --build-from-source` (requires Go + pnpm).

## Build from source

Requires Go 1.26+ and pnpm:

```sh
make build          # frontend (vite) + single binary → bin/hubscope
./bin/hubscope      # serves :8080, data in ./data/app.db
```

## Configuration

Everything is injected via environment variables; hub credentials live in the database (managed from the admin console), never in env or git.

| Variable  | Default         | Purpose            |
|-----------|-----------------|--------------------|
| `ADDR`    | `:8080`         | Listen address     |
| `DATA_PATH` | `./data/app.db` | SQLite data file |

Full table: [docs/deployment.md](docs/deployment.md).

## License

[MIT](LICENSE)
