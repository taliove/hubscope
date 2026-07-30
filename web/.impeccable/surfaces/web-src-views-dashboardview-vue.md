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
构成两行:行 1 = [alert-dot(仅 failing,hs-blink 闪烁全横幅独占)] 大字结论(display 档)+ 右端元信息区([stale chip「数据非最新」]+「更新于 HH:mm · 每 10s 自动刷新」,xs/placeholder,margin-left:auto,溢出优先截断 cadence 段);行 2 = display 档 24h 可用率大数字(label「24h 可用率」xs,数字墨色 --hs-text-primary 不着色——banner 底色 + 结论已是双编码;null → 占位色「-」+「24h 内无探测数据」)+ 异常端点 chips 横排。四态:healthy 显结论 + 大数字(正向证据,无 chips);degraded / abnormal = 结论 + 大数字 + chips;空态(无数据)= 中性灰底 + 结论「暂无数据」+ 可用率「-」,不渲染 chips,永不读作全部正常;skeleton 固定高(min-height 104px)不跳。**异常 chips:** 集合 = 调用方 enabled entries 中 failing + down + degraded;排序走 #52 `SEVERITY_RANK`(utils/severitySort.ts 全站唯一秩表,经 `sortEntriesBySeverity` 复用,tie 按 model_id → protocol → endpoint_id,禁第二份秩表);上限 `MAX_ABNORMAL_CHIPS=5`,溢出「+N」(中性 placeholder,不可点);纯函数 `abnormalChips` 收 utils/healthConclusion.ts(输出仅 endpoint_id / model_id / protocol / status,函数内零文案字面量,vitest 覆盖)。chip 形态:**`--hs-bg-card` 表面底、无描边** + radius-sm(2026-07-29 两轮用户实机反馈定稿:透明底发白发冷、带描边仍丑;无描边表面底在任何 tone 浅底读作干净小卡片,亮暗四 tone 构造性成立,零新色),**hover 浮起 `--hs-shadow-md`**(阴影=可点的高度语义),状态词(语义色/600,failing 用 --hs-status-failing,down=danger,degraded=warning)+ 模型名(primary,截断 + title 全显「模型 · 协议 · 状态词」);**禁圆点、禁闪烁、禁状态底色**(W5 视觉镜像,闪烁由 alert-dot 独占;表面底是中性容器非状态填充);hover 仅描边转 brand。溢出「+N」**无框纯文字**(placeholder,不可点)——与可点 chips 形态区分,不读作失效按钮。**chip 点击 = 复用 onBannerInspect(status) 状态过滤 + 滚动 matrix**(@click.stop 不触发整卡);banner 整卡点击保留(仅 abnormal 态,过滤到最紧急状态 failing>down)。**可用率口径:** `scopedAvailability(enabledEntries)`(statusCardSummary.ts 纯函数,探测加权 ok/total,与后端 overview 聚合构造性相等);`availability24h` / `enabledEndpoints` props 已随重构退役。**计数归 strip:** banner 不再渲染任何状态计数(告警/宕机/降级计数、共 N 个、另有 N 个正常、N 个已停用全部退役,stats strip 是唯一渲染处)。数据只反映全局,永不受页面过滤器影响;其他页面不得复刻其结论文案模式。

