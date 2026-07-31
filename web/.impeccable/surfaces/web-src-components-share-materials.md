---
version: 1
slug: "web-src-components-share-materials"
primary_target: "web/src/components/StatusCard.vue"
related_targets: ["web/src/components/StatusCardDetail.vue","web/src/components/StatusCardMetrics.vue","web/src/components/StatusCardSingleModelMetrics.vue","web/src/components/EvalCard.vue","web/src/components/StatusShareDialog.vue","web/src/components/EvalShareDialog.vue","web/src/components/Leaderboard.vue","web/src/components/EvalProgressGrid.vue","web/src/views/CampaignReportView.vue","web/src/utils/statusCardSummary.ts","web/src/utils/statusCardSnapshot.ts","web/src/utils/evalCardSnapshot.ts","web/src/utils/cardImage.ts"]
---

# 分享物料与分享弹窗 — 表面简报

> 组件族 brief:StatusCard 族 / EvalCard / 两个分享弹窗是跨视图消费的品牌物料组件族,「一张物料一处规格」。业务语义(防作假同源口径、三态词表、静态物料双编码、*-text 分工)以 ui-guidelines §3/§5 为准,本 brief 不重复。
> **v2 更新(2026-08-01,GH #122):** 物料新视觉重建(GH #121)与三态措辞(GH #113)落地后的定稿规格;旧世界(teal 品牌区、四态、display 28 档)规格作废。

## 范围与读者

- 读者:外部接收者(隔着一条分享链接/一张转发图,PRODUCT.md 第三受众)与转发者(状态板/管理台读者)。
- 物料纪律(ui-guidelines §5/§2):恒亮主题导出、2x 导出、离屏捕获 `position: absolute; left: -10000px`(**禁 fixed**——snapdom 视口重排)、数字与范围 chips 同源、空态永不读作「全部正常/全部稳定」。
- **盒模型事实登记:** 本仓无全局 `box-sizing` reset,物料卡根元素均为 content-box——`width: 720px` 是**内容盒**宽,外盒 = 722(1px×2 描边在盒外);「720 画布内容宽 640」即由此而来(720 − 40×2 padding)。480 族同(内容盒 480,body padding 20×2 → 内容 440)。弹窗预览缩放一律按**外盒**(722 / 482)计算。

## 版式体系(GH #93,沿置)

两档版式,同一份冻结快照的第二排版,数字口径零变化:

| 版式 | 设计宽 | 适用 | 构成 |
|---|---|---|---|
| 完整版 | 720px(2x 导出 1440) | 全局 / 分组 / 单模型 | 下「完整版构成」 |
| 紧凑版 | 480px(2x 导出 960) | 全局 / 分组 | 同构降宽,见「480 窄版构成」 |
| 紧凑版(单模型)= 端点小卡 | 480px(2x 导出 960) | 单模型 | 独立紧凑构成,见「端点小卡构成」 |

**五形态 = StatusCard 三模式(全局/分组/单模型)× 两版式(完整版/紧凑版,单模型紧凑版即端点小卡)+ EvalCard。** EvalCard 无紧凑版(榜单物料手机可读性由弹窗预览缩放承担),登记为后续可选。

- **版式切换:** StatusShareDialog 预览上方「完整版 / 紧凑版」el-radio-group(size small);Dashboard 全局分享与 EndpointDetailView 单模型分享两入口同生效(v2 起组分享入口随旧组头退役,分组 chip 由筛选条件承担)。切换只换预览与离屏双份的版式 prop,快照同一份。
- **默认档:** 打开弹窗时视口宽 < 768px → 紧凑版,≥768px → 完整版;打开时一次性 matchMedia 判定,不随 resize 重判、不持久化。
- **文件名:** 紧凑版加 `-compact` 段:`hubscope-status[-scope][-compact]-YYYYMMDD-HHmm.png`。

## v2 视觉重建(GH #121,2026-08-01)

外框结构不变(品牌区 / 范围行 chips / 页脚三段,ui-guidelines §5 外框约定),字排与着色按新世界重排:

- **品牌区:** 4px `--hs-brand` 品牌条(蓝)+ `--hs-brand-soft` 浅底 + BrandMark(**蓝渐变** blue-400→blue-700)+ Wordmark + 物料标题——标题取 **3xl 页面标题档**(「服务状态」/「评估榜单」,取代旧 2xl 24);端点小卡品牌细条行(BrandMark 16 + Wordmark sm,无浅底无标题词)沿置。
- **hero 大数字:** 完整版可用率大数字取 **hero 72 档**(`--hs-text-hero`,600,tracking -0.02em,tabular-nums——与状态概览 StatusHero 同字排;旧 display 28 档在物料同步退役);**480 族锚点降 3xl**(480 hero panel 左列容不下 72px——仍是全卡最大数字);次级「%」完整版 xl / 紧凑版 md。
- **卡内分隔全部 hairline:** hero panel 竖分隔、明细区行分隔、页脚分隔一律 `--hs-border-light`(hairline 层级,不用 loud border;GH #121 line-lightening,与 GH #118 页面同口径)。
- **文字场景一律 *-text 阶:** 状态词、分布串非零段、可用率三档着色数字、failed 警示行(warning-text)、chips 状态值、涨跌箭头;图形(分段格、alert-dot、条形)仍本体阶(ui-guidelines §3 文字/图形分工)。
- **分布串三段(GH #113):** 「稳定 N · 性能下降 N · 服务异常 N」恒列,零计数段整段 placeholder;failing 由「含 N 个告警」事件 chip 披露(danger-text 描边 chip + danger 实心点——显示层无第四色,ui-guidelines §3.2)。
- **小卡 av-* 类补登(GH #121,GH #93 潜伏 bug 修复):** 端点小卡指标行大数字的 `av-ok / av-partial / av-fail / av-none` 类在 GH #93 落地时从未定义(数字静默落默认墨色),GH #121 补登于 StatusCard.vue(*-text 阶三档 + none placeholder);`av-*` 着色类自此是物料族登记类名,StatusCardDetail 名单区同源消费。
- **compact override 落点(GH #121 check HIGH-1):** 紧凑版对子组件 hero panel 的覆写全部在 **StatusCard.vue 以 `.compact :deep(.hero-panel / .metric-divider / .hero-big / .metric-unit)`** 书写——scoped 作用域 ID 只父→子,子组件内 `:deep(.compact)` 是构造性死选择器(ui-guidelines §4 纪律;GH #93 时代两条死规则同批清除)。
- **EvalCard 同构重建:** 轻容器语法(白面 + 1px 描边 + radius-lg + 无阴影)、hairline 行列分隔、档色数字 *-text 阶、模型名 md/600 墨色、前 3 名仪式(3px brand 竖条 + 名次大字)重译蓝品牌;failed 警示行 warning-text;范围 chips / 判分不完整水印 / 涨跌基准口径全不变。

## 完整版构成(720,GH #121 现状)

自上而下八区(沿置,字排按上节):① 品牌区(padding 16px 40px;BrandMark 32 / Wordmark xl 20 / 标题 3xl 32);② 范围行 chips(无筛选纯文本「全部端点」;有筛选逐项 chips,状态 chip 值 *-text 阶着色);③ hero panel(`--hs-bg-page` 中性浅底 + radius-lg + padding 16px 20px,左右两列 + 1px hairline 竖分隔:左列可用率 hero 72 大数字 + verdict + failing chip + 三段分布串,右列平均延迟 xl/600 墨色);④ 24h 分段可用率条(24 格填满式、格高 16px、radius-xs、2px 间距,三档着色 + 无数据灰,条下轴标「24 小时前 / 现在」);⑤ 异常明细(封顶 10 条,严重度排序,三段式行:状态词 *-text/600 + 模型·协议 + 单端点可用率;status_reason 两行截断;单端点 24h 打点条格高 8px 无轴标;overflow 收尾「另有 N 个异常端点未列出,详见状态板」);⑥ 正常端点名单区(GH #92 沿置,见下节);⑦ 一句话总结(`summaryText`/`singleModelSummaryText` 纯函数,优先级命中即止;不得掩盖异常);⑧ 页脚(hairline,左「生成于 YYYY-MM-DD HH:mm」+「另有 N 个已停用」,右 location.origin,xs placeholder)。空态:chips 保留、hero panel 中性灰底 + 可用率 `-`、分布串与 verdict 不渲染、不渲染总结与名单区。

**单模型模式(完整版):** 判定 `entries.length === 1 && hubName 非空`;范围区三枚 chips(模型/协议/Hub);hero panel 走 StatusCardSingleModelMetrics(可用率 hero 大数字 + 单状态陈述行 `singleModelStatement` + failing chip|评估区:总分 + suite tags 封顶 6);明细区单条三段式照常;名单区不渲染;全正常陈述「当前状态正常」。

## 正常端点名单区(GH #92,沿置)

- 位置 = 异常明细区之下、总结行之上;区头「正常端点」同 detail-title 规格(sm/600 secondary)。
- 2 列网格(720 列宽 312 / 480 列宽 212),条目 = 模型名(sm 截断)+ 24h 可用率(sm/600,`av-*` 三档 *-text 阶,null → `-` av-none);不带协议、不带点条。
- 排序 = `success_rate_24h` 升序(最脆弱在前),null 沉底,同率 model_id 字典序,纯函数集中 `utils/statusCardSummary.ts`(vitest 覆盖);封顶 20 条,overflow 收尾「另有 N 个正常端点未列出,详见状态板」。
- 边界:全异常不渲染区与区头;全正常保留「全部 N 个端点稳定」陈述行(词走显示层映射);单模型模式不渲染。

## 480 窄版构成(全局/分组,GH #93 沿置 + GH #121 字排)

| 区块 | 720 完整版(基准) | 480 窄版 |
|---|---|---|
| 卡 | width 720(content-box),外盒 722 | width 480,外盒 482 |
| 品牌区 | padding 16px 40px;BrandMark 32 / Wordmark xl 20 / 标题 3xl 32 | padding 12px 20px;BrandMark 24 / Wordmark lg 16 / 标题 2xl 24;brand-bar 4px 不变 |
| card-body | padding 24px 40px 0 | padding 16px 20px 0 |
| scope chips | chip-value max-width 220px | max-width 160px |
| hero panel | padding 16px 20px;divider margin 0 20px;hero 72 大数字 | padding 12px 16px;divider margin 0 12px;**大数字降 3xl**(`.compact :deep()` 覆写,落点见上节) |
| 分段条 | 格高 16,格宽 ≈24.75 | 格高 16 不变,格宽 ≈16.4 |
| 异常明细 / 名单区 / 总结 | 见上 | 同构;名单仍 2 列(列宽 212) |
| 页脚 | margin 24px 40px 0;padding 16px 0 24px | margin 16px 20px 0;padding 12px 0 16px |

## 端点小卡构成(单模型紧凑版,GH #93 沿置 + GH #121)

480px 竖版紧凑物料,自上而下六区:① 品牌细条行(4px brand bar + BrandMark 16 + Wordmark sm,padding 8px 20px;无浅底无标题词);② 模型行(模型名 md/600 截断 + 协议 tag);③ 状态陈述行(`singleModelStatement` sm/600 *-text 阶 + failing 时 danger 实心点 + 「含告警」danger 描边 chip);④ 指标行(左「24h 可用率」xs label + 3xl/600 大数字 `av-*` 三档 *-text 阶——**GH #121 补登着色,修复 GH #93 起静默墨色 bug**;% 次级 md;右「平均延迟」xs label + xl 20/600 墨色,null → `-` +「24h 内无探测数据」);⑤ 迷你 24h 点条(格高 8px、2px 间距、radius-xs、dotTier 同源;轴标「24 小时前 / 现在」保留);⑥ 页脚(hairline + 生成时间 + origin)。

**不渲染(物料分工登记):** scope chips 行(Hub 是内部拓扑,完整范围声明在大卡)、评估区、status_reason 明细、一句话总结——异常由状态陈述行 tone + failing chip 承担,「不掩盖异常」约束保持;数字与大卡同源零变化。

## 分享弹窗响应式(GH #94/#95,沿置)

- **断点:** 全站唯一 768px(`(max-width: 767px)`)。
- **弹窗宽度:** `min(752px, 94vw)`;预览衬底 `--hs-bg-page` + space-4 内边距 + radius-lg;预览物料 `transform: scale(s)`,`s = min(1, 可用宽 / 外盒)`——可用宽 = dialog 内容宽 − 衬底 padding×2(`getComputedStyle` 实测扣除,GH #95 级联修复);容器显式高度 = 自然高 × s。
- **捕获不受影响:** 导出走离屏双份 absolute -10000px,不吃 transform;恒亮主题(暗色后置期天然满足)。
- **/report/:token 窄屏:** 榜单矩阵断点以下降级卡片式列表(算术证伪登记:最紧收敛 472 > 343;卡片式行保留前 3 名仪式、总分档色条、涨跌、ScoreCell `show-name` 维度格;排序入口收敛为工具条 el-select,降序唯一方向);进度网格模型列 96px、cell 词省略留点。打印不受断点影响。

## 级联算术表(GH #88 纪律:完整级联逐项取值,check 逐项复算)

**720 StatusCard(现状基准):** 卡根 width 720 content-box,外盒 722;body padding 24 40 0 → 内容 **640**;分段条格宽 (640 − 23×2)/24 ≈ **24.75**,格高 16;hero panel padding 16 20 → inner 600,divider 1 + 20×2 = 41;名单 2 列 (640 − 16)/2 = **312**;弹窗 width 752(border-box,EP padding-primary 16×2)→ content 720。

**480 窄版:** 卡 width 480 content-box → 外盒 482;body padding 16 20 0 → 内容 **440**;hero panel padding 12 16 → inner 408,divider 1 + 12×2 = 25;分段条格宽 (440 − 46)/24 ≈ **16.4**;名单 2 列 (440 − 16)/2 = **212**;chip-value max-width 160。

**端点小卡(480):** 内容 440;品牌行 padding 8 20;迷你点条格宽 ≈16.4,格高 8。

**弹窗缩放:** dialog = min(752, 94vw);预览可用宽 = (dialog − 32 EP padding-primary) − 32(衬底 space-4×2);s = min(1, 可用宽 / 外盒)(720→722,480→482);容器高 = 自然高 × s。

**/report 窄屏(375 基准):** 内容宽 343;卡片式维度格 ≈166;进度网格模型列 96。

**断点:** 767px max-width 单断点;版式默认档判定同一 768 值。

## 排版纪律(GH #95 沿置)

- 间距类 px 字面量一律 `--hs-space-*`(物料画布边距 720→40 / 480→20 是设计常量不进令牌,注释互指);刻度外值退役方向:结论行向宽(space-2)、分隔符向窄(space-1)。
- **残留 off-grid 保留:** 数字→档色条间距 `margin-top: 2px`(ScoreCell/EvalCard/Leaderboard 共 4 处)——数字与其正下档色细条是一体的「数字块」,2px 是块内光学贴合,入网格会把条读成独立元素;具名保留,不再扩散。

## 已知差距(登记)

- **StatusCardSingleModelMetrics.vue 两处 `:deep(.compact)` 残留**(hero-panel padding / metric-divider margin):GH #121 清除 StatusCardMetrics 死选择器时漏网;且 single-model + compact 恒渲染端点小卡独立模板,该组件的 compact prop 构造性不可达——双重死代码,视觉无影响,另立清理票。
