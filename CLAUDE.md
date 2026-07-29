# HubScope — Claude Code 增强层

**本项目治理正文(宪法)在 [AGENTS.md](./AGENTS.md)**——铁律、承重墙四问、测试三层、开工纪律、工作流、持续进化机制全部以该文件为唯一事实源,本文件不重复。本文件只登记 Claude Code 特有的增强机制。

## .claude 目录

| 路径 | 内容 |
|---|---|
| `.claude/agents/` | 项目级代理(3 个,扁平无 Lead):`plan`(开工前影响分析 + UI 评审)/ `write`(ticket 实现)/ `check`(提交前三维度验证)。调用网与派发协议见 [.claude/rules/collaboration.md](./.claude/rules/collaboration.md) |
| `.claude/skills/` | 5 个领域 skill,write agent 按任务组合使用:`product`(产品形态/读者/防作假/README 门面)/ `frontend`(前端开发流程)/ `backend`(后端开发流程)/ `database`(数据库变更)/ `ops`(发布与部署);另有横切工具 skill:`zh-punct`(中文物料全角标点保障,fix/check/selftest 三模式 + 线上回读复验) |
| `.claude/rules/` | 规则正文:load-bearing-walls.md(承重墙 W1–W8)、ui-guidelines.md(设计规范,由 `plan` agent 维护)、collaboration.md(协作协议 + 调用网)、graph-tools.md(graph 使用纪律) |
| `.claude/hooks/` | 自动化钩子:block-no-verify.sh(PreToolUse 拦 `--no-verify`)、format-go.sh(PostToolUse 格式化 Go)、format-web.sh(PostToolUse 格式化 `web/src`) |

## 硬门禁

clone 后跑一次 `make hooks` 启用 `.githooks/`:pre-commit(凭证扫描 + `make test`)、commit-msg(Conventional Commits 校验)、pre-push(保护 main + `make test`)。`.claude/settings.json` 接线上表 hooks(PreToolUse 接 block-no-verify;PostToolUse `Edit|Write` 接 format-go 与 format-web)。

## 设计规范

前端 UI/UX 唯一设计规范是 [.claude/rules/ui-guidelines.md](./.claude/rules/ui-guidelines.md),由 `plan` agent 维护;新视图/新交互/新复用组件开工前必过设计评审(经 frontend skill 调 `plan` 的 UI 评审子能力)。

## Agent skills

### Issue tracker

工作项全部走 GitHub Issues(`taliove/hubscope`,经 `gh` CLI 操作);2026-07-28 前的本地 Markdown 票(`.scratch/hubscope-mvp/issues/`)只读保留为历史档案。详见 `docs/agents/issue-tracker.md`。

### Triage labels

五个状态角色直接使用同名标签(`needs-triage` / `needs-info` / `ready-for-agent` / `ready-for-human` / `wontfix`),类别用 `bug` / `enhancement`。详见 `docs/agents/triage-labels.md`。

### Domain docs

单上下文布局:根 `CONTEXT.md` + `docs/adr/`;项目治理层(AGENTS.md / 承重墙 / 设计规范)优先于通用 skill 约定。详见 `docs/agents/domain.md`。
