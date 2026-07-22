# HubScope — Eval Campaign 与报告体系 Spec

> 状态:ready-for-agent。覆盖 ticket 28、29、31、32、33、34(前置 26/27/30 已合并 main)。术语遵循 [CONTEXT.md](../../CONTEXT.md),架构决策见 [ADR-0003](../adr/0003-eval-campaign-as-reporting-unit.md)(Campaign 聚合)、[ADR-0004](../adr/0004-task-as-first-class-entity.md)(Task 实体)、[ADR-0005](../adr/0005-absolute-scoring-over-elo.md)(绝对分制)、[ADR-0006](../adr/0006-share-link-token-access.md)(分享链接)、[ADR-0007](../adr/0007-immutable-cases-suite-versioning.md)(Case 不可变 + Suite 版本化)。

## Problem Statement

MVP 的评估体系(spec 0001)解决了"定期跑分",但无法回答"防供应商作假"的核心追问:

- 每周全量评估产生一组松散 Eval Run,没有"第 N 周考核"的批次概念,排行榜与跨周对比只能靠时间窗口隐式推断(定时与手动撞车即乱);
- 没有报告与排行榜:分数散在各 Run 里,看不出"这批模型整体谁强谁弱",更看不出"同一模型较上周是否被悄悄降级";
- 改题之后历史分数与当下不可比,但现状没有任何断点提示,趋势图会把题库变化误读为模型变化;
- 报告无法分享给未登录的同事/外部(ticket 16 后评估 GET 均需 session);
- 分数大跌告警逐 Run 对比,跨 Suite 版本时把"题变了"误报为"模型降级"。

## Solution

以 **Eval Campaign(考核批次)** 为一等聚合单位,向上长出报告体系:

1. **Campaign**:一键全量(所有 chat Model × 全部 Suite)产出每 Suite 一个 Run、同归一个 Campaign;手动单跑与每周定时也各自归入 Campaign,数据模型统一;
2. **Report**:每个完成的 Campaign 有报告页——Leaderboard(绝对分 0–100,加权平均)+ 跨 Campaign 趋势(Suite 版本断点标注)+ 探测侧延迟/成功率走势并列;
3. **Share Link**:报告可生成免登录只读 token 链接,可撤销、走审计;
4. **告警对齐**:分数大跌告警改为 Campaign 级对比,跨 Suite 版本跳过并标注"题目已变更";
5. **任务中心补全**:发现同步与 rollup/retention 清理注册为 Task,低频后台作业全部可追踪。

## 现状盘点(2026-07-22,基于 main c16ce6d 核实)

| 已有 | 缺口 |
|---|---|
| `tasks`/`task_logs` 表 + Task 中心页 + Eval Run 接入(ticket 27) | 发现同步、rollup/retention 未注册为 Task(ticket 28) |
| `tasks.type` 字段已存在 | 任务中心无类型过滤(28 顺带做) |
| Suite 版本化:`suites.version`、`eval_runs.suite_version`、Case 不可变替换(ticket 30) | 趋势图无版本断点;告警不感知版本(32/34) |
| 评估视图隐藏已删模型(ticket 26);`eval_results` 反规范化 `model_id` 文本,删模型后历史可渲染 | Leaderboard/趋势的删除口径需沿用(31/32) |
| `models.family` 分类(ticket 13) | 报告按 category 过滤 = 按 `family` 过滤(31) |
| 分数大跌告警:`alerter/score_drop.go`,逐 Run 与上一 done Run 比,阈值 0.2 | 需改为 Campaign 级对比 + 跨版本跳过(34) |
| 每周定时:周日 <6h 逐 Suite 顺序触发 Run(`scheduler/eval.go`) | 需产出 Campaign 而非松散 Run(29) |
| ticket 16:评估/设置/审计类 GET 需 session,仅看板类 GET 公开 | 分享链接需在鉴权层开 token 口子(33,ADR 0006) |
| 设置项:webhook、告警开关、裁判模型、默认采样次数 | 缺 Suite 权重设置(31) |

**ticket 28 半成品提示**:worktree `ticket-28b` 留有接近完工的未提交改动(`store/task_tracker.go` + 黑盒测试),但 `internal/server/discovery.go` 与 `tasks_jobs_test.go` 残留改名前模块路径 `ai-hub-checker`,编译不过;收尾时先修导入再跑测试。