### EndpointCard(端点卡,GH #54 层级重构定稿)
矩阵卡片,自上而下:状态左边条 + 头行(模型名 + 协议 tag)+ 状态行(StatusBadge **md 档** + 评分徽章 + 已停用 tag)+ 三指标 + 24h 分段条 + LatencySparkline + 最近探测时间。**整卡可点开启 EndpointQuickViewDialog 速览弹窗**(2026-07-29 修订,取代直接下钻);深链下钻由弹窗内「打开完整详情」主按钮承担,/endpoints/:id 深链页不动。24h 分段条 + LatencySparkline 两行重构为消费 EndpointUptimePanel。
- **模型名中间截断(splitMiddle,utils/truncate.ts 纯函数):** 双 span——head 吃 CSS ellipsis(min-width:0),tail(flex:none,默认保 12 字符)永不截断,截断由宽度驱动、禁 JS 字符预算;短名(len ≤ tailKeep+1)不拆,tail 为空;title 全显保留。后缀通常是最具区分度的版本/变体段,尾截断砍后缀已废。
- **状态行升格:** StatusBadge 用 md 档(size prop 'sm'|'md' 默认 sm,md = 词 md/600 + 圆点 12px;仅 EndpointCard 消费 md,其余消费方默认 sm;禁 :deep 覆写,不构成第二状态灯);成因副标签恒 sm/secondary 不随档升。
- **指标主从:** P50 主(lg/600 primary)、P95 次(sm/secondary)、24h 成功率 md 不变;顺序不变(成功率 / P50 / P95)。
- **评分徽章:** 有分恒显「评分 N」前缀(稳定性评分,非 formatScore 管辖);「暂无评分」不变。
- **协议 tag 组内同值收敛(GH #34 映射不动):** OverviewGroupSection 算 uniformProtocol(筛选后 entries 全部 protocol 相等且非空),同值时组头身份区(group-count 之后)渲染一枚 el-tag(protocolTagType, size small),卡片收 `showProtocolTag=false`(prop 默认 true);**三例外/边界:** grouping='protocol' 时组名即协议——组头不渲染 tag、卡片 tag 同样收敛;flat 模式不收敛(卡片保留 tag);混搭组/空组不收敛。EndpointTable / EndpointDetailView / StatusCard 静态物料(明细行「模型 · 协议」)不动。
- **卡片网格:** minmax(300px, 1fr)(原 260px,4 列→3 列),OverviewGroupSection 与 DashboardView flat 两处 .card-grid 同步;sparkline 经 ResizeObserver 重算,卡宽变化不破 x 轴构造性对齐(.dots-strip 2px gap 与 .dots-label 26px 宽仍是共享常量,不动)。

### OverviewGroupSection(分组 section)
折叠箭头用 EP `ArrowDown/ArrowRight` 图标(2026-07-29 用户反馈,取代文本三角形 ▾/▸);**筛选后空组自动折叠**(2026-07-29 用户裁决:空组不再渲染大空态盒,匹配恢复即自动展开,两态均可手动折叠)。
**折叠披露过渡(2026-07-29 设计评审,克制版动效,/impeccable animate):** 卡片矩阵容器走 ui-guidelines §6 披露容器三件套(grid `0fr→1fr` + 内层 `min-height:0/overflow:hidden` + `visibility` 延迟切换:折叠向 `0s 0.2s`、展开向 `0s 0s`),时长 `--hs-transition`;visibility 保证折叠态退出 tab 序与 a11y 树(与 v-show 等价,a11y harden 成果不回退),不支持 grid 轨道动画的浏览器退化为瞬切。折叠箭头改单图标(ArrowRight)`transform: rotate(0↔90deg)` 过渡,取代 ArrowRight/ArrowDown 双图标瞬切;终态语义不变。**仅用户点击触发过渡;watch(entries.length) 的筛选空组自动折叠/恢复展开走 no-motion 瞬切**(延伸 GH #52 数据驱动不动画纪律,防筛选连续击键时多组同时高度补间噪音)。
标题行:折叠箭头 + 组名 + 端点计数 + 状态计数 chips + 组聚合指标(24h 可用率/均延)+ 分享入口。整行可点折叠。**分组独立分享入口**(批 59):标题行右端 text 型按钮(Share 图标 + 「分享」文字),@click.stop 不触发折叠;复用 StatusShareDialog,快照范围 = 该分组条目 ∩ 当前页面筛选,scope chips 首位恒为分组 chip(label「分组」,值「厂商/能力/协议 · 组名」);**卡片所有数字一律从快照 entries(enabled)计算,与范围 chips 恒一致**——24h 可用率 = 快照 entries dots_24h 按小时求和 ok/total;平均延迟 = enabled entries p50_ms 均值(唯一 scope 恒一致口径;与组头「均延」探测加权值可能略异,卡片内部自洽优先)。

### EndpointQuickViewDialog(端点速览弹窗,2026-07-29 设计评审;2026-07-30 morph + 明细曲线修订)
Dashboard 卡片点击开启的轻量速览(el-dialog,640px + max-width 92vw,**align-center 垂直居中**(2026-07-30 修订,取代 EP 默认 15vh 顶距——终态在屏幕正中,与 morph「弹出到正中」语义一致;EP 2.14 per-instance prop,仅本弹窗,其他弹窗定位语义不变),radius-lg,shadow-lg 浮层语义,消费页密度)。
- **FLIP morph 编舞(2026-07-30 修订,取代原地翻入/翻出;用户实机反馈「要的是翻转、变大、弹出」):** 开(≤460ms 预算不变)= 卡片 rotateY 0→-90° 翻出(0.2s default 档)→ 140ms 起雾化浮层淡入 + 弹窗**从被点卡片的屏幕矩形出发 morph 进场**(0.32s focal 档):enter-from = `translate(dx,dy) scale(s) perspective(1600px) rotateY(90deg)`(**函数顺序固定**:位移与缩放在父坐标系像素精确,perspective+rotate 在元素本地系;变换原点 50% 50%),enter-to = 恒等——位移/缩放/翻转三段同拍,视觉 = 卡片放大飞入正中并翻正。几何纯函数收 utils/quickViewChoreo.ts(vitest 钉死,含卡片缺失回退分支):dx/dy = 卡片中心 − 弹窗终态中心,s = cardW/dialogW(**统一缩放,禁非均匀拉伸**)。测量时机:卡片矩形**点击时同步实测,必须先于 cardFlipped 置位**(旋转中的 projected rect 是塌缩条);弹窗终态矩形**挂载后、Vue enter 双 rAF 移除 enter-from 前实测**(EP @open/nextTick 间隙注入,不做高度预测)。动态变换走 CSS 变量:`--qv-from`(enter)/`--qv-to`(leave)注入弹窗根内联样式,ep-theme.css 的 hs-flip 类消费,fallback = 原地翻转(变量缺省即现状形态)。关 = 严格镜像倒带:leave 起点重测卡片矩形——卡片处于 -90° 翻转态,用**未旋转矩形还原法**(rotateY 绕自身中心纵轴,投影中心与高不变:cx/cy/h 取 getBoundingClientRect,w 取 offsetWidth,x = cx − w/2),禁直接用 projected rect;卡片已离开 DOM(筛选变更/分组切换/端点删除)或完全滚出视口 → 不设 `--qv-to`,退化为原地翻出。两段透视不共享消失点(卡片 = .card-grid 祖先 perspective,弹窗 = transform 函数内联 perspective)登记为**可接受**:±90° 边缘态投影宽度为零,交接穿帮被几何掩盖,连续性由位置/缩放锚点承担;stagger 140ms 不变(交接时卡片 ≈-68° 已近边缘态,弹窗从 +90° 起步,双条同位同尺度,角度差不构成可见跳变)。**enter 类早摘修补(同批):** 弹窗 transform 过渡声明从 `.hs-flip-enter-active .el-dialog` 提升为静态恒挂(`.hs-quickview-dialog` 级)——Vue 按 overlay 根 opacity(0.2s)判定 enter 结束并摘除 enter 类,子元素 0.32s 过渡在 ~62% 处被截断(纯翻转时被 front-loaded 缓动掩盖,morph 的位移/缩放末段截断可见,一并修补);reduced-motion 全局归零与 JS 门控不变。**async 区定高:** 两个异步区(延迟明细、最近失败)骨架与终态同高——弹窗打开高度确定(morph 锚点精度),且 align-center 下异步加载不引发双向重心跳动。
- **雾化浮层:** modal-class 走 ep-theme.css 全局块,--hs-overlay-bg 衬底 + backdrop-filter blur 8px(专属例外,管理台弹窗不雾化)。
- **内容(零等待):** 首帧全部由打开时冻结的 entry 快照渲染——StatusBadge md + causes、协议 tag、模型名、三指标、EndpointUptimePanel;「最近失败」区异步拉 listProbeHistory top 5(ProbeRecordTable `slim` 变体:隐藏 类型/HTTP/TTFT/输入token/输出token 五列,保 结果/错误摘要/延迟/时间,列宽合计 ≈550px 算术贴合 600px 内容宽——2026-07-29 main 裁决,评审就绪文本「:compact="false"」全列 ≈1010px 与 640px 弹窗自相矛盾,§4 禁横向滚动为承重纪律;empty-text 走 prop,本区传「暂无失败记录」——该区只查 ok=false,共享固定文案「暂无探测记录」语义不准),skeleton/错误重试三态,失败只影响该区。底部「打开完整详情」主按钮 router.push /endpoints/:id。
- **「24h 延迟明细」区(2026-07-30 新增,用户实机反馈「Modal 里也要显示更细节的曲线」;语义层约束见 ui-guidelines §5 本组件条目):** 位于 EndpointUptimePanel(小时级,保留——「也要」是加不是换)下方、最近失败区上方。组件 = **新迷你裸图表组件**(ECharts 既有栈 + useChartColors 镜像,TrendChart「裸图表布局由父级负责」先例;TimeSeriesChart 是 el-card 封装 + category 轴契约,不适配,不复用);数据变换(records → 线/散点系列,接口倒序 → 绘图正序)抽纯函数,vitest 覆盖 空/全失败/单点/倒序输入。图表高 180px,宽随弹窗内容宽;**scatter 需在 utils/echarts.ts 增注册 ScatterChart**(注册表注释同步,模块化体积纪律不破)。三态与最近失败区同款(skeleton/错误重试/数据,失败只影响本区),打开时一次性拉取、随打开冻结(快照纪律延伸)。**数据路径(契约变更,2026-07-30 评审登记):** `GET /api/endpoints/{id}/probes` 增可选 `hours` 开窗参数(窗口查询行帽 2000;默认探测周期 300s 下 24h ≈ 288 轮 ×chat 双记录 = 576 条,载荷 ≈150KB;现状 limit 钳 [1,200] 拿不到 24h 全量,这是本次必须动后端的原因),api-contract.md 同步 + W1 黑盒测试(hours 边界/行帽截断/ok+hours 组合/倒序不变);既有 limit/ok 语义与三处存量调用方(listProbeHistory × EndpointDetailView/EndpointTable/本弹窗最近失败区)零改动。高频探测端点(周期 <300s)超行帽时按最新 2000 条截断,覆盖区间由时间轴自明,不做静默「24h」宣称。
- **快照冻结:** 弹窗内容不跟随 overview 轮询更新(与 StatusCard 快照同哲学);延迟明细区同为打开一次性拉取;实时数据走完整详情。
- **排除项:** 不放分享入口(防弹窗叠弹窗)。
- **关闭:** ESC/浮层/按钮/路由跳转统一走 EP @closed 单点复位 flipCardId;关闭后焦点归还触发卡片(EP focus-trap + 手动 focus 双保险)。

### AppHeader(公开侧形态)
导航按登录态过滤:未登录只渲染公开页项(状态总览 + 评估榜单→/board);登录态 = 状态总览 + 评估榜单(→/eval)+ 任务中心,随路由切换重检。**未登录 header 一律不渲染登录按钮**(ticket 90 裁决:醒目登录按钮传递「内容要账号」错误信号;判定走 route meta.public),登录入口统一由 PublicFooter 承担。右栏:亮/暗切换(未登录可用,localStorage `hs:dark`,默认亮不跟随系统)+ 登录态时批次进度入口(仅存在未完成批次时渲染,3s 轮询 settle 即停,「批次运行中 X/Y」点击跳 /eval;禁用橙与闪烁)+ 角色 tag(集中映射 utils/role.ts,primary=管理权/info=非管理,语义=权限层级非健康度)。

### PublicFooter(公开页管理入口唯一组件)
hairline + 一行左右分置:左 © 版权,右「管理登录」→ /login(xs placeholder,链接 hover brand)。状态总览、EndpointDetail、/board 三页一律复用;/login 页不渲染;登录态照常渲染。豁免:/report/:token 分享页不挂页脚。

## 数据与行为约束
- **防作假:** 任何汇总结论必须标注统计范围;筛选快照不得引用未筛选聚合字段;空态中性,永不读作「全部正常」。
- **轮询:** overview 走 utils/visibilityPoll.ts(10s,标签页隐藏降频 60s,回前台立即刷新),卸载必清理。
- **状态排序口径(GH #55 定稿):** stats strip 与组头共享同一严重度序,重→轻(failing>down>degraded>healthy);单一来源 = `utils/severitySort.ts` 的 `SEVERITY_ORDER` 数组(与 `SEVERITY_RANK` 秩序一致,vitest 断言守护),strip 原轻→重 `STRIP_ORDER` 与组头 `STATUS_PRIORITY` 两处本地口径已删除。
- **首屏严重度组织(GH #52 登记):** 状态板组间/组内/flat 三处统一走 `utils/severitySort.ts` 的严重度秩(`SEVERITY_RANK`:failing>down>degraded>healthy,全站唯一来源,StatusCard 异常明细同引);**口径一律按筛选后 entries 计算**。组间秩 = 组内 enabled entries 的最小秩,tie 按组键字典序(`<`,不用 localeCompare);组内秩升序,tie 按 model_id → protocol → endpoint_id;flat 模式同经 `sortEntriesBySeverity`。**已停用端点(`DISABLED_RANK`)恒沉底**——disabled 的 down/failing 不抬组秩、不参与首屏竞争;**筛选后空组沉底**(组键字典序,空 hint 现行为保留)。轮询/筛选引起的数据驱动重排不做动画(与榜单行重排同纪律)。同纪律延伸至披露动作:数据/筛选驱动的分组折叠与恢复不做动画(no-motion 双轨,机制见「组件规格」OverviewGroupSection 节)。
- **速览弹窗快照冻结(2026-07-29):** 打开即冻结 entry 快照,轮询不更新弹窗内容;翻转是用户触发单次过渡,与 GH #52 数据驱动不动画纪律兼容(同折叠披露条款口径)。
- **statusFilter 双控(GH #55 登记,有意为之):** stats strip 状态项点击(再点取消)与筛选行状态下拉绑定同一 `statusFilter` ref,双向同步构造性成立;strip 是快捷路径,banner inspect 写同一 ref;三者共享单一过滤源。选中态 = 1px brand 内嵌描边(box-shadow inset,零布局位移)+ 透明底 + brand 文字(2026-07-29 用户实机反馈定稿,取代 GH #55 的 brand-soft 浅底——浅底色块与状态点语义色打架;brand 描边保留「激活选择」语言且零色块;原 2px 下划线读作导航 tab 的撞车问题保持已解)。
- **「本组」前缀(GH #55):** 组头 group-metrics 以「本组:」容器级前缀一次统领 24h 可用率与均延两项,与 HealthBanner 的全局口径区分归属,两个可用率不再裸名并列。
- **筛选行下拉内联 label(GH #55,Dashboard 局部约定,不推广):** 协议/状态两个 select 前置内联 label(sm/secondary「协议:」「状态:」,与 select 同行),不再以 placeholder 兼任 label;关键词输入框与分组 select 不动。

## 可访问性(2026-07-29 harden 批)
- **四处主交互键盘可达(audit P1/P2,WCAG 2.1.1):**
  - **stats strip 可点项(总数 + 四状态项)** → 真 `<button type="button">`(font/color inherit、background/border none,padding 不变);`:focus-visible` = 1px brand inset ring(与 stat-active 同语言)。点击过滤/再点取消行为不变。
  - **组头折叠(OverviewGroupSection)** → 全宽 `<button type="button">`(text-align:left)+ `aria-expanded`;**分享按钮移出为兄弟节点**——`<button>` 内不得嵌套 `<button>`(原结构里 group-share 嵌在组头内,button 化必须拆出,视觉位置不变)。`:focus-visible` 同上 ring。
  - **HealthBanner 整卡** → 仅 abnormal 可点态根节点挂 `role="button" tabindex="0"` + Enter/Space 触发同一 inspect;非可点态无 role 不进 tab 序。`:focus-visible` = brand inset ring + shadow-md(与 chips 同语言)。chips 已是真 button。
  - **EndpointCard 整卡开速览** → el-card 根 `role="button" tabindex="0"` + `aria-haspopup="dialog"` + Enter/Space 触发开启(2026-07-29 修订:同页开弹窗是 button 语义,取代 link);`:focus-visible` = 1px brand inset ring(box-shadow,与 3px 状态左边条共存,零布局位移)。卡内无嵌套交互(仅 tooltip);弹窗关闭后焦点归还本卡(手动 focus 兜底,防轮询重渲致 EP 焦点归还落空)。
- **焦点语言全板统一:** 1px brand inset ring(`box-shadow: inset 0 0 0 1px var(--hs-brand)` + `outline: none`),不引入第二种焦点样式;ring 只在焦点态出现,不改变任何静态视觉。
- **语义 h1:** DashboardView 挂视觉隐藏 h1「HubScope 服务状态总览」(标准 sr-only 模式:absolute 1px + clip,禁 display:none——那会把它从 a11y 树里删掉),零视觉变化。
- **reduced-motion 替代(WCAG 2.3.3,前庭敏感):** failing 闪烁动画令牌化为 `--hs-blink`(semantics.css 唯一定义,StatusBadge failing 点与 HealthBanner alert-dot 两处消费);`@media (prefers-reduced-motion: reduce)` 下 `--hs-blink: none`——**闪烁在 reduced-motion 下静止,状态由实心橙 + 状态词双编码承载(动画是增强,从不是唯一通道)**。HealthBanner skeleton 的 pulse 动画沿用组件内既有 reduced-motion 豁免,保持不动。**全局过渡归零(2026-07-29 /impeccable animate 批,main 裁决):** semantics.css 同一 media 块内追加全局 `transition: none !important`——折叠高度/箭头旋转等一切用户触发过渡在 reduced-motion 下瞬时完成,一处覆盖新折叠 + LoginView 验证码区存量(首例原无 reduced-motion 处理,本批连带修补,登记不留暗账)+ 未来同类;blink 由令牌归零、pulse 由组件豁免,均不受该规则影响。`onBannerInspect` 的 `scrollIntoView` 对 reduced-motion 降级为 `behavior: 'auto'`(修补 a11y harden 批漏网)。

## 体检基线与已排改进
- critique 基线 22/36(2026-07-29,快照 .impeccable/critique/):严重度不驱动首屏、banner/strip 信息重复、卡片墙均质化(P1);排序口径两套、下拉 placeholder 当 label(P2)。
- 已排票:#52 severitySort(**已完成 2026-07-29**,约定见「数据与行为约束」)/ #53 HealthBanner 重构(**已完成 2026-07-29**,约定见「组件规格」HealthBanner 节)/ #54 卡片层级重构(**已完成 2026-07-29**,约定见「组件规格」EndpointCard 节)/ #55 一致性批(**已完成 2026-07-29**,约定见「数据与行为约束」状态排序口径/双控/「本组」前缀/下拉 label 各条)。

## 未决(另立批次)
- dots aria 等价(24h 分段条的屏幕阅读器等价信息)、URL 深链(筛选进 query)、非均质矩阵方向(异常卡大/健康卡小,Provocative Q3 未裁决)。a11y harden 批(键盘可达/h1/reduced-motion)已完成 2026-07-29,约定见「可访问性」节。
