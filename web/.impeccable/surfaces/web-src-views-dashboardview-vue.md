---
version: 1
slug: "web-src-views-dashboardview-vue"
primary_target: "web/src/views/DashboardView.vue"
related_targets: ["web/src/components/HealthBanner.vue","web/src/components/OverviewGroupSection.vue","web/src/components/EndpointCard.vue","web/src/components/StatusBadge.vue","web/src/components/LatencySparkline.vue","web/src/components/AppHeader.vue","web/src/components/PublicFooter.vue"]
---

# 状态板 Dashboard — 表面简报

## 范围与模式
- 模式:**Operate**(公开只读监控面);route `/`,未登录可达。
- 读者:状态板读者(3 秒看懂,可能投屏/路过/远距)。任务:一眼判断「健不健康、谁异常、多严重」。
- 全球约定见 PRODUCT.md(读者模型)与 DESIGN.md(令牌/布局/暗色);本简报只登记页面级构成与组件规格。

## 页面构成(自上而下)
1. **AppHeader**(公开侧形态,见组件规格)
2. **HealthBanner** 全局健康横幅
3. **stats strip + 筛选行**(计数快捷过滤 + 关键词/协议/状态/分组控件 + 分享状态按钮)
4. **分组 sections**(OverviewGroupSection 标题行 + EndpointCard 矩阵)
5. **PublicFooter**

## 组件规格

