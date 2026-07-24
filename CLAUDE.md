# HubScope — Claude Code 增强层

**本项目治理正文(宪法)在 [AGENTS.md](./AGENTS.md)**——铁律、承重墙四问、测试三层、开工纪律、工作流、持续进化机制全部以该文件为唯一事实源,本文件不重复。本文件只登记 Claude Code 特有的增强机制。

## .claude 目录

| 路径 | 内容 |
|---|---|
| `.claude/agents/` | 项目级代理(7 个,扁平无 Lead):architect / implementer / design-owner / frontend-checker / code-reviewer / test-verifier / readme-writer。调用网与派发协议见 [.claude/rules/collaboration.md](./.claude/rules/collaboration.md) |
| `.claude/skills/` | 高频流程:new-api、db-change、frontend-dev、design-review、add-tests、implement-ticket、review、release-check、deploy |
| `.claude/rules/` | 规则正文:load-bearing-walls.md(承重墙)、ui-guidelines.md(设计规范)、collaboration.md(协作协议)、graph-tools.md(graph 使用纪律) |
| `.claude/hooks/` | 自动化钩子:block-no-verify.sh(PreToolUse 拦 `--no-verify`)、format-go.sh(PostToolUse 格式化 Go)、format-web.sh(PostToolUse 格式化 web/src) |

## 硬门禁

clone 后跑一次 `make hooks` 启用 `.githooks/`:pre-commit(凭证扫描 + `make test`)、commit-msg(Conventional Commits 校验)、pre-push(保护 main + `make test`)。`.claude/settings.json` 接线上表 hooks。

## 设计规范

前端 UI/UX 唯一设计规范是 [.claude/rules/ui-guidelines.md](./.claude/rules/ui-guidelines.md),由 `design-owner` 维护;新视图/新交互/新复用组件开工前必过设计评审(design-review skill)。
