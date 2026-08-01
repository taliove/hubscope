---
version: 1
slug: "web-src-views-dashboardview-vue"
primary_target: "web/src/views/DashboardView.vue"
related_targets: ["web/src/components/StatusHero.vue","web/src/components/MetricWidgets.vue","web/src/components/ModelStatusList.vue","web/src/components/ModelDetailPanel.vue","web/src/components/StatusBadge.vue","web/src/components/TrendSparkline.vue","web/src/components/StatusShareDialog.vue","web/src/components/ProbeLatencyChart.vue","web/src/components/RecentEvents.vue","web/src/utils/statusDisplay.ts","web/src/utils/overviewMetrics.ts","web/src/utils/overviewDots.ts","web/src/utils/healthConclusion.ts","web/src/utils/severitySort.ts","web/src/utils/modelList.ts","web/src/utils/sparklineBars.ts","web/src/utils/modelDetailPanel.ts","web/src/utils/numberTween.ts","web/src/utils/recentEvents.ts"]
---

# 状态概览(/,v2 重建)— 表面简报

> **v2 重写(2026-08-01,GH #122):** 旧「信号墙」状态板(Hero 指挥台带 + EndpointCard 卡片矩阵 + OverviewGroupSection + UptimeStrip + 速览弹窗)全部退役;本简报按建成世界重写(GH #115/#116,spec 0018 §6/§7/§8/§10)。

## 范围与模式
- 模式:**Operate**(公开只读监控面);route `/`,未登录可达;壳内渲染(AppSidebar 外壳)。
- 读者:状态板读者(3 秒看懂,可能投屏/路过/远距)。任务:5 秒回答「整体健不健康 / 哪些模型有风险 / 异常影响范围 / 下一步处理什么」。
- 全球约定见 PRODUCT.md(读者模型)与 DESIGN.md(令牌/轻容器/动效);业务语义(三态映射/防作假)见 ui-guidelines §3。

## 页面构成(自上而下)
> **骨架底色(GH #139,2026-08-01 第四轮实机反馈):** 页面骨架底自白色回灰——App.vue main 区与 AppSidebar 同消费 `--hs-bg-subtle`,各区域(Hero / 指标格 / 列表容器 / 事件卡)= bg-card 白 tile + 1px 描边 + radius-lg 分层,AppTopbar 保持白面 + hairline;区间距统一 space-4 节奏(页头→Hero→指标→工具条→列表→事件)。
1. **可见页头(2026-08-01 参照稿复刻批):** h1「状态概览」(3xl 页面标题档,600 primary)+ lede 一行「全局视角,掌握 AI 服务运行健康状态」(md secondary)——「页面 h1 = 侧边栏标签」惯例的 sr-only 例外随本批退役,可见 h1 直接承担 a11y 树,不重复。
2. **StatusHero**(健康指数 hero 区)
3. **MetricWidgets**(四格指标区)
4. **筛选工具条**(关键词 / 供应商 / 状态(三态)+ 「分享状态」主按钮;协议筛选与分组选择器随 GH #131 退役)
5. **ModelStatusList**(高级列表,列头点击排序默认按可用率降序,GH #136)
6. **ModelDetailPanel**(行点击开启的右侧详情面板,teleport)
7. **RecentEvents**(近期事件区,GH #132;**登录态专属**——`authed=false` 整区不渲染零请求,匿名读者面不泄露运营事件)
8. 三态:首载 skeleton 行(后续轮询保持列表不打断——局部刷新)、筛选零匹配「暂无匹配的 Endpoint」、零端点「暂无监控端点,请先在模型管理中添加」、刷新失败 el-alert 带原因(保留上次好数据)。

## 组件规格

### StatusHero(健康指数区,GH #115)
- **白 tile 容器(GH #139):** bg-card + 1px 描边 + radius-lg,内 padding 24 纵 / 32 横(space-5/space-6),区间距 space-4;skeleton 与加载态同高锚定 242px(内容列 194 + 纵 padding 48,算术注释在组件内)。
- **布局比例(2026-08-01 参照稿复刻批;GH #139 级联修订):** 左列(统计)定宽 320px 窄列(`flex: 0 0 320px`),右列(趋势图)`flex: 1 1 0` 占满余宽;级联算术(盒模型前提:本仓无全局 box-sizing reset,content-box):1200px 内容宽(max-width 1200,padding 不入内容宽)− 2(tile 描边)− 64(tile padding 32×2)= 1134 内道;1134 − 320 − 64(space-8 列距)→ 右列 750px,构造比例 320:750 ≈ 1:2.3(票面「约 1:3」容差内);meta「自动刷新」保持右上,列间距 space-8。
- 构成:hero 72px/600 大数字(`--hs-text-hero`,tabular-nums,letter-spacing -0.02em)+ 次级「%」(2xl secondary)+ 右侧纵列:结论词(xl/600,tone *-text 阶)+ 日环比行(md,`较昨日 ±X.X%` / `较昨日持平`,tone 着色;null 整行不渲染)+ 统计范围(xs secondary,「统计范围:N 个启用端点」)。
- 数据:`health_score_24h` / `health_score_delta` 后端聚合直渲(api-contract 健康指数节);结论词 `utils/healthConclusion.ts`(与物料同源):异常态「N 个端点异常」(down+failing 合并)、降级态「N 个端点降级」、全稳定「全部稳定运行」、空「暂无数据」(词表 GH #128)。
- **防作假不变式:** null 健康指数 → 大数字位渲染中性「暂无数据」(3xl placeholder),永不显示 100%;null 折叠进 empty 分支,结论词与 tone 同归中性。
- 数字补间 500–800ms(useTweenedNumber/numberTween.ts,600ms 中值 easeOutCubic,reduced-motion 立即落终值);skeleton 与加载态同高锚定(min-height 242px,见上条白 tile 容器)。

### MetricWidgets(指标区,GH #115)
- 四格轻容器(grid 4 列 `minmax(0,1fr)`,gap space-4;hover 上浮 2px + shadow-md——卡片类):**24h 可用率**(3xl 大数字 + 副行日环比,与健康指数同口径同源;null 副行「较昨日暂无对比」)/ **24h 请求量**(大数字 + 副行「探测总次数」)/ **平均延迟**(大数字 + 副行恒注「启用端点 P50 均值」——批 59 scope 恒一致口径,`meanP50Ms`)/ **风险模型数**(大数字 + 副行「异常 N · 降级 N」或「全部稳定运行」;伞状标题不撞 incident 专属状态词「异常」,GH #128 裁决)。
- **风险模型数(原「异常模型数」,GH #128 改题)去重口径(GH #115 裁决):** `abnormalModelCounts` 按**模型**去重(不是端点计数)——同模型多端点取最重显示态(incident > degraded),只计 enabled;一个双协议同时异常的模型只数一次,与「哪些模型有风险」的读者问题对齐。
- 每格配 TrendSparkline(单调插值、null 断线、无轴无网格,aria-hidden;GH #130 语义四 lane 着色:线 = 功能基色、面积同色 0.15——可用率 success/请求量 brand/延迟 warning/风险模型 danger;GH #137 形态分工:**请求量格 = bars 柱状变体**(24 柱 2px 间隙、顶角 radius-xs、max 桶全高归一、空态灰轨道,`utils/sparklineBars.ts`),其余三格 = line 曲线);可用率/请求量/失败格序列来自聚合 dots(探测加权,overviewDots 纪律),延迟格来自 enabled entries 小时均值;每格右上 36px 图标 chip(radius-sm,soft 底 + 基色图标,GH #130)。
- 核心数字补间(同 Hero);null 值显 placeholder 色;skeleton 四格同形。

### ModelStatusList(模型状态列表,GH #115;取代 EndpointCard 矩阵)
- **容器(GH #139):** 分区 = 白 tile(bg-card + 1px 描边 + radius-lg + space-2 内 padding,灰骨架底上分层;8px 内 padding 让行 hover 上浮留在 tile 内、末行去 hairline 由容器边收口)。
- 列构成(共享 grid 模板 `minmax(0,1.8fr) 100px 150px 150px 100px minmax(150px,1fr) 40px`,表头与行同一模板,GH #139 重排):模型 / 供应商 / 状态 / 24h 可用率 / P95 延迟 / 24h 趋势 / 操作。
- **模型名第一层级:** md/600 墨色,中间截断(splitMiddle tailKeep=12,头 ellipsis 尾保区分度后缀)+ el-tooltip 快显(show-after 200ms)全显;已停用行名弱化(secondary/400)+「已停用」xs placeholder 注。指标全部辅助层级(P95 sm regular tabular-nums、可用率 md tabular-nums `availabilityRateTier` *-text 阶,数字左进度条右同行,GH #136);**供应商瓦片回名称格(GH #139,参照稿:名称格 = 瓦片 + 模型 ID,供应商列 = 文本):** 26×26 统一品牌色块瓦片(white glyph,未知回落字母瓦片,`utils/vendorIcon.ts` 单源)内联于模型 ID 左侧,`title` 兜底全名;**供应商列恢复 family 文本**(sm secondary,截断 + title)——取代 GH #136「供应商列图标化、名称格 chip 退役」的处置,「一行不重复两枚图标」纪律以语义位交换形态存续。
- 状态列 = StatusBadge sm + causes(唯一状态灯纪律不破);趋势列(GH #136)= TrendSparkline 行内 20px 道(行级延迟序列,按显示态着色 rowSparklineTone;UptimeMicroStrip 已随 GH #131 退役);操作列(GH #136)= chevron-right 裸按钮(lucide 18px,aria-label「查看详情」,@click.stop)。
- **行交互:** 整行可点开 ModelDetailPanel(role="button" + tabindex="0" + Enter/Space + `data-endpoint-id` 焦点归还锚点);**行 hover 上浮 2px + shadow-md + 卡面底**(卡片类读感:radius-lg 圆角盒 + 12px padding;与 Leaderboard 矩阵行的表格读感豁免分属两侧,差异登记在 ui-guidelines 附录第 5 项);`:focus-visible` = 2px brand outline(offset 1px)。
- **排序(GH #136 重写):** 列头点击排序(可排序键 = 名称/可用率/P95;`nextListSort` 状态机,新列首击优在前,再击翻向;`sortListEntries` 执行——桶序不随向:有值启用行 → 无数据启用行 → 已停用沉底,方向只作用桶内,同值 severity 秩回退);**默认 = 可用率降序(高在前,推翻 GH #131「差在前」)**;持久化 localStorage 键 `hs:list-sort`(坏值回落默认);副注动态化 `listSortNote`(如「(按可用率降序)」,标签与数据构造性一致);severitySort 仅余分区排序与 healthConclusion 消费;轮询/筛选驱动的重排不做动画(GH #52 纪律)。
- **分组模式(轻分区,取代 OverviewGroupSection):** section-header = 组名(lg/600)+ meta 行(xs secondary:「N 个端点」或「N 个端点 · 异常 N · 降级 N」,计数措辞由父级经显示层映射组句,禁字面量);分区排序 = 组内最重 enabled entry 秩,tie 组键字典序。**旧组头机械(折叠披露三件套、协议收敛 tag、「本组」指标、组级 UptimeStrip、组分享按钮)全部退役**——组分享入口退役后,分享只从筛选行全局入口与端点详情页进入(快照范围 = 当前筛选,分组 chip 由筛选条件承担)。

### ModelDetailPanel(右侧详情面板,GH #116;取代速览弹窗与整行深链)
- 自造右缘全高 sheet + scrim(`--hs-overlay-bg`),**不用 el-dialog**;入场 panel-slide/panel-fade 双 Transition(reduced-motion 全局归零覆盖)。
- 五区:头部(模型名 h2 xl/600 截断 + StatusBadge md + causes +「已停用」注 + status_reason xs secondary)/ 三指标格(24h 可用率 tier 着色、平均延迟 P50 墨色 + P95 副注、错误率 tier 着色——`utils/modelDetailPanel.ts` 快照推导,null 显 `-`)/ 「24h 延迟趋势」(ProbeLatencyChart 180px,**不平滑**,逐探测保真——ui-guidelines §3.3)/ 「事件记录」(24h 失败事件:时间 + 流式/非流式 + 原因截断,空态「24h 内无失败记录」)/ 底部「打开完整详情」主按钮 → /endpoints/:id。
- **「错误趋势」范围收窄(GH #116 main 裁决):** 错误维度由三处承担(延迟图故障窗 markArea + 错误率指标格 + 事件记录),独立错误趋势图不另立。
- **快照冻结:** entry prop = 行点击时冻结副本;overview 10s 轮询不刷新已开面板;异步区(延迟图 + 事件共享一次 24h `hours=24` 拉取)打开时一次性拉取,skeleton/错误重试三态,失败只污染该区。
- **自造模态面三件套(本组件为首例):** focus trap(utils/focusTrap.ts,Tab/Shift+Tab 面内循环)+ ESC/scrim/关闭按钮统一 `close` emit + 焦点归还触发行(父级 `data-endpoint-id` querySelector;深链跳走后 querySelector 构造性 no-op)。**不放分享入口**(弹窗不叠面板)。
- aria:`role="dialog" aria-modal="true" :aria-label="模型名"`;打开后焦点落关闭按钮。

### 筛选工具条
- 关键词 el-input(220px)+ 供应商 select(选项 = `familyOptions` 单一来源,GH #131 取代协议筛选)+ 状态 select(**三显示态**,轻→重排列,词来自 statusLabel;「异常」同筛 down+failing,`toDisplayStatus` 匹配)+ 「分享状态」主按钮(margin-left:auto;首载前禁用);**分组选择器随 GH #131 退役**;副注动态化(GH #136,`listSortNote`)。
- 筛选语义(GH #113 main 裁决):状态筛选跟显示词表走,UI 不提供 failing 单筛;关键词小写子串匹配模型名。
- 供应商/状态 select 前置内联 label(sm secondary「供应商:」「状态:」,GH #55 局部约定沿置,GH #131 改词)。

## 数据与行为约束
- **轮询:** overview 10s `createVisibilityPoll`(隐藏降频 60s,回前台立即刷新;useOverview 持有);**局部刷新**——轮询只更新数据,列表/hero/指标区组件级重渲,不整页重载;首载 skeleton 只在「无数据且无错误」分支,轮询失败保留上次好数据 + 顶部 el-alert。
- **防作假:** hero 结论只反映全局 enabled 集合,统计范围恒注;null 不冒充(健康指数/可用率/延迟 null → 「暂无数据」/`-`);分享快照 = 打开时筛选后 entries 冻结,数字与范围 chips 同源(ui-guidelines §5 物料条)。
- **严重度组织:** severitySort 秩表供 Hero 结论/分区排序/同值回退(healthConclusion 同源);行排序主键经 modelList(GH #136,见上);已停用沉底。
- **statusFilter 单一来源:** 状态下拉是唯一状态筛选入口(旧 hero 计数行双控随信号墙退役)。

## 可访问性
- 可见页头 h1 承担 a11y 语义位(见页面构成);列表行键盘可达(Enter/Space + focus ring);面板 aria-modal + focus trap + 焦点归还;tooltip 不拦事件不进 tab 序。
- **reduced-motion:** 全局 transition 归零(semantics.css)+ 补间/图表 JS 门控(numberTween/chartMotion)+ 批次图标旋转组件级门控(AppSidebar)。

## 退役登记(旧世界组件,防复活)
HealthBanner / EndpointCard / OverviewGroupSection / UptimeStrip / EndpointQuickViewDialog / EndpointUptimePanel / LatencySparkline / AppHeader / PublicFooter——全部随 GH #112/#115 物理删除;其设计条目(信号墙灯与词分层、dotless 封闭清单、GAP=2px 共享常量、折叠披露、组级条级联算术)随旧世界作废,历史在 git。StatusBadge 的 `dotless` prop 存续但零消费方(ui-guidelines §5 登记)。

## 未决(另立批次)
- 状态点呼吸动效(spec §15 未落地,StatusBadge 注释登记);筛选 URL 深链;dots aria 等价信息。
