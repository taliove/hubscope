<div align="center">

<img src="docs/assets/logo.svg" alt="HubScope" width="56">

# HubScope

**AI Hub(模型网关)的可用性监控与质量评估。**

单 Go 二进制,内嵌 Vue 看板(亮/暗双主题)与 SQLite,无运行时依赖。

[![CI](https://github.com/taliove/hubscope/actions/workflows/ci.yml/badge.svg)](https://github.com/taliove/hubscope/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/taliove/hubscope)](go.mod)
[![Go Report Card](https://goreportcard.com/badge/github.com/taliove/hubscope)](https://goreportcard.com/report/github.com/taliove/hubscope)
[![Release](https://img.shields.io/github/v/release/taliove/hubscope)](https://github.com/taliove/hubscope/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**[English](README.md)**

</div>

---

## 功能

- **可用性监控** — 以 Endpoint(模型 × 协议)为单位,每 5 分钟一轮探测(非流式 + 流式),记录成败、延迟、TTFT、token 用量,产出红黄绿状态与 24 小时可用率趋势。
- **模型评估** — 5 个内置能力 Suite(指令遵循/推理/代码/知识问答/语言理解与生成),规则判定 + LLM 裁判混合打分,每周定时或手动触发。绝对分制跨时间可比,供应商悄悄降级无处遁形。
- **告警** — Endpoint 连续失败与恢复时推送飞书群机器人。
- **管理后台** — Hub 实例、模型启停、Webhook、裁判模型全部在线配置。多用户、按 Hub 角色与数据隔离。亮/暗双主题。

## 快速开始

**预编译二进制**(Linux / macOS,amd64 / arm64,见 [Releases](https://github.com/taliove/hubscope/releases)):

```sh
curl -LO https://github.com/taliove/hubscope/releases/download/v0.1.0/hubscope_v0.1.0_linux_amd64.tar.gz
tar xzf hubscope_v0.1.0_linux_amd64.tar.gz
./hubscope_v0.1.0_linux_amd64
./hubscope_v0.1.0_linux_amd64 admin create --username admin --password '换成强口令'
```

**Docker**(无需 Go/Node 工具链):

```sh
git clone https://github.com/taliove/hubscope.git && cd hubscope
docker compose up -d --build
docker compose exec hubscope hubscope admin create --username admin --password '换成强口令'
```

**一键部署脚本**(Linux,需要 Go + pnpm——从源码构建并安装为加固的 systemd 服务):

```sh
sudo scripts/install.sh
```

然后打开 **http://localhost:8080**,登录,添加 Hub——模型自动发现。

生产部署(systemd / nginx / 反向代理)见 [docs/deployment.md](docs/deployment.md)。

## 从源码构建

需要 Go 1.26+ 与 pnpm:

```sh
make build          # 前端构建 + 单二进制 → bin/hubscope
./bin/hubscope      # 默认监听 :8080,数据存 ./data/app.db
```

## 配置

全部经环境变量注入;Hub 凭证存数据库(管理后台维护),不进环境变量、不进 git。

| 变量        | 默认值           | 用途           |
|-------------|------------------|----------------|
| `ADDR`      | `:8080`          | 监听地址       |
| `DATA_PATH` | `./data/app.db`  | SQLite 数据文件 |

完整环境变量表见 [docs/deployment.md](docs/deployment.md)。

## License

[MIT](LICENSE)
