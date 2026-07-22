# HubScope — Constitution

本项目的一切协作规则以此文件为准(宪法)。领域术语见 [CONTEXT.md](./CONTEXT.md),架构决策见 [docs/adr/](./docs/adr/),需求见 [docs/specs/](./docs/specs/),ticket 见 [.scratch/hubscope-mvp/issues/](./.scratch/hubscope-mvp/issues/)。承重墙清单见 [.claude/rules/load-bearing-walls.md](./.claude/rules/load-bearing-walls.md)。

## 铁律(不可违反)

1. **测试不绿,不许 commit。** 任何 `git commit` 之前必须跑通 `make test`(后端全部测试 + 前端类型检查 + 前端构建)。没有例外。本规则由 git 硬门禁强制执行(clone 后跑一次 `make hooks` 启用):`pre-commit`(凭证扫描 + `make test`)、`commit-msg`(Conventional Commits 校验)、`pre-push`(保护 main 禁止删除/非快进 + `make test`);`--no-verify` 绕过会被 `.claude/hooks/block-no-verify.sh` 拦截。
2. **Commit message 一律英文**,遵循 Conventional Commits:`feat|fix|refactor|docs|test|chore|perf|ci: <description>`。代码注释也用英文。
3. **不主动 push、不主动发布。** `git push`、打 tag、部署等动作只在用户明确指令后执行。
4. **凭证不进 git。** Hub token、管理员口令等秘密只允许经环境变量或数据库注入;代码、配置样例、测试中一律使用假值。
5. **TDD:** 新行为先写(或同批写)黑盒测试,测试走唯一接缝——HTTP API 层(stub Hub + 假时钟 + 真 SQLite 临时库),不 mock 内部模块,不断言内部状态。
6. **禁绕门禁。** 禁止 `--no-verify` 及任何形式的门禁绕过;门禁本身有误时修门禁,并在 commit message 说明。

## 承重墙(改动前必答四问)

承重墙是撑起系统语义、改错代价极高的结构与约定,清单维护在 [.claude/rules/load-bearing-walls.md](./.claude/rules/load-bearing-walls.md)。修改任何承重墙之前,必须在方案中书面回答四问:

1. 为什么必须改(不改的代价是什么)?
2. 影响哪些调用方(直接 + 间接,逐个列出)?
3. 有没有替代方案(加适配层/新接口代替改动)?
4. 改完回归测试什么(对应下方三层测试的哪几层)?

## 测试三层

提交前按改动范围自测三层,层数不够不许提交:

1. **当前功能层:** 本次改动直接覆盖的行为,新增/更新黑盒测试全绿。
2. **关联功能层:** 改动触及模块的既有测试回测(`go test ./internal/<module>/...` 及调用方包)。
3. **核心业务闭环层:** `make test` 全量(后端全部测试 + 前端类型检查 + 前端构建),覆盖"建 Hub → 同步模型 → 探测 → 状态/告警 → 评估 → 报表"闭环不被破坏——此层由 pre-commit 门禁强制执行。

## 工程化

- **统一入口是 Makefile。** 常用动作:`make build`(前端构建 + Go 单二进制)、`make test`(全部测试与静态检查,含 gofmt/vet)、`make fmt`(格式化)、`make lint`(静态检查)、`make package`(打包)、`make dev`(本地开发)。
- **技术栈(已定,不擅改):** Go + chi + modernc.org/sqlite(纯 Go,禁 cgo)+ 自写调度器(时钟可注入,非 cron 库);前端 Vue 3 + Vite + TypeScript + Element Plus + ECharts,产物 `go:embed` 进二进制。
- **目录约定:** `cmd/` 入口,`internal/` 后端模块(hubclient, prober, scheduler, discovery, status, evaluator, alerter, store, server),`web/` 前端工程,`docs/` 文档,`.claude/agents/` 项目级代理,`.claude/skills/` 项目级流程,`.claude/rules/` 承重墙与规则,`.claude/hooks/` 自动化钩子。
- **不可变数据优先:** 函数返回新值,不在调用者不知情时修改入参。
- **文件保持小而专:** 单文件超 ~400 行考虑拆分。

## 开工纪律

1. **先影响分析,后动手。** 每次开工前书面列出:直接影响(改哪些文件/接口)、间接影响(哪些调用方/页面/任务受波及)、公共调用方法(被动到的函数有哪些使用者)、权限与数据隔离风险(是否触碰鉴权边界、跨 Hub/租户数据)。
2. **改动收敛。** 单次任务只改必要范围,单 commit 最多 8 个文件(票内多 commit 拆分);不做无关重构,顺手发现的问题另记 ticket。
3. **Agent 分工。** 架构与影响分析用 `architect` 代理;代码审查一律由独立的 `code-reviewer` 代理执行(作者不自审);测试验证用 `test-verifier`;前端改动用 `frontend-checker`。高频流程见 `.claude/skills/`(new-api、db-change、frontend-dev、add-tests、review、release-check)。

## 工作流

1. 从 frontier ticket 开工(`.scratch/hubscope-mvp/issues/`,Blocked by 全部完成的票)。
2. 影响分析 → TDD 实现 → 三层测试 → `code-reviewer` 独立审查 → 英文 commit。
3. 一票一 commit(或一票内多个原子 commit),完成 ticket 后在其文件的 Status 处标记 done。
