---
version: 1
slug: "web-src-components-share-materials"
primary_target: "web/src/components/StatusCard.vue"
related_targets: ["web/src/components/StatusCardDetail.vue","web/src/components/StatusCardMetrics.vue","web/src/components/StatusCardSingleModelMetrics.vue","web/src/components/EvalCard.vue","web/src/components/StatusShareDialog.vue","web/src/components/EvalShareDialog.vue","web/src/components/Leaderboard.vue","web/src/components/EvalProgressGrid.vue","web/src/views/CampaignReportView.vue","web/src/utils/statusCardSummary.ts","web/src/utils/statusCardSnapshot.ts","web/src/utils/cardImage.ts"]
---

# 分享物料与分享弹窗 — 表面简报

> 组件族 brief(首张非视图 brief,2026-07-31 GH #91 批 plan 裁决):StatusCard 族 / EvalCard / 两个分享弹窗是跨视图消费的品牌物料组件族,随任一视图 brief 拆散会破坏「一张物料一处规格」。本 brief 登记 GH #91 批(#92/#93/#94/#95)的新增构成规格与级联算术;批前的物料构成规格本体仍在 ui-guidelines §5(StatusCard / 单模型模式 / EvalCard / 外框约定各条),全量迁移另议,两处互指。业务语义(防作假同源口径、词表、静态物料双编码)以 ui-guidelines §3/§5 为准,本 brief 不重复。

## 范围与读者

- 读者:外部接收者(隔着一条分享链接/一张转发图,PRODUCT.md 第三受众)与转发者(状态板/管理台读者)。
- 物料纪律(ui-guidelines §5/§2a):恒亮主题导出、2x 导出、离屏捕获 `position: absolute; left: -10000px`、数字与范围 chips 同源、空态永不读作「全部正常」。
- **盒模型事实登记(GH #91 批 plan 实测):** 本仓无全局 `box-sizing` reset,物料卡根元素均为 content-box——`width: 720px` 是**内容盒**宽,外盒 = 722(1px×2 描边在盒外);「720 画布内容宽 640」(§5 EvalCard 条)即由此而来(720 − 40×2 padding)。480 族同(content-box,内容盒 480,body padding 20×2 → 内容 440)。弹窗预览缩放一律按**外盒**(722 / 482)计算。

## 版式体系(GH #93,2026-07-31 设计评审)

两档版式,同一份冻结快照的第二排版,数字口径零变化:

| 版式 | 设计宽 | 适用 | 构成 |
|---|---|---|---|
| 完整版 | 720px(2x 导出 1440) | 全局 / 分组 / 单模型 | 现状构成(ui-guidelines §5 StatusCard 条 ①–⑧),本批不动 |
| 紧凑版 | 480px(2x 导出 960) | 全局 / 分组 | 同构降宽,见下「480 窄版构成」 |
| 紧凑版(单模型)= 端点小卡 | 480px(2x 导出 960) | 单模型 | 独立紧凑构成,见下「端点小卡构成」 |

- **EvalCard 无紧凑版**(本批不引入;榜单物料的手机可读性由分享弹窗预览缩放承担),登记为后续可选。
- **版式切换:** StatusShareDialog 预览上方加「完整版 / 紧凑版」el-radio-group(size small);单组件挂点,Dashboard 全局分享、组分享、EndpointDetailView 单模型分享三入口同生效。切换只换预览与离屏双份的版式 prop,快照同一份。
- **默认档:** 打开弹窗时视口宽 < 768px → 紧凑版,≥768px → 完整版;打开时一次性 matchMedia 判定(onBannerInspect 同模式),不随 resize 重判、不持久化(转发意图每次不同)。与 #94 响应式断点同一 768 值。
- **文件名:** `cardFilename` 增 variant 段——紧凑版加 `-compact`(完整版不加,存量文件名习惯不破):`hubscope-status[-scope][-compact]-YYYYMMDD-HHmm.png`。
- 速览弹窗「不放分享入口」既有裁决(ui-guidelines §5 EndpointQuickViewDialog 条)不动——小卡入口只在 EndpointDetailView 的分享弹窗版式切换内。

## 480 窄版构成(全局/分组,GH #93)

与 720 完整版同构,逐项适配(720 版各项不动):

| 区块 | 720 完整版(基准) | 480 窄版 |
|---|---|---|
| 卡 | width 720(content-box),外盒 722 | width 480,外盒 482 |
| 品牌区 | padding 16px 40px;BrandMark 32 / Wordmark xl 20 / 标题 2xl 24 | padding 12px 20px;BrandMark 24 / Wordmark lg 16 / 标题 xl 20;brand-bar 4px 不变 |
| card-body | padding 24px 40px 0 | padding 16px 20px 0 |
| scope chips | chip-value max-width 220px | max-width 160px,余同构 |
| hero panel | padding 16px 20px;divider margin 0 20px;左右两列 | padding 12px 16px;divider margin 0 12px;**左右两列保持**(算术见下,左列 ≈307px 可容分布串与大数字纵排) |
| 分段条 | 格高 16,格宽 ≈24.75 | 格高 16 不变(跨版一致),格宽 ≈16.4(填满式随宽) |
| 异常明细 / 名单区 / 总结 | 见 §5 与本 brief 名单区节 | 同构;名单仍 2 列(列宽 212,截断既有机制) |
| 页脚 | margin 24px 40px 0;padding 16px 0 24px | margin 16px 20px 0;padding 12px 0 16px |

字阶/圆角/着色/令牌消费与 720 版完全一致;display 大数字不降档(Display Anchor Rule 跨版成立)。

## 端点小卡构成(单模型紧凑版,GH #93)

480px 竖版紧凑物料,自上而下固定六区:

1. **品牌细条行:** 4px brand bar + 品牌行(BrandMark 16px + Wordmark `--hs-text-sm` 13px,padding 8px 20px;无 `--hs-brand-soft` 整段衬底、无标题词——4px 品牌条承担品牌色;BrandMark 与 Wordmark 同场纪律不破)。
2. **模型行:** 模型名(`--hs-text-md`/600 primary,截断)+ 协议 tag(el-tag size small,`protocolTagType` 集中映射,GH #34 映射不动)。
3. **状态陈述行:** `singleModelStatement` 着色词(`--hs-text-sm`/600 tone,纯函数复用不另起措辞)+ failing 时橙实心点 + 「含告警」橙描边 chip(静态双编码,§3 静态物料约定不动)。
4. **指标行:** 左「24h 可用率」xs label + display 28px/600 大数字(§3 三档着色,% 次级 md secondary)——小卡唯一 display 消费(Display Anchor Rule:全卡最值得记住的数字);右「平均延迟」xs label + xl 20px/600 数字(墨色,null → `-` + 「24h 内无探测数据」注)。
5. **迷你 24h 点条:** 24 格分段填满式、格高 8px(明细行打点条同档迷你规格)、2px 间距、radius-xs、dotTier 三档同源;**条下轴标「24 小时前 / 现在」xs placeholder 保留**——全卡唯一时间条,方向必须自明(明细行打点条无轴标是十行并列的特例,小卡不适用)。
6. **页脚:** hairline + 左「生成于 YYYY-MM-DD HH:mm」+ 右 location.origin(xs placeholder)。

**不渲染(物料分工登记,信息差由大卡兜底):** scope chips 行(模型/协议已在行内;**Hub 名不进小卡**——转发读者关心模型服务本身,Hub 是内部拓扑,完整范围声明在大卡三枚 chips)、评估区(小卡是监控域快读物料,评估信息留在完整版)、异常明细(status_reason 不进小卡;异常由状态陈述行 tone 着色 + failing chip 承担,「不掩盖异常」约束保持)、一句话总结。单模型小卡数字全部来自同一 enabled entry,与大卡同源,口径零变化。

## 正常端点名单区(GH #92,2026-07-31 设计评审)

取代批 59「其余 N 个端点正常 · 24h 可用率区间」汇总行(语义裁决见 ui-guidelines §5 StatusCard 条 ⑥)。构成:

- **位置:** 异常明细区(含 overflow 收尾)之下,总结行之上;区头「正常端点」与「异常明细」detail-title 同规格(sm/600 secondary,上 24 下 8)。
- **网格:** CSS grid 2 列,column-gap 16px / row-gap 4px;720 版列宽 312px,480 版列宽 212px(算术见下节)。条目 = 模型名(`--hs-text-sm` primary,flex 1 min-width 0 截断)+ 24h 可用率(`--hs-text-sm`/600 右对齐,§3 三档着色 av-ok/partial/fail,null → `-` av-none)。**不带协议、不带点条。**
- **排序:** `success_rate_24h` 升序(与明细行同源字段;最脆弱的正常端点在前),null 沉底,同率 `model_id.localeCompare` 字典序;封顶 20 条(2 列 × 10 行);overflow 收尾「另有 N 个正常端点未列出,详见状态板」(sm secondary,与异常 overflow 同构)。
- **纯函数:** 排序/封顶抽 `utils/statusCardSummary.ts`(如 `healthyRoster(entries)`),vitest 覆盖升序 / null 沉底 / 字典序 tie / 封顶 / 空集。
- **边界:** 无正常端点(全异常)不渲染区与区头;全正常态保留「全部 N 个端点正常」陈述行(去区间后缀,`.detail-healthy` 规格不变)名单区接其下;单模型模式不渲染(「当前状态正常 · 24h 可用率 X」行保留)。

## 分享弹窗响应式(GH #94,2026-07-31 设计评审)

- **断点:** 全批唯一响应式断点 **768px**(`(max-width: 767px)` 生效;≥768px 桌面逐像素不变)。不设中间档。
- **弹窗宽度:** StatusShareDialog / EvalShareDialog 由 `752px` 改 `min(752px, 94vw)`(752 = 720 卡 + EP `--el-dialog-padding-primary` 16px×2,EP dist 实测)。
- **预览等比缩放:** 预览容器内物料 `transform: scale(s)`,`s = min(1, 预览可用宽 / 物料外盒宽)`(720 版外盒 722、480 版外盒 482),`transform-origin: top center`;容器显式高度 = 物料自然高 × s(ResizeObserver 量宽;transform 不改布局盒,高度必须补偿,否则下方留白/裁切)。缩放系数与补偿高度抽纯函数(vitest 覆盖双版式双宽度)。物料无交互,scale 不损害功能。
- **捕获不受影响论证(登记):** 导出永远走离屏双份(`.capture-source`,`position: absolute; left: -10000px`),它不在预览容器内、不吃 transform,按设计宽渲染——预览缩放只作用于预览 DOM;恒亮主题 reparent 捕获、absolute 离屏纪律不变。
- **现状登记:** 桌面 752 弹窗 content(752−32=720)< 720 卡外盒(722),既有 2px 溢出由 `.preview` overflow:auto 兜底,本批不动;缩放按外盒 722 计算后该溢出在缩放分支自然消失。

## /report/:token 窄屏(GH #94;CampaignReportView shared 模式,机制组件级生效)

**页头:** title-row 既有 flex-wrap 自适配(标题 / tags / 时间 / 操作折行),无结构变更。

**榜单矩阵 → 窄屏卡片式列表(本批最大设计题,裁决):** 断点以下 Leaderboard 行改渲染为纵向卡片块;桌面矩阵逐像素不变。候选评估:

| 候选 | 结论 | 理由 |
|---|---|---|
| 横向滚动容器 | 否 | §4 无横向滚动是承重纪律,无豁免先例 |
| 列收敛(隐涨跌列 + 压模型列) | 否 | **算术证伪**:375 − 32(页面 padding)= 343 内容宽;最紧收敛 = 名次 24 + 模型 100 + 总分 64 + 5 维度 × 44 + 8px × 8 gaps = **472 > 343**;维度数由数据决定,规格不能按典型值赌 |
| **卡片式列表(选定)** | 是 | 矩阵列 x 恒定不变式是桌面构造属性;窄屏换形态不换数据 |

卡片式行构成(断点以下;行块间 1px `--hs-border-light` hairline,与矩阵同节奏):

- 行 1:名次(前 3 名 brand lg/600 + 行块左缘 3px rail,非前 3 透明 rail 位,仪式感保留)+ 模型名(flex 截断)+ 总分(xl/600 墨色 + 其下 6px 档色条,0–100 恒刻度,轨道 `--hs-bg-hover`)。
- 行 2(基准可比时):涨跌(▲▼ + `formatScoreDelta`;占位 `–` 同口径;基准不可比整行不渲染)。
- 维度区:2 列网格(gap 8px,375 下格宽 ≈166px),每格 = 能力点名(xs secondary)+ 分数(md/600 档色)+ 4px 细条——**复用 ScoreCell,增 `show-name` prop**(交互面 prop,物料 staticMode 不受影响;「ScoreCell 是维度格子唯一组件」纪律不破)。
- live 半成品与判分不完整两模式同语义映射:名次弱化数字 / `–` 占位、水印(「判分不完整,缺 N/M 维度」模型名下第二行)、总分 `–` 空轨道、维度格状态词 / 空轨道、块尾「N 个维度进行中 · N 个失败」注——口径零变化(ui-guidelines §5 两条目不动)。
- **排序入口:** 窄屏无列头——排序走工具条 el-select(选项 = 总分 + 各能力点,降序唯一方向,禁前端 reverse 第二口径纪律不动),仅断点以下渲染;family 筛选工具条 wrap;桌面列头排序不动。shared 模式行不可点、无下钻不动;控制台 selectable 语义卡片块整行同效。
- **断点判定:** matchMedia('(max-width: 767px)') + change 监听,组件级统一实现(禁各消费方自造第二套断点逻辑)。

**进度网格(EvalProgressGrid)窄屏:** 模型列 220px → 96px(截断 + title 已有);cell 内状态词省略、只渲染状态圆点(8px,§3 批次/运行状态色映射不动;全信息由既有 cell tooltip「状态词 · X/Y 题 · 耗时 · Token」兜底——桌面 tooltip 本就承载该口径);覆盖率「X/Y 题」注在 96px 列下折行或省略由实现按不溢出定,网格四态语义零变化。card-top 批次汇总行既有 flex-wrap 自适配。

**轮询 / 三态 / 暗色:** visibilityPoll、settle 转场口径、三态、暗色双主题全部不动。打印:断点基于 layout viewport,桌面打印(≥768)不受影响;窄屏设备打印输出随当前形态(卡片式),登记为已知并接受(可读性反而更优)。

## 排版精修验收清单(GH #95;依赖 #92–#94 落地后执行)

全部消费既有语义令牌与刻度,零新刻度、零新色相、零硬编码:

1. **间距令牌化:** StatusCard 族 / EvalCard / 两个分享弹窗 scoped style 中间距类 px 字面量(gap/margin/padding 中落 4px 网格者)迁移 `--hs-space-*`(§2 渐进迁移既定方向,分享物料族本批迁完);物料画布边距(720→40 横向 / 480→20 横向、小卡 20)是物料设计常量不进令牌,注释与本 brief 互指。
2. **hero panel 右列基线:** 右列 `padding-bottom: 2px` 魔法数退役,改 `var(--hs-space-1)`(4px 网格内最近值,消灭刻度外 2px);720/480 双宽度同改。
3. **chips 行:** gap 8px(space-2)、chip padding 2px 8px 不动;chip-value max-width 分档核对(720→220 / 480→160)。
4. **纵向节奏核对:** 名单区(区头 24/8、行距 4、overflow 8)与小卡(品牌行 8、区块 12/16)并入 4px 网格核对表。
5. **页脚:** hairline + 左右分置 + baseline 对齐不动;小卡/窄版页脚节奏按本 brief 规格核对。
6. **弹窗预览:** 预览区加 `--hs-bg-page` 衬底 + `--hs-space-4` 内边距 + `--hs-radius-lg`——物料在预览中读作「桌面上的卡」,衬底 = 页面级中性底语义(hero panel 同令牌先例);居中保持;480/720 双版式 × 亮暗 × 缩放三态视觉核对。**(2026-07-31 GH #95 执行注记)** 衬底内边距计入缩放级联:`clientWidth` 含预览自身 padding,可用宽必须再减 space-4×2,否则缩放后的卡溢出出横向滚动条(§4)——两弹窗 `updatePreviewScale` 已改经 `getComputedStyle` 减 padding,级联表「弹窗缩放」行同步修订。
7. **刻度外值退役(GH #95 执行增补;check MEDIUM-1 补全):** hero-verdict gap/margin 6px → `--hs-space-2`、failing-chip padding 0 6px → `0 --hs-space-2`、statement gap 6px → `--hs-space-2`(单模型 statement margin-top 6px 同 → `--hs-space-2`)、metric-unit margin-left 2px → `--hs-space-1`、分布串分隔符 `::before` margin 0 6px → `0 --hs-space-1`、dist-label margin-right 3px → `--hs-space-1`、明细行 row-reason margin-top 2px → `--hs-space-1`——与第 2 项同方向(刻度外值就近入网格),双宽度同改;chip padding 2px 8px 的纵向 2px 保留(chips 行既定规格,第 3 项)。**退役方向取舍(check LOW-1 登记):** 同为 6px,结论行(verdict/statement)向宽退役到 space-2(8px)——结论行要呼吸、与 alert dot/chip 的间隔是视觉分组;分布串分隔符向窄退役到 space-1(4px)——分隔符要紧凑,四段恒列在 480 宽度下不为分隔让位。**残留 off-grid 保留清单(check LOW-2 登记):** 数字→档色条间距 `margin-top: 2px`(ScoreCell.vue、EvalCard.vue、Leaderboard.vue 矩阵行与卡片行共 4 处)保留不入网格——数字与其正下档色细条是一体的「数字块」,2px 是块内光学贴合,入网格(4px)会把条读成独立元素;此为刻度下限之下的光学微调的具名保留,不再扩散。

## 级联算术表(GH #88 纪律:完整级联逐项取值,check 逐项复算)

**720 StatusCard(现状基准,实测登记):**

| 级联项 | 取值 | 出处 |
|---|---|---|
| 卡根 | width 720,**content-box**(无全局 reset),border 1px×2 → 外盒 722 | StatusCard.vue `.status-card` |
| body | padding 24px 40px 0 → 内容宽 720 − 80 = **640** | `.card-body`(与 §5 EvalCard「内容宽 640」同口径) |
| 分段条 | 格宽 = (640 − 23×2) / 24 ≈ **24.75px**,格高 16,间距 2,radius-xs | StatusCardMetrics `.uptime-strip` |
| hero panel | padding 16 20 → inner 600;divider 1 + margin 20×2 = 41;左列 flex 1 | `.hero-panel` |
| 名单区(#92 新增) | 2 列 gap 16 → 列宽 = (640 − 16) / 2 = **312px** | 本 brief |
| 弹窗 | width 752(border-box,EP `.el-dialog` 自带)= 720 + padding-primary 16×2 → content 720 | EP dist 实测 |

**480 窄版(本批新登):** 卡 width 480 content-box → 外盒 482;body padding 16px 20px 0 → 内容 **440**;品牌区 padding 12 20;hero panel padding 12 16 → inner 408,divider 1 + 12×2 = 25,右列 ≈76(xl 数字 + xs label)→ 左列 ≈ **307**(分布串典型 ≈230、display 大数字 ≈110,纵排均可容);分段条格宽 = (440 − 46) / 24 ≈ **16.4px**,格高 16;名单 2 列 = (440 − 16) / 2 = **212px**;chip-value max-width 160;页脚 margin 16 20 0 / padding 12 0 16。

**端点小卡(480):** 内容 440;品牌行 padding 8 20;迷你点条格宽 = (440 − 46) / 24 ≈ **16.4px**,格高 8;指标行左右两列 flex。

**弹窗缩放:** dialog = min(752, 94vw);预览可用宽 = (dialog − 32 EP padding-primary) − 32(预览衬底 space-4×2)(2026-07-31 GH #95 修订:衬底 padding 原漏算,`updatePreviewScale` 经 `getComputedStyle` 实测扣除,防缩放卡溢出横向滚动);s = min(1, 可用宽 / 外盒)(720→722,480→482);容器高 = 自然高 × s。

**/report 窄屏(375 基准):** 内容宽 = 375 − 16×2(`.report-page` padding)= **343**;矩阵收敛假说 = 24 + 100 + 64 + 5×44 + 8×8 = **472 > 343(证伪)**;卡片式维度格 = (343 − 3(rail)− 8(gap)) / 2 ≈ **166px**;进度网格 = 模型列 96 + N × flex 1,cell dot 8px。

**断点:** 767px max-width 单断点;版式默认档判定同一 768 值。