### StatusBadge(唯一状态灯,迁自 ui-guidelines §5)
全站唯一 endpoint 状态展示组件,需要展示状态处一律复用,禁止第二个状态灯实现。四态圆点 + 状态词双编码;failing 为唯一动画(hs-blink)。**降级成因副标签**(ticket #7,spec 0013):可选 prop `causes?: DegradeCause[]`,非空且 status=degraded 时在状态词后行内渲染「· 可用性」「· 延迟」,双命中「· 可用性 + 延迟」(顺序固定,与后端 degrade_causes 一致)。副标签是 Badge 文字一部分:无独立圆点/图标/底色/动画;字号同档 sm,颜色 secondary。防御:causes 非空但 status≠degraded 不渲染;聚合场景(Dashboard 汇总条、分组头部、HealthBanner)永不传 causes——成因是端点粒度信息,聚合层不下钻。

### 24h 分段条(批 59 口径)
24 格填满式时间条:格 = 一小时,xs(2px)圆角,2px 间距。三档着色:≥95% success、<95% warning、0%(有探测且全失败)danger、无探测数据 border 灰;阈值同时适用于单元格、聚合可用率数字与明细行可用率列,不为任何单一场景另定分界线。聚合口径 = 按小时对齐求和 total/failures(探测加权,与后端一致),禁止按端点简单平均。

### LatencySparkline(EndpointCard 延迟曲线唯一组件,迁自 ui-guidelines §5)
24h 分段条下方同构曲线行,按小时桶 P50 绘制(仅统计成功探测;全失败桶分段条显红、曲线断线)。形态:纯 SVG polyline,不引 ECharts;行高 28px,每卡恒渲染(无数据时同高灰轨道占位)。**x 轴与分段条构造性对齐(硬约束):** 两行标签统一固定宽 26px、flex:none;SVG 经 ResizeObserver 实测 strip 像素宽,桶中心 x 由几何纯函数按 flex+gap 公式计算(slot=(W−23×GAP)/24);**GAP=2px 是 dots CSS 与 sparkline 几何的唯一共享常量,改动必须同步**;禁止固定比例 viewBox + preserveAspectRatio="none"。曲线:stroke secondary 1.5px round,孤立单点段渲染 r=1.5 圆点;null 桶断线分段;曲线下方面积浅填充 bg-hover 实心(填充随段断,孤立点不填);曲线中性色不承载状态语义。**量程:** 数据驱动 yMax = max(峰值×1.25, 1000ms 下限);降级阈值虚线(2×7天 P50 基线,warning 1px dasharray 4 3)**按需出现** ⟺ 阈值 ≤ yMax,不出现零残余指示,tooltip 恒兜底,不加迟滞。hover:strip 顶层 24 列透明 overlay 分列 tooltip;p50 null 桶按事实二分措辞(无探测→「无数据」;全失败→「探测全部失败,无延迟样本」)。几何抽 utils/latencySparkline.ts 纯函数(vitest 覆盖),组件只渲染。

### HealthBanner(全局健康横幅,GH #53 重构定稿)
构成两行:行 1 = [alert-dot(仅 failing,hs-blink 闪烁全横幅独占)] 大字结论(display 档)+ 右端元信息区([stale chip「数据非最新」]+「更新于 HH:mm · 每 10s 自动刷新」,xs/placeholder,margin-left:auto,溢出优先截断 cadence 段);行 2 = display 档 24h 可用率大数字(label「24h 可用率」xs,数字墨色 --hs-text-primary 不着色——banner 底色 + 结论已是双编码;null → 占位色「-」+「24h 内无探测数据」)+ 异常端点 chips 横排。四态:healthy 显结论 + 大数字(正向证据,无 chips);degraded / abnormal = 结论 + 大数字 + chips;空态(无数据)= 中性灰底 + 结论「暂无数据」+ 可用率「-」,不渲染 chips,永不读作全部正常;skeleton 固定高(min-height 104px)不跳。**异常 chips:** 集合 = 调用方 enabled entries 中 failing + down + degraded;排序走 #52 `SEVERITY_RANK`(utils/severitySort.ts 全站唯一秩表,经 `sortEntriesBySeverity` 复用,tie 按 model_id → protocol → endpoint_id,禁第二份秩表);上限 `MAX_ABNORMAL_CHIPS=5`,溢出「+N」(中性 placeholder,不可点);纯函数 `abnormalChips` 收 utils/healthConclusion.ts(输出仅 endpoint_id / model_id / protocol / status,函数内零文案字面量,vitest 覆盖)。chip 形态:**`--hs-bg-card` 表面底** + 描边 + radius-sm(2026-07-29 用户实机反馈修订:透明底 + 青灰描边在 tone 浅底上发白发冷「不搭」;表面底使 chip 在任何 tone 浅底读作「色板上的小卡片」,亮暗四 tone 构造性成立),状态词(语义色/600,failing 用 --hs-status-failing,down=danger,degraded=warning)+ 模型名(primary,截断 + title 全显「模型 · 协议 · 状态词」);**禁圆点、禁闪烁、禁状态底色**(W5 视觉镜像,闪烁由 alert-dot 独占;表面底是中性容器非状态填充);hover 仅描边转 brand。溢出「+N」**无框纯文字**(placeholder,不可点)——与表面底可点 chips 形态区分,不读作失效按钮。**chip 点击 = 复用 onBannerInspect(status) 状态过滤 + 滚动 matrix**(@click.stop 不触发整卡);banner 整卡点击保留(仅 abnormal 态,过滤到最紧急状态 failing>down)。**可用率口径:** `scopedAvailability(enabledEntries)`(statusCardSummary.ts 纯函数,探测加权 ok/total,与后端 overview 聚合构造性相等);`availability24h` / `enabledEndpoints` props 已随重构退役。**计数归 strip:** banner 不再渲染任何状态计数(告警/宕机/降级计数、共 N 个、另有 N 个正常、N 个已停用全部退役,stats strip 是唯一渲染处)。数据只反映全局,永不受页面过滤器影响;其他页面不得复刻其结论文案模式。

### EndpointCard(端点卡,GH #54 层级重构定稿)
矩阵卡片,自上而下:状态左边条 + 头行(模型名 + 协议 tag)+ 状态行(StatusBadge **md 档** + 评分徽章 + 已停用 tag)+ 三指标 + 24h 分段条 + LatencySparkline + 最近探测时间。整卡可点下钻 EndpointDetail。
- **模型名中间截断(splitMiddle,utils/truncate.ts 纯函数):** 双 span——head 吃 CSS ellipsis(min-width:0),tail(flex:none,默认保 12 字符)永不截断,截断由宽度驱动、禁 JS 字符预算;短名(len ≤ tailKeep+1)不拆,tail 为空;title 全显保留。后缀通常是最具区分度的版本/变体段,尾截断砍后缀已废。
- **状态行升格:** StatusBadge 用 md 档(size prop 'sm'|'md' 默认 sm,md = 词 md/600 + 圆点 12px;仅 EndpointCard 消费 md,其余消费方默认 sm;禁 :deep 覆写,不构成第二状态灯);成因副标签恒 sm/secondary 不随档升。
- **指标主从:** P50 主(lg/600 primary)、P95 次(sm/secondary)、24h 成功率 md 不变;顺序不变(成功率 / P50 / P95)。
- **评分徽章:** 有分恒显「评分 N」前缀(稳定性评分,非 formatScore 管辖);「暂无评分」不变。
- **协议 tag 组内同值收敛(GH #34 映射不动):** OverviewGroupSection 算 uniformProtocol(筛选后 entries 全部 protocol 相等且非空),同值时组头身份区(group-count 之后)渲染一枚 el-tag(protocolTagType, size small),卡片收 `showProtocolTag=false`(prop 默认 true);**三例外/边界:** grouping='protocol' 时组名即协议——组头不渲染 tag、卡片 tag 同样收敛;flat 模式不收敛(卡片保留 tag);混搭组/空组不收敛。EndpointTable / EndpointDetailView / StatusCard 静态物料(明细行「模型 · 协议」)不动。
- **卡片网格:** minmax(300px, 1fr)(原 260px,4 列→3 列),OverviewGroupSection 与 DashboardView flat 两处 .card-grid 同步;sparkline 经 ResizeObserver 重算,卡宽变化不破 x 轴构造性对齐(.dots-strip 2px gap 与 .dots-label 26px 宽仍是共享常量,不动)。

### OverviewGroupSection(分组 section)
标题行:折叠箭头 + 组名 + 端点计数 + 状态计数 chips + 组聚合指标(24h 可用率/均延)+ 分享入口。整行可点折叠。**分组独立分享入口**(批 59):标题行右端 text 型按钮(Share 图标 + 「分享」文字),@click.stop 不触发折叠;复用 StatusShareDialog,快照范围 = 该分组条目 ∩ 当前页面筛选,scope chips 首位恒为分组 chip(label「分组」,值「厂商/能力/协议 · 组名」);**卡片所有数字一律从快照 entries(enabled)计算,与范围 chips 恒一致**——24h 可用率 = 快照 entries dots_24h 按小时求和 ok/total;平均延迟 = enabled entries p50_ms 均值(唯一 scope 恒一致口径;与组头「均延」探测加权值可能略异,卡片内部自洽优先)。

### AppHeader(公开侧形态)
导航按登录态过滤:未登录只渲染公开页项(状态总览 + 评估榜单→/board);登录态 = 状态总览 + 评估榜单(→/eval)+ 任务中心,随路由切换重检。**未登录 header 一律不渲染登录按钮**(ticket 90 裁决:醒目登录按钮传递「内容要账号」错误信号;判定走 route meta.public),登录入口统一由 PublicFooter 承担。右栏:亮/暗切换(未登录可用,localStorage `hs:dark`,默认亮不跟随系统)+ 登录态时批次进度入口(仅存在未完成批次时渲染,3s 轮询 settle 即停,「批次运行中 X/Y」点击跳 /eval;禁用橙与闪烁)+ 角色 tag(集中映射 utils/role.ts,primary=管理权/info=非管理,语义=权限层级非健康度)。

### PublicFooter(公开页管理入口唯一组件)
hairline + 一行左右分置:左 © 版权,右「管理登录」→ /login(xs placeholder,链接 hover brand)。状态总览、EndpointDetail、/board 三页一律复用;/login 页不渲染;登录态照常渲染。豁免:/report/:token 分享页不挂页脚。

## 数据与行为约束
- **防作假:** 任何汇总结论必须标注统计范围;筛选快照不得引用未筛选聚合字段;空态中性,永不读作「全部正常」。
- **轮询:** overview 走 utils/visibilityPoll.ts(10s,标签页隐藏降频 60s,回前台立即刷新),卸载必清理。
- **状态排序口径(GH #55 定稿):** stats strip 与组头共享同一严重度序,重→轻(failing>down>degraded>healthy);单一来源 = `utils/severitySort.ts` 的 `SEVERITY_ORDER` 数组(与 `SEVERITY_RANK` 秩序一致,vitest 断言守护),strip 原轻→重 `STRIP_ORDER` 与组头 `STATUS_PRIORITY` 两处本地口径已删除。
- **首屏严重度组织(GH #52 登记):** 状态板组间/组内/flat 三处统一走 `utils/severitySort.ts` 的严重度秩(`SEVERITY_RANK`:failing>down>degraded>healthy,全站唯一来源,StatusCard 异常明细同引);**口径一律按筛选后 entries 计算**。组间秩 = 组内 enabled entries 的最小秩,tie 按组键字典序(`<`,不用 localeCompare);组内秩升序,tie 按 model_id → protocol → endpoint_id;flat 模式同经 `sortEntriesBySeverity`。**已停用端点(`DISABLED_RANK`)恒沉底**——disabled 的 down/failing 不抬组秩、不参与首屏竞争;**筛选后空组沉底**(组键字典序,空 hint 现行为保留)。轮询/筛选引起的数据驱动重排不做动画(与榜单行重排同纪律)。
- **statusFilter 双控(GH #55 登记,有意为之):** stats strip 状态项点击(再点取消)与筛选行状态下拉绑定同一 `statusFilter` ref,双向同步构造性成立;strip 是快捷路径,banner inspect 写同一 ref;三者共享单一过滤源。选中态 = `--hs-brand-soft` 浅底 + brand 文字 + radius-sm(脱离下划线语言——原 2px brand 下划线读作导航 tab 而非「已选过滤条件」,且与导航 tab 语言撞车)。
- **「本组」前缀(GH #55):** 组头 group-metrics 以「本组:」容器级前缀一次统领 24h 可用率与均延两项,与 HealthBanner 的全局口径区分归属,两个可用率不再裸名并列。
- **筛选行下拉内联 label(GH #55,Dashboard 局部约定,不推广):** 协议/状态两个 select 前置内联 label(sm/secondary「协议:」「状态:」,与 select 同行),不再以 placeholder 兼任 label;关键词输入框与分组 select 不动。

## 体检基线与已排改进
- critique 基线 22/36(2026-07-29,快照 .impeccable/critique/):严重度不驱动首屏、banner/strip 信息重复、卡片墙均质化(P1);排序口径两套、下拉 placeholder 当 label(P2)。
- 已排票:#52 severitySort(**已完成 2026-07-29**,约定见「数据与行为约束」)/ #53 HealthBanner 重构(**已完成 2026-07-29**,约定见「组件规格」HealthBanner 节)/ #54 卡片层级重构(**已完成 2026-07-29**,约定见「组件规格」EndpointCard 节)/ #55 一致性批(**已完成 2026-07-29**,约定见「数据与行为约束」状态排序口径/双控/「本组」前缀/下拉 label 各条)。

## 未决(另立批次)
- a11y(click-only 主交互键盘可达、dots aria 等价)、URL 深链(筛选进 query)、非均质矩阵方向(异常卡大/健康卡小,Provocative Q3 未裁决)。