## User Stories

### Campaign(ticket 29)

1. As an 管理员, I want 一键触发全量评估(所有 chat 模型 × 全部 Suite),so that 不用逐 Suite 手动点。
2. As an 管理员, I want 手动单跑一个 Suite 也自动归入单 Run Campaign,so that 数据模型统一、报告语义一致。
3. As an 管理员, I want 每周定时全量产出一个 Campaign,so that "第 N 周考核"是显式实体而非时间窗口推断。
4. As a Hub 使用者, I want 在任务中心看到 Campaign 整体进度(各 Run 聚合),so that 长跑的评估批次有可观测性。
5. As a Hub 使用者, I want 评估中心运行列表按 Campaign 分组,so that 我能按考核批次浏览而非面对松散 Run 列表。

### 任务中心补全(ticket 28)

6. As an 管理员, I want Hub 发现同步(手动触发与 Hub 新增自动同步)在任务中心可见,日志含新增/更新/停用模型数,so that 同步结果有据可查。
7. As an 管理员, I want rollup 与数据保留清理注册为 Task,日志含处理行数,so that 维护作业也可追踪。
8. As an 管理员, I want 任务中心按任务类型过滤,so that 作业多了能快速定位;探测轮次不出现(量级会淹没任务中心,ADR 0004)。

### Report 与 Leaderboard(ticket 31)

9. As a Hub 使用者, I want 每个完成的 Campaign 有报告页,Leaderboard 每模型一行、总分柱状排行(0–100,各 Suite 加权平均、默认等权),so that 一眼看出本批谁强谁弱。
10. As a Hub 使用者, I want 切换到按单个 Suite 查看分数、按模型 family 过滤、按总分/各 Suite 切换排序,so that 我能按场景钻取。
11. As an 管理员, I want 在设置页调整 Suite 权重,so that 总分口径贴合团队关注点。
12. As a Hub 使用者, I want 已删除模型不上榜,so that 榜单只反映当前可用模型(口径沿用 ticket 26)。

### 趋势与防作假(ticket 32)

13. As a Hub 维护方, I want 每模型 × 每 Suite 的跨 Campaign 分数趋势线,so that 上游悄悄降级可见。
14. As a Hub 维护方, I want 趋势线在 Suite 版本变更处标注断点("v2 起题目变更"),so that 题库变化不被误读为模型变化(ADR 0007)。
15. As a Hub 维护方, I want 报告页并列展示该模型探测侧延迟/成功率走势(复用 probe rollup),so that "分数稳但延迟暴涨"一眼可见。
16. As a Hub 维护方, I want 已删除模型的趋势仍可见并带「已删除」标记,so that 历史走势完整。

### 分享与导出(ticket 33)

17. As a Hub 维护方, I want 为 Campaign 报告生成免登录只读链接(/report/{token}),so that 能分享给未登录的同事/外部。
18. As an 管理员, I want 撤销分享链接(撤销后 404),并在管理页列出/管理全部链接,so that 分享面可控(ADR 0006)。
19. As an 管理员, I want 创建与撤销走审计日志,so that 外发行为可追溯。
20. As a Hub 维护方, I want 报告导出图片/PDF,so that 能贴进周报(静态分发,排在链接之后)。

### 告警与设置对齐(ticket 34)

21. As a Hub 维护方, I want 分数大跌告警以 Campaign 为单位对比(同 Suite 与上一个完成的 Campaign 比),so that 告警语义与"考核批次"一致。
22. As a Hub 维护方, I want 告警内容附各 Suite 跌幅与变动 Case 明细(得分变未判分/大降),so that 拿到告警就能定位。
23. As a Hub 维护方, I want 参与对比的两个 Run 的 Suite 版本不同则跳过告警并标注「题目已变更,分数不可比」,so that 改题不误报(ADR 0007)。
24. As an 管理员, I want 设置页告警文案/开关与新口径对齐,裁判模型、每周计划、权重、采样等评估设置齐全可用,so that 配置不误导。

## Implementation Decisions

### 数据模型(新增/变更)

