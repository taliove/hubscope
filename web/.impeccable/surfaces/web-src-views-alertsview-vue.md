---
version: 2
slug: "web-src-views-alertsview-vue"
primary_target: "web/src/views/AlertsView.vue"
related_targets: ["web/src/utils/alertTimeline.ts","web/src/utils/alertKind.ts"]
---

# 故障记录 /alerts 事件时间线 — 表面简报

> **v1(2026-08-01,GH #122 补建,GH #117):** 告警历史自系统设置区迁出,重建为独立一级页事件时间线;AlertHistory 旧表格组件已删除,本页是告警历史的唯一呈现。
> **v2(2026-08-02,spec 0019 公开化与重构,五项裁决):** 页面对匿名开放(四类故障叙事边界,附录第 16 项);时间线改月>周>日三级嵌套可折叠 + 轨道跨日跨周连续;事件行内展开详情;筛选 URL 深链。与裁决冲突的 v1 规格(按日平铺、轨道日界裁剪、无行内详情、无深链、需登录)以本版为准,历史在 git。

## 范围与模式
- 模式:**公开状态板页**(spec 0019;v1 的「Operate 登录面」作废)——route `/alerts` 带 `meta.public`,侧栏「故障记录」项 `public: true`,匿名可达;**信息边界在服务端**:匿名 payload 只含故障叙事四类(down / recovered / group_down / group_recovered),七类运维管道事件(test / batch / quiet_summary / score_drop / score_drop_skipped / retire_pending / retired)登录可见(ui-guidelines 附录第 16 项,W6 级)。
- 读者:匿名访客——读事故叙事(什么时候坏的、影响谁、持续多久、恢复没有);登录运维——同页读全量事件(含运维管道类)与行内明细。

## 页面构成(自上而下)
1. **h1「故障记录」**(3xl 页面标题档;「页面 h1 = 侧边栏标签」惯例)。
2. **筛选条:** 模型 select(选项来自窗口内事件解析出的模型名集)/ 类型 select(登录态 = 十一 kind,选项 = `ALERT_KINDS` 单一来源,词经 `alertKindLabel`;**匿名态只列四类**——列出永不可能命中的七类 = 不诚实 UI)/ 时间范围 select(今天 = 本地日历日;最近 24 小时 / 7 天 / 30 天 = 滚动窗,默认 7d)。**三个筛选全部客户端过滤既有 limit 窗口,不发明第二服务端过滤口径。**
3. **筛选 URL 深链(裁决 5,ModelStatusList 五参数先例):** `model` / `kind` / `range` 三参数经 `router.replace` 镜像——200ms 防抖;默认值(model 空 / kind 空 / range=7d)不入参保持净 URL;打开时 URL 优先,缺省回落默认;编解码纯函数(参照 modelList.ts `listSortToQuery`/`parseListSortQuery` 先例,就近放 alertTimeline.ts 或新 util)+ vitest 守卫;query 变化不重挂载。
4. **时间线面板(轻容器):月 > 周 > 日三级嵌套(裁决 3)。**
   - **月组:** 月标签(同年「N 月」,跨年「YYYY 年 N 月」——不另立年层级)+ 可折叠;月组是最外分组,轨道线只在月组边界与面板顶/底收口。
   - **周组:** 月内按周分组(本地日历周,周一始),**周标签 = 周一始日期区间「M/D–M/D」(2026-08-02 main 落锤,否掉「第 N 周」——日期区间自解释、无周编号歧义;区间跨年时两端补年)** + 可折叠(周年月口径随月标签,同月内不重复年)。
   - **日组:** 周內按本地日历日分组,日标签 = 今天/昨天锚点 + 日期(v1 `groupEventsByDate` 标签口径沿置)。
   - **轨道线跨日跨周连续不断**——v1 的 `:first-child/:last-child` 日界裁剪作废,裁剪只发生在月组首行之前与末行之后;折叠是视图组织,折叠不丢事件(展开即恢复,不重新拉取)。
   - **折叠默认态(2026-08-02 main 落锤确认):** 当前月及其各周默认展开,更早月份默认折叠;折叠态会话内保持,不做 localStorage 持久化。
   - **事件行:** 时钟时间(sm secondary,tabular-nums)+ 轨道节点(9px 圆点,tag type 图形档着色)+ 类别 el-tag(`alertKindTagType`,size small)+ 影响对象(模型名;endpoint_id null 的聚合类事件显 group_key family 名;**已删除端点回退裸 id 标签——审计面永不丢行**)+ 持续时间(FIFO 配对,进行中显「持续中」danger-text)+ 投递状态 + message(列表态两行截断 + title 兜底沿置)。
   - **行内展开详情(裁决 4,EvalLiveFeed 先例——否掉弹窗/面板):** 点击事件行就地展开(行 role/tabindex/Enter/Space 键盘可达,aria-expanded),展开区纵排——完整 message(不截断)/ 完整时间戳(含日期与秒)/ 事件 ID · 端点 ID / 配对恢复事件与其持续时长(未配对显「进行中」)/ 投递状态明细;**端点事件附「查看端点详情」链接 → /endpoints/:id**(已删除端点同样可点,详情页承接深链);展开态 keyed by event id,轮询/筛选变化不塌(v1 无轮询,本条为防御登记);同时只展开一个或多个不设限(参照 EvalLiveFeed 多开先例)。
5. **分页:** 「加载更早的事件」走既有 `limit` 参数(50 → 100 → … → 200 服务端帽,`internal/server/probes.go parseLimit` 同口径);帽到显「已达单次上限 200 条,更早事件请缩小时间范围」。
6. 三态:首载 skeleton(**静态灰条,无 pulse**——v2 动效预算只给状态变化,spec 0018 决策 4)/ 错误带原因 + 重试 / 空态「所选范围内暂无告警事件」+ 提示「可放宽时间范围或清除筛选条件」(**空态命名当前范围,窄筛选不冒充清白记录**;匿名面空态同文案,永不读作「从无事故」)。

## 数据与纯函数(utils/alertTimeline.ts,vitest 覆盖)
- **`pairIncidentDurations`(沿置):** 每条 down/group_down 与同 scope(endpoint 或 group_key)内**其后第一条** recovered/group_recovered 配对,FIFO;未配对 = 进行中。配对在**未过滤窗口**上计算,显示筛选不改变事故跨度。行内详情复用同一配对结果(不发明第二配对口径)。
- **三级分组纯函数(新建,spec 0019):** 月 > 周 > 日嵌套分组装载 v1 `groupEventsByDate` 的日分组逻辑;周口径(本地日历周,周一始)与月/周标签格式在函数注释与本简报互指;输入保持 newest-first,输出各级均新在前。
- **`filterEventsByTimeRange`(沿置):** 客户端时间窗过滤(今天 = 日历日,余为滚动)。
- **深链编解码纯函数(新建,裁决 5):** model/kind/range 三参数双向编解码,坏值回落默认,参照 `utils/modelList.ts` 的 query 编解码先例。
- **模型名解析(沿置):** endpoint_id → model_id 映射来自 overview 载荷(公开 API,匿名可用);map 缺失回退裸 id(「审计面不丢行」)。

## 语义边界(沿置,不得回退)
- **告警事件词表 = 类别词表(十一 kind),非健康状态**——词表与 tag type 映射集中 `utils/alertKind.ts`(ui-guidelines §7),组件内禁写词字面量;本页不经过显示层三态映射。
- **借字例外:** 「厂商组告警」的「告警」借自域模型状态词表,语境限定本页,禁「修」字。
- **公开四类 / 登录七类的分叉在服务端**(ui-guidelines 附录第 16 项):前端不做 kind 隐藏式过滤;匿名面类型筛选器只列四类是「选项诚实」,不是信息边界本身。
- W5 告警管线零改动:本页只是既有 `GET /api/alerts` 数据的新呈现;告警生命周期(防抖/聚合窗口/分组告警/静默时段)测试保持绿是「后端告警逻辑不动」的回归证据(公开化改动仅限 API 读取分叉)。
- **「查看端点详情」链接是本页唯一出页交互**;不引入分享/导出入口(状态分享走状态概览,与 RecentEvents 边界一致)。

## 未决(另立批次)
- 服务端筛选(模型/类型/时间范围下推到 API——当前客户端窗口过滤,超 200 帽的历史事件不可达;三级折叠缓解但不根治)。
- 「处理状态」列(spec 0018 §12 story 44 的「处理状态」当前由投递状态 + 持续时长承担,无独立处置跟踪字段)。

## 落锤登记(2026-08-02,spec 0019 开工前)
- 折叠默认态:当月及其各周展开、更早月折叠、会话内保持不持久化(main 确认建议值);周标签 = 周一始日期区间「M/D–M/D」(main 落锤,否掉「第 N 周」)。
- group 事件 message 含 Hub 名分段:用户落锤接受并登记(Hub 名 = 租户标签,与 overview 匿名全局同层级;文案与历史行零改动,ui-guidelines 附录第 16 项)。
- sent_ok 投递状态:用户落锤不分叉全员可见(事故证据链一部分,非敏感)。
