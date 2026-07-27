# HubScope — 工程治理规范

本文件是 HubScope 的治理正文(宪法),对**所有 AI 编程工具**生效(Claude Code、Codex 及任何读 AGENTS.md 的代理)。Claude Code 特有机制(agents / skills / hooks 目录)见 [CLAUDE.md](./CLAUDE.md)。领域术语见 [CONTEXT.md](./CONTEXT.md),架构决策见 [docs/adr/](./docs/adr/),需求见 [docs/specs/](./docs/specs/),ticket 见 [.scratch/hubscope-mvp/issues/](./.scratch/hubscope-mvp/issues/)。

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

1. **先影响分析,后动手。** 每次开工前书面列出:直接影响(改哪些文件/接口)、间接影响(哪些调用方/页面/任务受波及)、公共调用方法(被动到的函数有哪些使用者)、权限与数据隔离风险(是否触碰鉴权边界、跨 Hub/租户数据)。影响分析由 `plan` 代理产出,write agent 不复制其文本、直接调用。**按 Hub 查询隔离不变量(spec 0005,Phase 64 起的新隐式承重约定):** 新增 list/query handler 必须按 session user 的 hub_id 过滤(super_admin 传 nil / 走 `*All` 变体);store 层 `List*` 函数签名强制 hubID 非可选(去无参形态,拆 `ListXByHub(hubID)` + `ListXAll()`,`All` 仅 super_admin 路径与 store-internal 全局维护可达)——漏传 hub 过滤 = 编译错误;运行时第二道防线是 `internal/server/isolation_test.go` 的 sweep,新增已隔离 list 接口须登记入其 `isolatedListPaths` 表并加断言行。
2. **改动收敛。** 单次任务只改必要范围,单 commit 最多 8 个文件(票内多 commit 拆分);不做无关重构,顺手发现的问题另记 ticket。
3. **Agent 分工与协作。** 扁平结构不设 Lead,3 个 agent + 5 个领域 skill:`plan`(开工前影响分析 + UI/UX 设计评审,只读,仅维护 ui-guidelines.md)、`write`(ticket 实现,阶段 1 调 plan、阶段 2 组合领域 skill)、`check`(提交前三维度验证:测试 + 规范双轴 + 前端细节,不改代码只报告);领域 skill 由 write agent 按任务组合:`product`/`frontend`/`backend`/`database`/`ops`。调用网、派发协议(任务/背景/输入/执行/输出/影响/风险七字段)、职责重叠裁决见 [.claude/rules/collaboration.md](./.claude/rules/collaboration.md)。探索代码先用 code-review-graph 工具,纪律见 [.claude/rules/graph-tools.md](./.claude/rules/graph-tools.md)。

## 工作流

1. 从 frontier ticket 开工(`.scratch/hubscope-mvp/issues/`,Blocked by 全部完成的票)。
2. 影响分析(`plan`)→ TDD 实现(`write` + 组合领域 skill)→ 三层测试与独立审查(`check`)→ 英文 commit。
3. 一票一 commit(或一票内多个原子 commit),完成 ticket 后在其文件的 Status 处标记 done。
4. **合并与发布走 [docs/releasing.md](./docs/releasing.md)**:改动一律经 PR(模板 + 恰好一个 release-notes label + CI 绿 + squash merge)进 main,禁止本地 FF 直推;发版唯一入口是 `scripts/release.sh vX.Y.Z`(经用户明确指令后执行),GitHub Release 由 release workflow 自动创建(分类 notes + 产物)。
5. **测试线部署:** 192.168.1.101 测试服务器部署流程见 [.claude/skills/deploy-test-101/](./.claude/skills/deploy-test-101/),涵盖标签部署(生产发布)与开发部署(测试未提交代码)两种模式,包含完整的备份、健康检查和回滚流程。用户明确指令后可执行 `/deploy-test-101` skill。

## 持续进化

AI 团队在实践中沉淀,不设专门仪式,触发器挂在既有钩子上:

| 触发 | 沉淀到 |
|---|---|
| 同一流程被手写第 3 次 | `.claude/skills/<name>/` 新流程 |
| 一条约束被违反第 2 次(或一次代价高昂) | rules/ 新条目;涉及系统语义的升级为承重墙(W9+,走四问 + ADR) |
| 出现无人认领的职责 | 新增 agent,并在 [collaboration.md](./.claude/rules/collaboration.md) 调用网登记 |
| 一个非显而易见的决策被做出 | 系统语义 → `docs/adr/`;协作/流程教训 → 用户级 memory |

执行机制:`check` agent 审查时发现同类问题第二次出现,在报告末尾附「沉淀建议」;ticket 收尾(标记 Status: done 前)自问一句——有没有「第三次手写的流程 / 第二次违反的约束 / 无人认领的职责 / 未记录的决策」,有则顺手沉淀或另记 ticket。