- **`campaigns`**(新表):(id, trigger: scheduled|manual, status: pending|running|done|failed, started_at, finished_at, created_at)。状态由各归属 Run 聚合推导或落库更新,以落库为准(查询简单、进度可快照)。
- **`eval_runs`** 加列 `campaign_id INTEGER NOT NULL`(存量 Run 迁移:各自归入一个单 Run 的迁移 Campaign,保证 NOT NULL 成立)。
- **`share_links`**(新表):(id, token TEXT UNIQUE 高熵随机, campaign_id, created_by, created_at, revoked_at NULL)。撤销 = 写 revoked_at,不删行(审计可溯)。
- **`settings`** 新键:`suite_weights`(JSON,如 `{"basic":1,"reasoning":1,"coding":1,"chinese":1}`,缺省等权)。
- 复用:`tasks`/`task_logs`(28 注册新 type:`discovery_sync`、`rollup`、`retention`;29 的 Campaign 是否另挂 Task 以任务中心展示,按 ADR 0004 语义——Campaign 本身有状态机,任务中心展示 Campaign 进度走 campaigns 表,不重复建 Task)。

### API 契约(在 ticket 16 鉴权分档上叠加)

- 需 session(沿用评估类口径):
  - `GET /api/campaigns`(列表,含聚合进度)、`GET /api/campaigns/{id}`(含各 Run 状态/进度)
  - `GET /api/campaigns/{id}/report`(Leaderboard 聚合 + 各 Suite 明细 + 权重回显)
  - `GET /api/campaigns/{id}/trends`(每模型 × 每 Suite 跨 Campaign 分数 + Suite 版本号序列 + 探测侧 rollup 走势)
  - `POST /api/campaigns/{id}/share-links`、`DELETE /api/share-links/{id}`、`GET /api/share-links`
  - `POST /api/evals` 扩展:`suite_id` 缺省 = 一键全量
- 公开(无 session,token 即凭证,ADR 0006):
  - `GET /api/shared-reports/{token}` — 仅返回该 Campaign 的报告数据;token 错误/已撤销一律 404(不区分原因,防枚举);不暴露任何其他 API。
- 既有 `GET /api/evals`(Run 列表)响应增加 `campaign_id` 分组字段,保持字段向后兼容。

### 关键交互

- **一键全量**:创建一个 Campaign + 每 Suite 一个 Run(全部 chat 模型,`capability=chat` 自动排除 non_chat 与手动登记的非对话模型),Run 顺序执行(沿用现有周批节奏,避免瞬时打满 Hub)。
- **总分计算**:每 Suite 分 = Case 均分 × 100;总分 = 各 Suite 分按 `suite_weights` 加权平均(缺省等权)。已删除模型( ticket 26 口径:manual 已删 / discovered 已 retire)不上榜,但趋势视图保留并带「已删除」标记。
- **版本断点**:趋势数据按 Campaign 有序返回 (campaign_id, score, suite_version);前端在 version 变化处画断点标注,后端不做插值。
- **探测走势并列**:趋势 API 复用 `probe_rollups`(小时/天桶),按模型聚合其启用 Endpoint 的成功率与 P50/P95,与分数同时间轴返回,前端双图并列。
- **Campaign 级告警**:Campaign 完成时,对每个 (suite, model) 与上一个 done Campaign 的同 Suite Run 比;版本不同 → 跳过 + 任务日志/告警事件标注;阈值沿用 `ScoreDropThreshold`(0.2),`score_drop_alert_enabled` 开关语义不变。
- **分享链接**:token 高熵随机(≥128 bit);只读、仅单个 Campaign 报告;前端 `/report/{token}` 路由免登录渲染(复用报告页组件,隐藏操作类按钮);创建/撤销写审计。

### 前端页面(2026-07-22 重组定稿,已过 design-owner 评审,有条件通过)

**信息架构:/eval 改为纯消费页,运营与配置全部收进管理台。**

| 路由 | 页面 | 可见性 |
|---|---|---|
| `/`、`/endpoints/:id` | 状态板 | 公开(现状不变) |
| `/eval` | 评估榜单(新) | 登录 |
| `/admin` | 管理台 tabs:资源 \| 分类规则 \| **评估运营** \| **题库** \| 操作日志 \| 设置 | 登录 |
| `/report/{token}` | 分享报告(复用 Leaderboard,隐藏操作类按钮) | token 免登录 |
| `/tasks` | 任务中心:类型过滤下拉;发现同步/rollup/retention 日志展示统计行 | 登录 |

