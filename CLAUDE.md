# HubScope — Constitution

本项目的一切协作规则以此文件为准(宪法)。领域术语见 [CONTEXT.md](./CONTEXT.md),架构决策见 [docs/adr/](./docs/adr/),需求见 [docs/specs/](./docs/specs/),ticket 见 [.scratch/hubscope-mvp/issues/](./.scratch/hubscope-mvp/issues/)。

## 铁律(不可违反)

1. **测试不绿,不许 commit。** 任何 `git commit` 之前必须跑通 `make test`(后端全部测试 + 前端类型检查 + 前端构建)。没有例外。本规则由 git 硬门禁强制执行(clone 后跑一次 `make hooks` 启用):`pre-commit`(凭证扫描 + `make test`)、`commit-msg`(Conventional Commits 校验)、`pre-push`(保护 main 禁止删除/非快进 + `make test`);`--no-verify` 绕过会被 `.claude/hooks/block-no-verify.sh` 拦截。
2. **Commit message 一律英文**,遵循 Conventional Commits:`feat|fix|refactor|docs|test|chore|perf|ci: <description>`。代码注释也用英文。
3. **不主动 push、不主动发布。** `git push`、打 tag、部署等动作只在用户明确指令后执行。
4. **凭证不进 git。** Hub token、管理员口令等秘密只允许经环境变量或数据库注入;代码、配置样例、测试中一律使用假值。
5. **TDD:** 新行为先写(或同批写)黑盒测试,测试走唯一接缝——HTTP API 层(stub Hub + 假时钟 + 真 SQLite 临时库),不 mock 内部模块,不断言内部状态。

## 工程化

- **统一入口是 Makefile。** 常用动作:`make build`(前端构建 + Go 单二进制)、`make test`(全部测试与静态检查)、`make dev`(本地开发)。
- **技术栈(已定,不擅改):** Go + chi + modernc.org/sqlite(纯 Go,禁 cgo)+ robfig/cron;前端 Vue 3 + Vite + TypeScript + Element Plus + ECharts,产物 `go:embed` 进二进制。
- **目录约定:** `cmd/` 入口,`internal/` 后端模块(hubclient, prober, scheduler, discovery, status, evaluator, alerter, store, server),`web/` 前端工程,`docs/` 文档。
- **不可变数据优先:** 函数返回新值,不在调用者不知情时修改入参。
- **文件保持小而专:** 单文件超 ~400 行考虑拆分。

## 工作流

1. 从 frontier ticket 开工(`.scratch/hubscope-mvp/issues/`,Blocked by 全部完成的票)。
2. TDD 实现 → `make test` 全绿 → code review → 英文 commit。
3. 一票一 commit(或一票内多个原子 commit),完成 ticket 后在其文件的 Status 处标记 done。
