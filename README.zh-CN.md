<p align="center">
  <img src="docs/assets/logo.svg" alt="HubScope" width="128" />
</p>

<h1 align="center">HubScope</h1>

<p align="center">
  <strong>AI Hub(模型网关)的可用性监控与质量评估。</strong><br>
  单 Go 二进制,内嵌 Vue 看板(亮/暗双主题)与 SQLite,无运行时依赖。
</p>

<p align="center">
  <a href="https://github.com/taliove/hubscope/actions/workflows/ci.yml"><img src="https://github.com/taliove/hubscope/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/taliove/hubscope" alt="Go Version" /></a>
  <a href="https://goreportcard.com/report/github.com/taliove/hubscope"><img src="https://goreportcard.com/badge/github.com/taliove/hubscope" alt="Go Report Card" /></a>
  <a href="https://github.com/taliove/hubscope/releases"><img src="https://img.shields.io/github/v/release/taliove/hubscope" alt="Release" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT" /></a>
</p>

<p align="center">
  <strong><a href="README.md">English</a></strong>
</p>

---

## 功能

- **可用性监控** — 以 Endpoint(模型 × 协议)为单位,每 5 分钟一轮探测(非流式 + 流式),记录成败、延迟、TTFT、token 用量,产出红黄绿状态与 24 小时可用率趋势。
- **模型评估** — 5 个内置能力 Suite(指令遵循/推理/代码/知识问答/语言理解与生成),规则判定 + LLM 裁判混合打分,每周定时或手动触发。绝对分制跨时间可比,供应商悄悄降级无处遁形。
- **告警** — Endpoint 连续失败与恢复时推送飞书群机器人。
- **管理后台** — Hub 实例、模型启停、Webhook、裁判模型全部在线配置。多用户、按 Hub 角色与数据隔离。亮/暗双主题。

## 快速开始

**一键部署脚本**(Linux——下载最新 release 二进制、校验 sha256、安装为加固的 systemd 服务;无需 Go/Node 工具链):

```sh
curl -fsSL https://raw.githubusercontent.com/taliove/hubscope/main/scripts/install.sh | sudo bash
sudo DATA_PATH=/var/lib/hubscope/app.db /usr/local/bin/hubscope admin create --username admin --password '换成强口令'
```

可用 `HUBSCOPE_VERSION=v0.2.3` 锁定版本;`HUBSCOPE_PREFIX`、`HUBSCOPE_DATA_DIR`、`HUBSCOPE_PORT` 等环境变量可覆盖默认值,详见脚本头部注释。

**Docker**(无需 Go/Node 工具链——镜像在本地自行构建):

```sh
git clone https://github.com/taliove/hubscope.git && cd hubscope
docker compose up -d --build
docker compose exec hubscope hubscope admin create --username admin --password '换成强口令'
```

> **走 HTTP 代理?** 构建需要拉基础镜像、Go modules 和 npm 包,Docker daemon 和构建容器都要配代理:
>
> ```sh
> # 1. daemon:/etc/systemd/system/docker.service.d/proxy.conf
> [Service]
> Environment="HTTP_PROXY=http://<代理地址>:<端口>"
> Environment="HTTPS_PROXY=http://<代理地址>:<端口>"
> Environment="NO_PROXY=localhost,127.0.0.1,::1"
> # 然后:sudo systemctl daemon-reload && sudo systemctl restart docker
>
> # 2. 构建容器:
> HTTP_PROXY=http://<代理地址>:<端口> HTTPS_PROXY=http://<代理地址>:<端口> \
>   docker compose up -d --build
> ```

**预编译二进制,不装服务**(Linux / macOS,amd64 / arm64,见 [Releases](https://github.com/taliove/hubscope/releases);适合快速体验):

```sh
curl -LO https://github.com/taliove/hubscope/releases/download/v0.2.3/hubscope_v0.2.3_linux_amd64.tar.gz
tar xzf hubscope_v0.2.3_linux_amd64.tar.gz
./hubscope_v0.2.3_linux_amd64
./hubscope_v0.2.3_linux_amd64 admin create --username admin --password '换成强口令'
```

然后打开 **http://localhost:8080**,登录,添加 Hub——模型自动发现。

生产部署(systemd / nginx / 反向代理)见 [docs/deployment.md](docs/deployment.md)。从源码安装或二次开发:`sudo scripts/install.sh --build-from-source`(需要 Go + pnpm)。

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