- **导航按登录态过滤**:未登录只渲染「状态总览」+ 登录按钮,不渲染会被门禁弹走的项(现有路由 bounce 保留为兜底)。
- **评估榜单页(/eval)**:顶部批次切换器(默认最新 done Campaign)+ 批次元信息 + 分享报告按钮;Hero 为 Leaderboard 条形排行(0–100 总分、较上批次涨跌箭头、Suite 切换 / family 过滤 / 排序走工具条);点行下钻走 `el-dialog`(不内嵌行内展开,§4),dialog 内为该模型的跨批次分数趋势(版本断点标注)+ 探测延迟/成功率并列;已删除模型不上榜、趋势带「已删除」标记。
- **评估运营 tab(/admin)**:「触发评估」收进对话框(单 Suite + 可搜索模型多选,或一键全量);批次列表一行一批、展开看各 Suite Run;Run 详情沿用 EvalRunDetailDialog。
- **题库 tab(/admin)**:CaseLibrary 平移,`authed` prop 删除(整页已有登录门禁)。
- **设置页**:Suite 权重编辑;告警文案对齐 Campaign 口径;分享链接管理列表。
- **评审条件(开工必须满足)**:① 下钻用 dialog,不做表格行内展开;② 跨 Suite 版本断点不显示涨跌箭头,占位灰 +「题目已变更」;③ 等待中/运行中批次榜单区呈进度态,不显示半成品名次,失败批次错误态 + 原因;④ 分数统一 0–100,`formatScore` 集中 `utils/format.ts`,组件禁自写 `toFixed`。
- **密度档位**:/eval 与分享报告页走 16px 消费档(读者是看板消费者,与登录态无关);/admin 全部 tab 走 12px 紧凑档。
- **废弃清单**:ScoreMatrix.vue、EvalTrendChart.vue 随榜单页落地废弃;EvalCenterView 重写为榜单页;EvalRunList 拆为「评估运营」tab(触发表单抽为对话框组件);CaseLibrary 挪管理台;EvalRunDetailDialog 保留。

## Testing Decisions

沿用 spec 0001 的唯一接缝(HTTP API 黑盒 + stub Hub + 假时钟 + 真 SQLite),不断言内部状态。各票核心断言:

- **28**:触发发现同步 → API 断言 Task 存在且日志含新增/更新/停用统计;推进时钟触发 rollup/retention → 断言 Task 与处理行数日志;任务列表按类型过滤生效;探测轮次不产生 Task。
- **29**:一键触发 → 断言 Campaign 结构(每 Suite 一个 Run、覆盖全部 chat 模型、non_chat 排除);手动单 Suite → 单 Run Campaign;假时钟推进到周批窗口 → 产出一个 Campaign;Campaign 聚合状态随各 Run 完成流转。
- **31**:report API 的总分加权(自定义权重生效)、family 过滤、排序切换;已删除模型不上榜。
- **32**:趋势 API 按 Campaign 有序返回分数与版本号;版本变更处数据含断点信息;删除模型趋势带标记。
- **33**:无登录凭 token 可读报告;撤销后 404;无 token/错 token 404;创建/撤销有审计记录;token 不可枚举(错误与撤销同形)。
- **34**:Campaign 对比触发告警(跌幅超阈值,内容含 Suite 跌幅与 Case 明细);跨版本跳过并标注;开关关闭不产生告警。

## Out of Scope

- Elo / Win Rate 等相对分制(ADR 0005 明确不做)
- 报告的多语言/主题定制
- 分享链接的密码保护、有效期自动过期(只有手动撤销;后续可加)
- Campaign 的部分重跑(失败 Suite 单独重跑归入原 Campaign 的语义后续再定)
- 公开 benchmark 题集引入(ADR 决策:泄露后被刷题反而帮供应商作弊)

## Further Notes

- **依赖链**:28 独立(27 已完成);29 是关键路径,解锁 31 与 34;32、33 依赖 31。完成 29 后 31 与 34 可并行。
- **ticket 28 收尾**:worktree `ticket-28b` 有近完工改动,先修 `ai-hub-checker` → `hubscope` 残留导入,再跑测试验证验收项。
- **评估成本**:一键全量 = chat 模型数 × 4 Suite × 每 Case 采样次数;采样上限 `MaxSampleCount=10` 已有,报告体系不新增成本维度。
- **迁移**:存量 Eval Run 逐个包成单 Run Campaign,历史数据立即可按 Campaign 浏览;趋势从迁移后开始有跨 Campaign 意义。
