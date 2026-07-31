---
version: 1
slug: "web-src-views-dashboardview-vue"
primary_target: "web/src/views/DashboardView.vue"
related_targets: ["web/src/components/HealthBanner.vue","web/src/components/OverviewGroupSection.vue","web/src/components/EndpointCard.vue","web/src/components/StatusBadge.vue","web/src/components/LatencySparkline.vue","web/src/components/EndpointUptimePanel.vue","web/src/components/EndpointQuickViewDialog.vue","web/src/components/ProbeRecordTable.vue","web/src/components/AppHeader.vue","web/src/components/PublicFooter.vue"]
---

# 状态板 Dashboard — 表面简报

## 范围与模式
- 模式:**Operate**(公开只读监控面);route `/`,未登录可达。
- 读者:状态板读者(3 秒看懂,可能投屏/路过/远距)。任务:一眼判断「健不健康、谁异常、多严重」。
- 全球约定见 PRODUCT.md(读者模型)与 DESIGN.md(令牌/布局/暗色);本简报只登记页面级构成与组件规格。

## 页面构成(自上而下;2026-07-30 GH #69 批骨架重组)
1. **AppHeader**(公开侧形态,见组件规格)
2. **Hero 指挥台带**(HealthBanner 演进:大字结论 + 异常 chips + 计数行 + 24h 可用率合一,单带 hairline 收边,取代原 banner 卡 + stats strip 两段)
3. **筛选工具条**(关键词/协议/状态/分组 + 分享状态按钮,细一行)
4. **分组 sections**(OverviewGroupSection 标题行 + hairline + EndpointCard 矩阵)
5. **PublicFooter**

## 组件规格

