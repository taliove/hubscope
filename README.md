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

**Prebuilt binary** (Linux / macOS, amd64 / arm64 — from [Releases](https://github.com/taliove/hubscope/releases)):

```sh
curl -LO https://github.com/taliove/hubscope/releases/download/v0.1.0/hubscope_v0.1.0_linux_amd64.tar.gz
tar xzf hubscope_v0.1.0_linux_amd64.tar.gz
./hubscope_v0.1.0_linux_amd64
./hubscope_v0.1.0_linux_amd64 admin create --username admin --password 'your-strong-password'
```

**Docker** (no Go/Node toolchain needed):

```sh
git clone https://github.com/taliove/hubscope.git && cd hubscope
docker compose up -d --build
docker compose exec hubscope hubscope admin create --username admin --password 'your-strong-password'
```

**One-command install** (Linux, requires Go + pnpm — builds from source and installs a hardened systemd service):

```sh
sudo scripts/install.sh
```

Then open **http://localhost:8080**, log in, add a hub — models are discovered automatically.

Production deployment (systemd / nginx / reverse proxy): [docs/deployment.md](docs/deployment.md).

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
