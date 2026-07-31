---
version: 1
slug: "web-src-views-alertsview-vue"
primary_target: "web/src/views/AlertsView.vue"
related_targets: ["web/src/utils/alertTimeline.ts","web/src/utils/alertKind.ts"]
---

# 故障记录 /alerts 事件时间线(v2 新建)— 表面简报

> **v2 新建(2026-08-01,GH #122 补建):** 告警历史自系统设置区迁出,重建为独立一级页事件时间线(GH #117,spec 0018 §12);AlertHistory 旧表格组件已删除,本页是告警历史的唯一呈现。

## 范围与模式
- 模式:**Operate**(登录运维面);route `/alerts`,需登录(告警数据按监控数据分级,不进公开面)。
- 读者:运维——快速理解事故过程:什么时候坏的、影响谁、持续多久、告警发出去没有。

## 页面构成(自上而下)
1. **h1「故障记录」**(3xl 页面标题档;「页面 h1 = 侧边栏标签」惯例)
2. **筛选条:** 模型 select(选项来自窗口内事件解析出的模型名集)/ 类型 select(十一 kind,选项 = `ALERT_KINDS` 单一来源,词经 `alertKindLabel`)/ 时间范围 select(今天 = 本地日历日;最近 24 小时 / 7 天 / 30 天 = 滚动窗,默认 7d)。**三个筛选全部客户端过滤既有 limit 窗口,不发明第二服务端过滤口径。**
3. **时间线面板(轻容器):** 按日分组(h2 日标签),事件行 = 时钟时间(xs)+ 轨道节点(圆点,tag type 着色)+ 类别 el-tag(`alertKindTagType`,size small)+ 影响对象(模型名;endpoint_id null 的聚合类事件显 group_key family 名;**已删除端点回退裸 id 标签——审计面永不丢行**)+ 持续时间(FIFO 配对,见下;进行中显「持续中」)+ 投递状态(已发送/未发送,sent_ok 列)+ message(title 全显)。
4. **分页:** 「加载更早的事件」走既有 `limit` 参数(50 → 100 → … → 200 服务端帽,`internal/server/probes.go parseLimit` 同口径);帽到显「已达单次上限 200 条,更早事件请缩小时间范围」。
5. 三态:首载 skeleton(**静态灰条,无 pulse**——v2 动效预算只给状态变化,spec 0018 决策 4)/ 错误带原因 + 重试 / 空态「所选范围内暂无告警事件」+ 提示「可放宽时间范围或清除筛选条件」(**空态命名当前范围,窄筛选不冒充清白记录**)。

## 数据与纯函数(utils/alertTimeline.ts,vitest 覆盖)
- **`pairIncidentDurations`:** 每条 down/group_down 与同 scope(endpoint 或 group_key)内**其后第一条** recovered/group_recovered 配对,FIFO——多个连续 down 只消耗各自的 recovered,不多算不reuse;未配对 = 进行中(「持续中」)。持续时间文案走 `formatDuration`。
- **`groupEventsByDate`:** 按本地日历日分组,新日在前;日标签 = 今天/昨天/YYYY-MM-DD。
- **`filterEventsByTimeRange`:** 客户端时间窗过滤(今天 = 日历日,余为滚动)。
- **模型名解析:** endpoint_id → model_id 映射来自 overview 载荷;map 缺失回退裸 id(见上「审计面不丢行」)。

## 语义边界(沿置,不得回退)
- **告警事件词表 = 类别词表(十一 kind),非健康状态**——词表与 tag type 映射集中 `utils/alertKind.ts`(ui-guidelines §7),组件内禁写词字面量;本页不经过显示层三态映射(utils/statusDisplay.ts 注释与本条互指)。
- **借字例外:** 「厂商组告警」的「告警」借自域模型状态词表,语境限定本页,禁「修」字。
- W5 告警管线零改动:本页只是既有 `GET /api/alerts` 数据的新呈现;告警生命周期(防抖/聚合窗口/分组告警/静默时段)测试保持绿是「后端不动」的回归证据。

## 未决(另立批次)
- 服务端筛选(模型/类型/时间范围下推到 API——当前客户端窗口过滤,超 200 帽的历史事件不可达);事件详情展开;「处理状态」列(spec 0018 §12 story 44 的「处理状态」当前由投递状态 + 持续时长承担,无独立处置跟踪字段)。