### StatusBadge(唯一状态灯,迁自 ui-guidelines §5)
全站唯一 endpoint 状态展示组件,需要展示状态处一律复用,禁止第二个状态灯实现。四态圆点 + 状态词双编码;failing 为唯一动画(hs-blink)。**词随灯着色(2026-07-30,GH #71,shape 定稿):** 状态词颜色从 `--hs-text-regular` 改为状态语义色——degraded=`--hs-warning`、down=`--hs-danger`、failing=`--hs-status-failing`(本体,白底 4.92/4.8/5.1 均过 AA)、healthy=`--hs-success-text`(亮 #047857 5.48:1 / 暗 #10b981 6.94:1;本体 #059669 白底 3.77 不过 AA,深阶化即 GH #62 落点,语义裁决见 ui-guidelines 附录 B #15)。圆点尺寸微调:sm 10→9px、md 12→11px。全站消费方(hero 带计数行、组头、详情页、速览弹窗、管理台表格)零改动继承(GH #71 时点——实机迭代批起,计数行/组头/卡片状态行三处按 dotless 变体去点,见本节末段),不构成第二状态灯。**dotless 变体(2026-07-30 实机迭代批 GH #80,shape 定稿;GH #81/#82):** 词随灯着色后,着色词自身即色+词双编码的文本形态——聚合/重复场景允许去点渲染(仅着色词),防一屏多灯稀释信号(用户实机反馈「灯看花眼」)。封闭适用清单(仅三处,禁止扩散):① EndpointCard 状态行(头行信号墙灯是全卡唯一状态灯);② Hero 带计数行;③ 组头计数 chips。详情页与速览弹窗的 Badge 保留点(实体信号位,该对象的首要状态信号,非重复场景);管理台表格等其余消费方保持现状不动。prop 形态由实现定(建议布尔 prop,默认带点),规范只登记语义;failing dotless 词不闪烁(闪烁是灯的语言,闪烁位置封闭清单见 DESIGN.md「信号墙灯与词分层」节)。**a11y 无回退:** 状态词本来就是 Badge 的可访问名,圆点是无词装饰(头灯 aria-hidden),dotless 与带点的 a11y 树等价。
**降级成因副标签**(ticket #7,spec 0013):可选 prop `causes?: DegradeCause[]`,非空且 status=degraded 时在状态词后行内渲染「· 可用性」「· 延迟」,双命中「· 可用性 + 延迟」(顺序固定,与后端 degrade_causes 一致)。副标签是 Badge 文字一部分:无独立圆点/图标/底色/动画;字号同档 sm,颜色 secondary(不随词着色)。防御:causes 非空但 status≠degraded 不渲染;聚合场景(Dashboard 计数行、分组头部、Hero 带)永不传 causes——成因是端点粒度信息,聚合层不下钻。

### 24h 分段条(批 59 口径)
24 格填满式时间条:格 = 一小时,xs(2px)圆角,2px 间距。三档着色:≥95% success、<95% warning、0%(有探测且全失败)danger、无探测数据 border 灰;阈值同时适用于单元格、聚合可用率数字与明细行可用率列,不为任何单一场景另定分界线。聚合口径 = 按小时对齐求和 total/failures(探测加权,与后端一致),禁止按端点简单平均。

### LatencySparkline(EndpointCard 延迟曲线唯一组件,迁自 ui-guidelines §5)
24h 分段条下方同构曲线行,按小时桶 P50 绘制(仅统计成功探测;全失败桶分段条显红、曲线断线)。形态:纯 SVG polyline,不引 ECharts;行高 28px,每卡恒渲染(无数据时同高灰轨道占位)。**x 轴与分段条构造性对齐(硬约束):** 两行标签统一固定宽 26px、flex:none;SVG 经 ResizeObserver 实测 strip 像素宽,桶中心 x 由几何纯函数按 flex+gap 公式计算(slot=(W−23×GAP)/24);**GAP=2px 是 dots CSS 与 sparkline 几何的唯一共享常量,改动必须同步**;禁止固定比例 viewBox + preserveAspectRatio="none"。曲线:stroke secondary 1.5px round,孤立单点段渲染 r=1.5 圆点;null 桶断线分段;曲线下方面积浅填充 bg-hover 实心(填充随段断,孤立点不填);曲线中性色不承载状态语义。**量程:** 数据驱动 yMax = max(峰值×1.25, 1000ms 下限);降级阈值虚线(2×7天 P50 基线,warning 1px dasharray 4 3)**按需出现** ⟺ 阈值 ≤ yMax,不出现零残余指示,tooltip 恒兜底,不加迟滞。hover:strip 顶层 24 列透明 overlay 分列 tooltip;p50 null 桶按事实二分措辞(无探测→「无数据」;全失败→「探测全部失败,无延迟样本」)。几何抽 utils/latencySparkline.ts 纯函数(vitest 覆盖),组件只渲染。

### Hero 指挥台带(2026-07-30,GH #73,shape 定稿;由 HealthBanner 原地演进,取代 GH #53 banner 卡 + stats strip 两段)
单表面指挥带 = 全局结论 + 异常 chips + 状态计数 + 24h 可用率合一。**带形态:** 内容列全宽平带,无圆角、无描边盒,底部 1px `--hs-border-light` hairline 收边;上下内边距 `--hs-space-4`(16px),左右 `--hs-space-4`(浅底色块内文字不吃边,与卡片内边距同节奏)。带底沿用 GH #53 tone-soft 四态(healthy=success-soft / degraded=warning-soft / abnormal=danger-soft;空态与首载 skeleton=中性 bg-page,永不读作全部正常)——plan 评审裁决(2026-07-30):中性无 tint 方案被否,tone-soft 是严重度双编码的一部分(信号墙「整墙点亮」语义),且 chips 的 bg-card 表面底在浅底上构造性成立。
**构图(2026-07-30 实机迭代批修订,用户实机反馈「可用率沉底」;取代「左列纵排 + 右列垂直居中沉底」两列构图,GH #81):**
- **行 1:** [alert-dot 仅 failing,hs-blink 闪烁全带独占] + 大字结论 display 档(tone 着色)+ 可用率大数字(display 档,行右端 `margin-left:auto`,与结论同基线;墨色 `--hs-text-primary` 不着色——带底 + 结论已是双编码;null → 占位色「-」)。
- **行 1 之下(可用率子行,右对齐于大数字下方):** 「24h 可用率」label(xs)+ meta([stale chip「数据非最新」] + 「更新于 HH:mm · 每 10s 自动刷新」,xs/placeholder,溢出优先截断 cadence 段);子行 = label + meta,在 null/非 null 下构图稳定、内容不变。可用率 null 时注记「24h 内无探测数据」**不下放子行**,与占位「-」同处行 1 内联于 availability-line(2026-07-30 main 裁决维持行 1 内联;GH #81 check LOW-1 口径差登记)。
- **行 2(全宽):** 异常端点 chips 横排(集合/排序/上限/形态/点击口径见下「沿用口径」,不变)。
- **行 3(全宽):** 计数行。
- **skeleton 锚定重算(2026-07-30 实机迭代批,硬要求):** 原 114px 锚定(结论 42 + chips 28 + counts 28 + 两个 8px 间距,check GH #73 LOW-1 登记)随构图修订失效——首载 skeleton 定高必须按新构图 chips-present 布局(行 1 + 可用率子行 + chips + counts)从最终 CSS 算术重算,重算值写入代码注释并与本节登记一致;锚定哲学不变(锚定偏向异常态,healthy 首载无 chips 行短一截为已登记取舍——一条定高匹配不了四态,加载时状态未知)。
**display 锚点:** 带内 display 档仅两处(大字结论、可用率大数字),Display Anchor Rule 重申,本页不再新增 display 消费。
**计数行(stats strip 迁入,行为全保留,GH #55 双控纪律不动):** 总数 + 四状态计数(SEVERITY_ORDER 重→轻)+ 已停用;**计数项 dotless(2026-07-30 实机迭代批,封闭清单场景②):** 去点,状态词语义色着色词 + 数字 `--hs-text-primary` 不变,着色词自身承担双编码;状态项点击过滤/再点取消;选中态 = 1px brand inset 环 + 透明底 + brand 文字;全部真 `<button>`(键盘 Enter/Space + focus ring);statusFilter 与筛选行状态下拉同一 ref 双向同步。
**沿用口径(GH #53 不变):** 异常 chips 集合 = enabled entries 中 failing+down+degraded;排序走 SEVERITY_RANK(utils/severitySort.ts 唯一秩表,tie model_id→protocol→endpoint_id);上限 MAX_ABNORMAL_CHIPS=5,溢出「+N」无框纯文字不可点;chip 形态 = bg-card 表面底无描边 + radius-sm + hover shadow-md,状态词语义色/600 + 模型名 primary 截断,禁圆点禁闪烁禁状态底色;chip 点击 = inspect(状态过滤 + 滚动 matrix,@click.stop);整带点击仅 abnormal 态(过滤到最紧急状态 failing>down)。可用率口径 scopedAvailability(enabledEntries) 探测加权,与后端聚合构造性相等。四态与 skeleton(定高不跳)、空态语义不变。数据只反映全局,永不受页面过滤器影响;其他页面不得复刻其结论文案模式。
**组件实体:** HealthBanner.vue 原地演进(不重命名,减少 diff);新增计数行所需 props(statusCounts / disabledCount / statusFilter)与 toggle 事件,DashboardView 删除 stats-strip 块,strip 点击过滤逻辑随迁; emits inspect 口径不变。

### EndpointCard(端点卡,GH #54 层级重构;2026-07-30 GH #72 信号墙化修订)
矩阵卡片,自上而下:头行(模型名 + **信号墙灯**——2026-07-30 实机迭代批:协议 tag 移状态行,头行只剩名与灯)+ 状态行(StatusBadge **md 档 dotless** + 协议 tag + 评分徽章 + 已停用 tag + 右侧「最近探测」)+ 三指标 + 24h 分段条 + LatencySparkline(EndpointUptimePanel)。**整卡可点开启 EndpointQuickViewDialog 速览弹窗**(2026-07-29 修订,取代直接下钻);深链下钻由弹窗内「打开完整详情」主按钮承担,/endpoints/:id 深链页不动。
- **左边 3px 状态条退役(2026-07-30,GH #72,shape 定稿):** `.card-*` border-left 全删,卡片回干净 1px 描边卡;hover `--hs-shadow-md` 与 1px brand inset 焦点环不变。状态信号收敛为「头行灯 + StatusBadge 词着色」双通道。
- **信号墙灯(2026-07-30,GH #72;实机迭代批位移):** 头行右端、模型名之后一枚 9px 状态色圆点(2026-07-30 实机迭代批:协议 tag 移状态行后,头行 = 模型名 + 灯,灯是全卡唯一状态灯——每卡一灯,状态行 Badge dotless,语义见 StatusBadge 节 dotless 变体与 DESIGN.md「信号墙灯与词分层」),**仅 status ≠ healthy 渲染**(degraded=warning / down=danger / failing=failing 橙 + `animation: var(--hs-blink)`,reduced-motion 由令牌归零既有机制覆盖)——健康通道灭灯,异常才亮灯。`aria-hidden="true"`(状态词由 StatusBadge 承担,a11y 树不重复报状态);卡片级标记,非第二 StatusBadge;组内协议 tag 收敛(GH #54)时灯不随 tag 收敛、仍在头行右端。
- **矮化(2026-07-30,GH #72):** 状态行右侧并入「最近探测 HH:mm」(xs/secondary,`margin-left:auto`),页脚行取消;卡片各行间距统一收紧为 `--hs-space-2`(8px,原 10/12px);内边距 16px 消费档不动。三指标与 EndpointUptimePanel 行高不动(仅外边距随 8px 节奏)。
- **模型名中间截断(splitMiddle,utils/truncate.ts 纯函数):** 双 span——head 吃 CSS ellipsis(min-width:0),tail(flex:none,默认保 12 字符)永不截断,截断由宽度驱动、禁 JS 字符预算;短名(len ≤ tailKeep+1)不拆,tail 为空。**全显走 el-tooltip 快显(2026-07-31 GH #86,取代原生 title):** show-after 200ms,content = 完整 model_id,样式与全站 tooltip 统一(原生 title 约 1s 系统延迟、样式不可控;默认常滚与 hover 跑马灯均否决——撞「failing 独占全站唯一动画」承重语义,跑马灯亦慢于即时全显,裁决见 ui-guidelines §6 长文本条);恒挂不测量截断态——短名 tooltip 内容与可见文本一致,不影响展示;不干扰整卡点击开速览与 focus-visible ring(tooltip 不拦事件、触发元素不进 tab 序)。后缀通常是最具区分度的版本/变体段,尾截断砍后缀已废。
- **状态行升格:** StatusBadge 用 md 档(size prop 'sm'|'md' 默认 sm,md = 词 md/600 + 圆点 11px;仅 EndpointCard 消费 md,其余消费方默认 sm;禁 :deep 覆写,不构成第二状态灯);**此处 Badge dotless(2026-07-30 实机迭代批,封闭清单场景①)**——头行灯是全卡唯一状态灯,状态行仅着色词;成因副标签恒 sm/secondary 不随档升、不随词着色。
- **指标主从:** P50 主(lg/600 primary)、P95 次(sm/secondary)、24h 成功率 md 不变;顺序不变(成功率 / P50 / P95)。
- **评分徽章:** 有分恒显「评分 N」前缀(稳定性评分,非 formatScore 管辖);「暂无评分」不变。绿档徽章文字随 GH #71 语义迁移消费 `--hs-success-text`(文字场景 success 深阶,ui-guidelines §3)。
- **协议 tag 组内同值收敛(GH #34 映射不动;2026-07-30 实机迭代批位置随迁):** 协议 tag 现位于**状态行**(Badge 之后;2026-07-30 实机迭代批从头行迁入,用户实机反馈「模型名截断」——头行让给模型名与灯,模型名中间截断 tailKeep=12 口径不动,tag 移走后头部空间已够);映射与词表不动(ui-guidelines §5 协议 tag 条)。OverviewGroupSection 算 uniformProtocol(筛选后 entries 全部 protocol 相等且非空),同值时组头身份区(group-count 之后)渲染一枚 el-tag(protocolTagType, size small),卡片收 `showProtocolTag=false`(prop 默认 true);**三例外/边界:** grouping='protocol' 时组名即协议——组头不渲染 tag、卡片 tag 同样收敛;flat 模式不收敛(卡片保留 tag);混搭组/空组不收敛。EndpointTable / EndpointDetailView / StatusCard 静态物料(明细行「模型 · 协议」)不动。
- **卡片网格(2026-07-30,GH #72):** `minmax(272px, 1fr)`(原 300px;1200px 内容宽稳定 4 列:4×272 + 3×12 = 1124 ≤ 1200,5 列需 1408 超出),OverviewGroupSection 与 DashboardView flat 两处 .card-grid 同步;sparkline 经 ResizeObserver 重算,卡宽变化不破 x 轴构造性对齐(.dots-strip 2px gap 与 .dots-label 26px 宽仍是共享常量,不动)。

### OverviewGroupSection(分组 section)
折叠箭头用 EP `ArrowDown/ArrowRight` 图标(2026-07-29 用户反馈,取代文本三角形 ▾/▸);**筛选后空组自动折叠**(2026-07-29 用户裁决:空组不再渲染大空态盒,匹配恢复即自动展开,两态均可手动折叠)。
**折叠披露过渡(2026-07-29 设计评审,克制版动效,/impeccable animate):** 卡片矩阵容器走 ui-guidelines §6 披露容器三件套(grid `0fr→1fr` + 内层 `min-height:0/overflow:hidden` + `visibility` 延迟切换:折叠向 `0s 0.2s`、展开向 `0s 0s`),时长 `--hs-transition`;visibility 保证折叠态退出 tab 序与 a11y 树(与 v-show 等价,a11y harden 成果不回退),不支持 grid 轨道动画的浏览器退化为瞬切。折叠箭头改单图标(ArrowRight)`transform: rotate(0↔90deg)` 过渡,取代 ArrowRight/ArrowDown 双图标瞬切;终态语义不变。**仅用户点击触发过渡;watch(entries.length) 的筛选空组自动折叠/恢复展开走 no-motion 瞬切**(延伸 GH #52 数据驱动不动画纪律,防筛选连续击键时多组同时高度补间噪音)。
**组头节奏(2026-07-30,GH #74,shape 定稿):** 组名字号 `--hs-text-lg` → `--hs-text-xl`(20px/600,Title 档——分组标题是状态板次层级锚点);端点计数、状态计数 chips、「本组:」聚合指标、协议收敛 tag、分享按钮规格不动,随新字号基线对齐。**组头下 1px `--hs-border-light` hairline 分隔**(装饰性分隔用 border-light,不用 border):组头行 `padding-bottom: var(--hs-space-2)`(8px)+ hairline + `margin-bottom: var(--hs-space-3)`(12px)到卡片矩阵。**组上下呼吸:** section 间距 `--hs-space-3`(12px)→ `--hs-space-6`(32px)。折叠披露三件套、no-motion 双轨、空组自动折叠、组头分享入口、协议收敛全部不动。
**折叠组头修订(2026-07-30 实机迭代批,用户实机反馈「折叠线」;GH #83):**
- **hairline 仅展开态显示:** 折叠态为干净单行,hairline 隐藏;**几何稳定硬要求**——折叠态以 1px 透明边占位(border-bottom 保留、颜色 transparent 或等价机制),折叠/展开两态组头行高度逐像素一致,禁高度跳变(与 ui-guidelines §6 披露容器「禁布局跳动」同纪律)。
- **组头行垂直居中修正:** padding 上下对称(重心回中;原仅 padding-bottom 8px 的不对称内边距使行内容偏上)。
- **状态计数 chips dotless(封闭清单场景③):** 去点,状态词语义色着色词 + 数字不变,着色词自身承担双编码(语义见 StatusBadge 节 dotless 变体)。
**细带化 + 指标下移同行(2026-07-31 /impeccable 实机迭代,用户裁决;GH #85):**
- **构成变更:** 组头行不再含「本组:」聚合指标(组头行 = 折叠箭头 + 组名 + 端点计数 + 状态计数 chips(+ 协议收敛 tag)+ 分享按钮);**第二行 = UptimeStrip 细带 +「本组:24h 可用率 X · 均延 Y」指标右对齐同行**——形态 → 读数一次扫完。(2026-07-31 GH #87:条的 flex-1 全宽登记由定宽 360px 取代,指标右对齐机制随迁,见下条。)
- **细带化:** 条格高 10px → 6px(用户实机反馈「格子太大」;组级条是全组扫读带,EndpointCard 卡内条 10px 端点粒度证据不动,两处格高自此分档);24 格 flex 槽、2px 间距、`--hs-radius-xs`、§3 批 59 三档着色 + 无数据灰、逐格 tooltip 口径全部不动。
- **否决登记:** 「条收进组头行」方案用户否决——组头左侧内容(组名长度、chips 个数、计数数字)逐组不同且随轮询变化,条内联后跨组位置参差、左侧长内容有折行风险;跨组对齐是时间轴语言的核心价值,不能牺牲。
- **对齐约束:** 条左缘全组严格对齐;右缘随指标文字长度几 px 参差(格宽差异 <2%,不可察),登记为已知并接受,禁为右缘对齐把指标文字定宽。(2026-07-31 GH #87:本条参差登记随定宽 360px 自然失效撤销,见下条。)
- **折叠恒显不破(GH #64/spec 0017):** 折叠态组条行照常渲染;折叠披露三件套与 no-motion 双轨不受影响。
**条收敛定宽 360px(2026-07-31 /impeccable 实机迭代,用户裁选定稿;GH #87,取代 GH #85 的 flex-1 全宽登记):**
- **定宽:** 组条从 flex-1 全宽改为**左对齐定宽 360px**——格约 13×6px,「灯」感消失、读作微缩时间轴(实机证据:GH #85 细带化方向对,但全宽 24 格平分下内容宽 ≈1150px 时每格 ≈76px,一串长胶囊远看仍是一排大灯;格宽 = 条宽 ÷ 24,只压高度治不了「灯太大」,必须收敛条总宽)。
- **指标右对齐机制变更:** 条不再 flex:1 后,指标右对齐改由指标自身 `margin-left: auto` 承担;条与指标之间自然留白(工具风克制,行高约 18px 不变)。
- **对齐约束修订:** 条宽定值 → 跨组条完全同宽、左缘严格对齐,右缘亦因定宽天然对齐——GH #85「右缘随指标文字长度几 px 参差」的登记随定宽自然失效撤销。
- **窄视口(§4 不破):** 条 `flex: 0 1 360px` + `min-width: 0` 可收缩(槽内 24 格 flex 1 1 0 同步缩),指标 nowrap 不收缩,叠加不撑出横向滚动。
- 其余规格不动:24 格、2px 间距、`--hs-radius-xs`、§3 批 59 三档着色 + 无数据灰、逐格 tooltip、折叠恒显。
标题行(2026-07-31 GH #85 修订,组聚合指标移出):折叠箭头 + 组名 + 端点计数 + 状态计数 chips + 分享入口。整行可点折叠。**分组独立分享入口**(批 59):标题行右端 text 型按钮(Share 图标 + 「分享」文字),@click.stop 不触发折叠;复用 StatusShareDialog,快照范围 = 该分组条目 ∩ 当前页面筛选,scope chips 首位恒为分组 chip(label「分组」,值「厂商/能力/协议 · 组名」);**卡片所有数字一律从快照 entries(enabled)计算,与范围 chips 恒一致**——24h 可用率 = 快照 entries dots_24h 按小时求和 ok/total;平均延迟 = enabled entries p50_ms 均值(唯一 scope 恒一致口径;与组条行右侧「本组:均延」探测加权值可能略异,卡片内部自洽优先)。

### EndpointQuickViewDialog(端点速览弹窗,2026-07-29 设计评审;2026-07-30 安静入场 + 明细曲线视觉修订,morph 编舞同日退役)
Dashboard 卡片点击开启的轻量速览(el-dialog,640px + max-width 92vw,**align-center 垂直居中**(2026-07-30 修订,取代 EP 默认 15vh 顶距——终态在屏幕正中,与安静入场「中心淡入」语义一致;EP 2.14 per-instance prop,仅本弹窗,其他弹窗定位语义不变),radius-lg,shadow-lg 浮层语义,消费页密度)。
- **安静入场(2026-07-30 用户实机裁决,FLIP morph 编舞整体退役——用户三连否「不做翻转放大吧」「(明细曲线)这个太丑了」「动画也很丑」;方向定稿:工具风「反馈在,表演不在」):** 弹窗入场 = **0.2s default 档 `opacity + scale(0.96→1)` 中心淡入**——无位移、无翻转、无飞行;纯 CSS transition,reduced-motion 由全局归零覆盖(无 JS 门控分支)。卡片回到**静态可点**:hover `--hs-shadow-md` + focus ring 保留,零 transform 编舞。**退役清单(同批移除,登记防复活):** 卡片飞行 inline transform 终态串(translate→scale→rotateY)、`.is-flipped` 与 `.el-card__body` 内容淡出、`.card-grid` perspective 1600px、`.collapse-inner` 的 `has-flipped` 裁剪豁免类、`cardFlightTransform` 等编舞纯函数与 quickViewChoreo.ts 全部编舞常量(QUICKVIEW_* 系列)、ep-theme 的 hs-flip 弹窗 transform 过渡、`--hs-transition-focal` 令牌(零消费方)。**沿用品:** 雾化 blur 8px 与滚动条三件套保留(用户从未否定);align-center 保留;async 区定高保留(align-center 下异步加载不引发双向重心跳动)。
- **雾化浮层:** modal-class 走 ep-theme.css 全局块,--hs-overlay-bg 衬底 + backdrop-filter blur 8px(专属例外,管理台弹窗不雾化)。
- **内容(零等待):** 首帧全部由打开时冻结的 entry 快照渲染——StatusBadge md + causes、协议 tag、模型名、三指标、EndpointUptimePanel;「最近失败」区异步拉 listProbeHistory top 5(ProbeRecordTable `slim` 变体:隐藏 类型/HTTP/TTFT/输入token/输出token 五列,保 结果/错误摘要/延迟/时间,列宽合计 ≈550px 算术贴合 600px 内容宽——2026-07-29 main 裁决,评审就绪文本「:compact="false"」全列 ≈1010px 与 640px 弹窗自相矛盾,§4 禁横向滚动为承重纪律;empty-text 走 prop,本区传「暂无失败记录」——该区只查 ok=false,共享固定文案「暂无探测记录」语义不准),skeleton/错误重试三态,失败只影响该区。底部「打开完整详情」主按钮 router.push /endpoints/:id。
- **「24h 延迟明细」区(2026-07-30 新增,用户实机反馈「Modal 里也要显示更细节的曲线」;同日视觉三修——用户实机否「这个太丑了」「(区域)都很丑」;语义层约束见 ui-guidelines §5 本组件条目):** 位于 EndpointUptimePanel(小时级,保留——「也要」是加不是换)下方、最近失败区上方。组件 = **新迷你裸图表组件**(ECharts 既有栈 + useChartColors 镜像,TrendChart「裸图表布局由父级负责」先例;TimeSeriesChart 是 el-card 封装 + category 轴契约,不适配,不复用);数据变换(records → 线系列 + 故障窗 markArea,接口倒序 → 绘图正序;中位间隔计算、断线切分、故障窗合并全部走纯函数)抽纯函数,vitest 覆盖 空/全失败/单点/倒序输入 + 断线窗口 + 故障窗合并边界。**视觉三修(2026-07-30 定稿):** ① 延迟线 `showSymbol: false`——删逐点圆圈,纯线 1.5px 中性色(与 LatencySparkline 中性曲线同语言);② **断线纪律:** 连续成功探测时间间隔 > 3×中位间隔处断线——禁直线横跨数据空洞(空洞 = 无证据区间,连直线即伪造形态);③ **失败表达 = 故障窗浅色带(markArea),逐点 rug 三角散点退役**——相邻失败间隔 ≤ 2×中位间隔合并为一个窗口,markArea 整高 danger 浅底(低透明度);稀疏失败 = 细带、密集失败 = 色带,严重度由带宽度承担;窗口 tooltip「HH:mm–HH:mm · 失败 N 次」;失败探测不再有任何散点。**实现层二修(2026-07-30 救援批第二轮登记):** ④ **单点故障窗最小可见宽**——start===end 零宽窗口的 markArea **渲染边界**(bandStart/bandEnd)向两侧各扩 0.5×中位间隔,真实窗口边界 start/end 不动:扩宽仅作用于渲染,窗口失败计数与 tooltip 起止时刻保真,多失败窗口不扩;⑤ **面积填充**——延迟线下方 `--hs-bg-hover` 实心填充(LatencySparkline 先例:功能性强调非装饰,禁渐变;connectNulls:false 下填充随断线分段;色值走 chartColors 镜像 `bgHover` 字段,亮暗双值与 semantics.css 逐一同步(值随 2026-07-30 GH #70 令牌精修,以 DESIGN.md 同族精修登记为准))。图表高 180px,宽随弹窗内容宽;**原 ScatterChart 注册需求随散点退役取消**(markArea 所需组件按需登记进 utils/echarts.ts,注册表注释同步,模块化体积纪律不破)。三态与最近失败区同款(skeleton/错误重试/数据,失败只影响本区),打开时一次性拉取、随打开冻结(快照纪律延伸)。**数据路径(契约变更,2026-07-30 评审登记):** `GET /api/endpoints/{id}/probes` 增可选 `hours` 开窗参数(窗口查询行帽 2000;默认探测周期 300s 下 24h ≈ 288 轮 ×chat 双记录 = 576 条,载荷 ≈150KB;现状 limit 钳 [1,200] 拿不到 24h 全量,这是本次必须动后端的原因),api-contract.md 同步 + W1 黑盒测试(hours 边界/行帽截断/ok+hours 组合/倒序不变);既有 limit/ok 语义与三处存量调用方(listProbeHistory × EndpointDetailView/EndpointTable/本弹窗最近失败区)零改动。高频探测端点(周期 <300s)超行帽时按最新 2000 条截断,覆盖区间由时间轴自明,不做静默「24h」宣称。
- **快照冻结:** 弹窗内容不跟随 overview 轮询更新(与 StatusCard 快照同哲学);延迟明细区同为打开一次性拉取;实时数据走完整详情。
- **排除项:** 不放分享入口(防弹窗叠弹窗)。
- **关闭:** ESC/浮层/按钮/路由跳转统一走 EP @closed 单点复位路径;关闭后焦点归还触发卡片(EP focus-trap + 手动 focus 双保险)。

### AppHeader(公开侧形态)
导航按登录态过滤:未登录只渲染公开页项(状态总览 + 评估榜单→/board);登录态 = 状态总览 + 评估榜单(→/eval)+ 任务中心,随路由切换重检。**未登录 header 一律不渲染登录按钮**(ticket 90 裁决:醒目登录按钮传递「内容要账号」错误信号;判定走 route meta.public),登录入口统一由 PublicFooter 承担。右栏:亮/暗切换(未登录可用,localStorage `hs:dark`,默认亮不跟随系统)+ 登录态时批次进度入口(仅存在未完成批次时渲染,3s 轮询 settle 即停,「批次运行中 X/Y」点击跳 /eval;禁用橙与闪烁)+ 角色 tag(集中映射 utils/role.ts,primary=管理权/info=非管理,语义=权限层级非健康度)。

### PublicFooter(公开页管理入口唯一组件)
hairline + 一行左右分置:左 © 版权,右「管理登录」→ /login(xs placeholder,链接 hover brand)。状态总览、EndpointDetail、/board 三页一律复用;/login 页不渲染;登录态照常渲染。豁免:/report/:token 分享页不挂页脚。

## 数据与行为约束
- **防作假:** 任何汇总结论必须标注统计范围;筛选快照不得引用未筛选聚合字段;空态中性,永不读作「全部正常」。
- **轮询:** overview 走 utils/visibilityPoll.ts(10s,标签页隐藏降频 60s,回前台立即刷新),卸载必清理。
- **状态排序口径(GH #55 定稿;2026-07-30 随 GH #73 迁入 hero 带):** hero 带计数行与组头共享同一严重度序,重→轻(failing>down>degraded>healthy);单一来源 = `utils/severitySort.ts` 的 `SEVERITY_ORDER` 数组(与 `SEVERITY_RANK` 秩序一致,vitest 断言守护),strip 原轻→重 `STRIP_ORDER` 与组头 `STATUS_PRIORITY` 两处本地口径已删除。
- **首屏严重度组织(GH #52 登记):** 状态板组间/组内/flat 三处统一走 `utils/severitySort.ts` 的严重度秩(`SEVERITY_RANK`:failing>down>degraded>healthy,全站唯一来源,StatusCard 异常明细同引);**口径一律按筛选后 entries 计算**。组间秩 = 组内 enabled entries 的最小秩,tie 按组键字典序(`<`,不用 localeCompare);组内秩升序,tie 按 model_id → protocol → endpoint_id;flat 模式同经 `sortEntriesBySeverity`。**已停用端点(`DISABLED_RANK`)恒沉底**——disabled 的 down/failing 不抬组秩、不参与首屏竞争;**筛选后空组沉底**(组键字典序,空 hint 现行为保留)。轮询/筛选引起的数据驱动重排不做动画(与榜单行重排同纪律)。同纪律延伸至披露动作:数据/筛选驱动的分组折叠与恢复不做动画(no-motion 双轨,机制见「组件规格」OverviewGroupSection 节)。
- **速览弹窗快照冻结(2026-07-29):** 打开即冻结 entry 快照,轮询不更新弹窗内容;入场为 0.2s 安静入场(用户触发单次过渡),与 GH #52 数据驱动不动画纪律兼容(同折叠披露条款口径)。(原「翻转是用户触发单次过渡」表述随 morph 编舞 2026-07-30 退役改写。)
- **statusFilter 双控(GH #55 登记,有意为之;2026-07-30 随 GH #73 迁入 hero 带):** hero 带计数行状态项点击(再点取消)与筛选行状态下拉绑定同一 `statusFilter` ref,双向同步构造性成立;计数行是快捷路径,hero 带 inspect 写同一 ref;三者共享单一过滤源。选中态 = 1px brand 内嵌描边(box-shadow inset,零布局位移)+ 透明底 + brand 文字(2026-07-29 用户实机反馈定稿,取代 GH #55 的 brand-soft 浅底——浅底色块与状态点语义色打架;brand 描边保留「激活选择」语言且零色块;原 2px 下划线读作导航 tab 的撞车问题保持已解)。
- **「本组」前缀(GH #55;2026-07-31 GH #85 起指标自组头移至组条行):** 组条行右侧本组指标以「本组:」容器级前缀一次统领 24h 可用率与均延两项,与 HealthBanner 的全局口径区分归属,两个可用率不再裸名并列。
- **筛选行下拉内联 label(GH #55,Dashboard 局部约定,不推广):** 协议/状态两个 select 前置内联 label(sm/secondary「协议:」「状态:」,与 select 同行),不再以 placeholder 兼任 label;关键词输入框与分组 select 不动。
- **滚动条抖动修复(2026-07-30,用户实机反馈):** 弹窗锁屏引发的页面/弹窗左移走 scrollbar-gutter 三件套(ep-theme.css:html stable + 中和 EP body 宽度补偿 + .el-overlay-dialog stable both-edges),滚动条存在性不再是布局变量;overlay 滚动条平台零视觉变化;代价 = 经典滚动条平台短页面常驻右侧 gutter 条(工具产品可接受,登记为已知取舍)。

## 可访问性(2026-07-29 harden 批)
- **四处主交互键盘可达(audit P1/P2,WCAG 2.1.1;2026-07-30 随 GH #73 迁移):**
  - **hero 带计数行可点项(总数 + 四状态项,原 stats strip 迁入)** → 真 `<button type="button">`(font/color inherit、background/border none,padding 不变);`:focus-visible` = 1px brand inset ring(与选中态同语言)。点击过滤/再点取消行为不变。
  - **组头折叠(OverviewGroupSection)** → 全宽 `<button type="button">`(text-align:left)+ `aria-expanded`;**分享按钮移出为兄弟节点**——`<button>` 内不得嵌套 `<button>`(原结构里 group-share 嵌在组头内,button 化必须拆出,视觉位置不变)。`:focus-visible` 同上 ring。
  - **Hero 带整带(原 HealthBanner 整卡)** → 仅 abnormal 可点态根节点挂 `role="button" tabindex="0"` + Enter/Space 触发同一 inspect;非可点态无 role 不进 tab 序。`:focus-visible` = brand inset ring + shadow-md(与 chips 同语言)。chips 已是真 button。
  - **EndpointCard 整卡开速览** → el-card 根 `role="button" tabindex="0"` + `aria-haspopup="dialog"` + Enter/Space 触发开启(2026-07-29 修订:同页开弹窗是 button 语义,取代 link);`:focus-visible` = 1px brand inset ring(box-shadow;2026-07-30 GH #72 左边条退役后 ring 独占焦点表达,零布局位移)。卡内无嵌套交互(仅 tooltip);弹窗关闭后焦点归还本卡(手动 focus 兜底,防轮询重渲致 EP 焦点归还落空)。
- **焦点语言全板统一:** 1px brand inset ring(`box-shadow: inset 0 0 0 1px var(--hs-brand)` + `outline: none`),不引入第二种焦点样式;ring 只在焦点态出现,不改变任何静态视觉。
- **语义 h1:** DashboardView 挂视觉隐藏 h1「HubScope 服务状态总览」(标准 sr-only 模式:absolute 1px + clip,禁 display:none——那会把它从 a11y 树里删掉),零视觉变化。
- **reduced-motion 替代(WCAG 2.3.3,前庭敏感):** failing 闪烁动画令牌化为 `--hs-blink`(semantics.css 唯一定义;消费方封闭清单(2026-07-30 实机迭代批更新):**卡片头灯 + Hero 带 alert-dot + 带点 Badge 的 failing 点(详情页/速览弹窗/管理台等一切带点消费方)**——Dashboard 三处 dotless(卡片状态行/计数行/组头 chips)后板面闪烁只剩头灯与 alert-dot 两处,全站不新增第四处);`@media (prefers-reduced-motion: reduce)` 下 `--hs-blink: none`——**闪烁在 reduced-motion 下静止,状态由实心橙 + 状态词双编码承载(动画是增强,从不是唯一通道)**。**dotless 无 a11y 回退(同批登记):** 状态词本来就是 Badge 的可访问名,圆点是无词装饰(头灯 aria-hidden),dotless 与带点 a11y 树等价,无需 aria 补偿。HealthBanner skeleton 的 pulse 动画沿用组件内既有 reduced-motion 豁免,保持不动。**全局过渡归零(2026-07-29 /impeccable animate 批,main 裁决):** semantics.css 同一 media 块内追加全局 `transition: none !important`——折叠高度/箭头旋转等一切用户触发过渡在 reduced-motion 下瞬时完成,一处覆盖新折叠 + LoginView 验证码区存量(首例原无 reduced-motion 处理,本批连带修补,登记不留暗账)+ 未来同类;blink 由令牌归零、pulse 由组件豁免,均不受该规则影响。`onBannerInspect` 的 `scrollIntoView` 对 reduced-motion 降级为 `behavior: 'auto'`(修补 a11y harden 批漏网)。

## 体检基线与已排改进
- critique 基线 22/36(2026-07-29,快照 .impeccable/critique/):严重度不驱动首屏、banner/strip 信息重复、卡片墙均质化(P1);排序口径两套、下拉 placeholder 当 label(P2)。
- 已排票:#52 severitySort(**已完成 2026-07-29**,约定见「数据与行为约束」)/ #53 HealthBanner 重构(**已完成 2026-07-29**;2026-07-30 随 GH #73 演进为 Hero 指挥台带,约定见「组件规格」Hero 带节)/ #54 卡片层级重构(**已完成 2026-07-29**,约定见「组件规格」EndpointCard 节)/ #55 一致性批(**已完成 2026-07-29**,约定见「数据与行为约束」状态排序口径/双控/「本组」前缀/下拉 label 各条)。
- **GH #69–#74 重构批(2026-07-30 /impeccable shape 定稿,plan 回写完成):** #70 T1 色调同族精修(令牌值见 DESIGN.md 同族精修登记)/ #71 T2 StatusBadge 词随灯着色 + 点微调(见 StatusBadge 节)/ #72 T3 EndpointCard 信号墙化(左条退役/异常灯/矮化/4 列 272px,见 EndpointCard 节)/ #73 T4 Hero 指挥台带(见 Hero 带节)/ #74 T5 分组节奏(组头 xl + hairline + space-6 呼吸,见 OverviewGroupSection 节)。依赖序 T1→T2→T3、T1→T4、T1→T5。
- **实机迭代批(2026-07-30,批次 GH #80,用户实机反馈逐项裁决):** GH #69–#74 发布测试线后的四项定稿——① Hero 带构图(GH #81):可用率大数字上提行 1 与结论同基线,label/meta 在其下,chips/计数行全宽居下(见 Hero 带节;实机证据「可用率沉底」);② 每卡一灯 + Badge dotless 三处封闭清单(GH #81 计数行 / GH #82 卡片状态行;见 StatusBadge 节与 DESIGN.md「信号墙灯与词分层」;实机证据「灯看花眼」);③ 协议 tag 移状态行,头行 = 模型名 + 灯(GH #82;见 EndpointCard 节;实机证据「模型名截断」);④ 折叠组头:hairline 仅展开态 + 1px 透明边占位 + 组头行垂直居中(GH #83;见 OverviewGroupSection 节;实机证据「折叠线」)。skeleton 锚定随①重算(硬要求,见 Hero 带节)。

## 未决(另立批次)
- dots aria 等价(24h 分段条的屏幕阅读器等价信息)、URL 深链(筛选进 query)、非均质矩阵方向(异常卡大/健康卡小,Provocative Q3 未裁决)。a11y harden 批(键盘可达/h1/reduced-motion)已完成 2026-07-29,约定见「可访问性」节。

## 维护登记
- **新组件入 related_targets(check 2026-07-30 沉淀建议②,防再漏机制):** 新组件接入本页消费链时,必须同步登记 frontmatter `related_targets`——frontmatter 是 GH #50 体系的索引面,漏登记会让后续检索失明。先例:2026-07-30 补登 EndpointQuickViewDialog / EndpointUptimePanel / ProbeRecordTable(LOW-1 修补)。该纪律对本页与后续新建/接入组件的 surface brief 同样适用。
