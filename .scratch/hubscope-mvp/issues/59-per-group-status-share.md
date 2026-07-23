# Ticket 59: Per-group status share + maintenance-oriented card redesign

Status: done

## 背景

现状(ticket 56):分享入口只有全局一个,StatusCard 内容是「结论 + 前 10 条异常名单」。
用户痛点(2026-07-23 反馈,面向 AI 社区小队的查看与维护场景):

1. 只有总分享,每个分组(厂商/能力/协议)不能独立分享。
2. 结论口径粗:「1 个端点异常」吞掉 22 个降级,读者误判。
3. 明细只列模型名,没有异常原因、没有 24h 可用率,无维护价值。
4. 没有 24h 可用率、没有分段时间条、没有总结说明。

## 需求(用户已拍板)

1. **分组独立分享**:分组区(OverviewGroupSection)标题旁加分享按钮,生成范围=该分组的卡片;全局分享入口保留。范围 chip 标注分组维度+组名(延续防作假约定)。
2. **卡片内容重设计**(纯前端,数据层已具备:全局/分组 `availability_24h`、每端点 `success_rate_24h`/`dots_24h`、分组 `avg_latency_ms`):
   - 结论横幅改完整分布口径(正常/降级/宕机/告警各多少个)
   - 24h 可用率大数字
   - 24h 分段可用率条(组内聚合,分段填满式,沿用状态板样式语义)
   - 平均延迟
   - 异常明细升级:状态词 + 模型名 + status_reason + 单端点 24h 可用率
   - 正常端点不逐行,汇总一行(含可用率区间)
   - 自动生成一句话总结说明(规则生成,如「X 持续降级超 N 小时,建议排查上游」)
   - 视觉要求:好看、有设计感(design-owner 出具体视觉方案)
3. 受众:AI 社区小队,用途=快速看懂健康度 + 判断要不要动手维护。

## 约束

- 不改后端 API(overview 数据已够)。
- StatusCard 是 ui-guidelines 批 56 登记的物料,本次属语义变更,必须 design-owner 评审 + 回写 ui-guidelines。
- 结论必须标注统计范围(防作假,批 56 约定):分组卡片范围 chip 标「分组:厂商 · Anthropic」此类。
- 静态物料规则沿用:无动画(failing 用橙红实心点+chip)、无 hover、明细状态词用着色文字。
- 一票一 commit(或票内原子 commit),TDD:组件逻辑(聚合/文案/总结生成)抽纯函数进 utils,单测覆盖;黑盒三层自测。

## 影响分析(开工前)

- 直接:`web/src/components/StatusCard.vue`、`StatusShareDialog.vue`、`OverviewGroupSection.vue`、`DashboardView.vue`;新增 `web/src/utils/statusCardSummary.ts`(总结/聚合纯函数)等。
- 间接:HealthBanner 结论逻辑不动(卡片用自己的分布口径,不改共享 `conclusionText` 语义);`statusCardImage.ts`/`statusCardSnapshot.ts` 扩展 snapshot 结构。
- 公共调用:`utils/healthConclusion.ts` 的使用者(HealthBanner、StatusCard)——卡片侧新增口径不动既有函数签名,新增函数而非修改。
- 权限/数据隔离:无(公开状态板数据,无会话依赖)。

## 验收

- [x] 每个分组区可独立分享,卡片范围=该分组,范围 chip 正确标注
- [x] 卡片含:分布结论、24h 可用率大数字、分段条、平均延迟、带原因明细、正常端点汇总行、一句话总结
- [x] 全正常/全停用/零匹配/全异常等边界形态符合规范
- [x] ui-guidelines 回写(批 59 约定)
- [x] make test 全绿

## 实现备忘(2026-07-23)

- 设计评审:design-owner 有条件通过,ui-guidelines 批 59 条目已回写(§2/§3/§5)。
- **聚合口径修订(实现中发现,design-owner 复审回写)**:评审原案「聚合标量从 OverviewGroup/Overview 透传」有两个漏洞——① 带筛选分享时透传数字与范围 chips 声明的范围不一致(违反批 56 防作假);② Overview 全局无 avg_latency_ms 可透传。修订为:卡片数字一律从快照 enabled entries 计算——可用率 = dots_24h 按小时求和 ok/total(探测加权,与后端 groupAccumulator 同定义,无筛选时构造性相等);平均延迟 = enabled entries 的 p50_ms 均值;snapshot 不携带 aggregates。
- 一句话总结纯函数 `summaryText` 在 `utils/statusCardSummary.ts`,七条规则优先级命中即止;全正常但无 24h 数据时输出「当前全部正常;暂无 24 小时探测数据」(不用「运行平稳」,无证据不宣称)。
- 下载文件名带分组 scope 后缀:`hubscope-status-{groupKey}-YYYYMMDD-HHmm.png`。
