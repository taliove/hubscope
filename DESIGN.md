---
# 摘要索引面(非穷举)——normative 唯一来源是正文,见正文首段声明(2026-07-30 裁决,check LOW-4)。
name: HubScope
description: 对内 LLM 端点监控与质量评估——状态板答「健不健康」,评估榜单答「哪个模型好」
colors:
  primary: "#0b7a72"
  primary-hover: "#0faea2"
  primary-active: "#095f59"
  primary-soft: "#effcfa"
  success: "#059669"
  success-text: "#047857"
  warning: "#a16207"
  danger: "#dc2626"
  info: "#40525a"
  failing: "#c2410c"
  page-bg: "#f3f6f7"
  surface: "#ffffff"
  surface-hover: "#e9eef0"
  text-primary: "#0b151a"
  text-regular: "#2c3b42"
  text-secondary: "#5a6a71"
  text-placeholder: "#8a99a0"
  border: "#dde4e7"
  border-light: "#edf2f4"
typography:
  display:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'PingFang SC', 'Microsoft YaHei', sans-serif"
    fontSize: "28px"
    fontWeight: 600
    lineHeight: 1.2
  headline:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'PingFang SC', 'Microsoft YaHei', sans-serif"
    fontSize: "24px"
    fontWeight: 600
    lineHeight: 1.2
  title:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'PingFang SC', 'Microsoft YaHei', sans-serif"
    fontSize: "20px"
    fontWeight: 600
    lineHeight: 1.2
  body:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'PingFang SC', 'Microsoft YaHei', sans-serif"
    fontSize: "14px"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'PingFang SC', 'Microsoft YaHei', sans-serif"
    fontSize: "12px"
    fontWeight: 400
    lineHeight: 1.5
rounded:
  xs: "2px"
  sm: "4px"
  lg: "8px"
  full: "999px"
spacing:
  space-1: "4px"
  space-2: "8px"
  space-3: "12px"
  space-4: "16px"
  space-5: "24px"
  space-6: "32px"
  space-7: "48px"
  space-8: "64px"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "#ffffff"
    rounded: "{rounded.sm}"
    padding: "0 16px"
    height: "32px"
  button-primary-hover:
    backgroundColor: "{colors.primary-hover}"
    textColor: "#ffffff"
    rounded: "{rounded.sm}"
    padding: "0 16px"
    height: "32px"
  card:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.lg}"
    padding: "16px"
  input:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text-regular}"
    rounded: "{rounded.sm}"
    height: "32px"
    padding: "0 12px"
  tag:
    backgroundColor: "{colors.surface-hover}"
    textColor: "{colors.text-regular}"
    rounded: "{rounded.sm}"
    padding: "0 8px"
    height: "24px"
---

# Design System: HubScope

> **本文件的 normative 结构(2026-07-30 裁决,check LOW-4):** frontmatter 是 impeccable 工具链消费的**摘要索引面,非穷举**——`*-soft` 功能浅底、`--hs-overlay-bg` 浮层衬底、`--hs-blink` 闪烁令牌等只在正文登记,不在 frontmatter。**normative 唯一来源是正文**:正文登记的令牌与规则即生效,不以是否入 frontmatter 为条件。新令牌先入正文;是否回登 frontmatter 按「工具链是否需要索引」判断,不作强制,也不以 frontmatter 缺席推断令牌不存在。

## Overview

**Creative North Star: "值班室信号墙 (The On-Call Signal Wall)"**

HubScope 的界面是一堵值班室的信号墙:状态即证据,严重度自动浮顶,一切为扫读与投屏服务。它的读者分三种——3 秒看屏的状态板读者、高密度作业的管理台读者、隔着一条分享链接的外部接收者——墙对这三种人都必须诚实:结论永远标注统计范围,没有数据就断线,失败不冒充空态。

审美语调:**克制、精确、冷峻**。灰阶为主,用色克制,描边分层;情绪由语义色承担,不由装饰承担。装饰性元素(渐变装饰、大圆角、彩色背景块、营销物料)在这堵墙上没有位置——唯一的渐变是 BrandMark 图形标本身。

**Key Characteristics:**
- 三层令牌架构(原始刻度 → 语义令牌 → Element Plus 映射),亮/暗双主题一等公民,暗色只覆盖语义层
- Flat-by-default:静态靠 1px 描边分层,阴影只表达「可点/浮层」
- 4px 基准网格,密度按读者分档(消费页 16px / 管理台 12px)
- 状态双编码:色 + 词永远同场,闪烁动画是 failing 告警的独占语义
- Element Plus 组件体系,不自造表单/弹窗/表格/分页

## Colors

调色板是「一族青 + 一组功能色 + 一个具名例外」:电波青承担品牌与交互,青灰承担全部中性表达,功能四色承担状态语义,告警橙是唯一的调色板外例外。

### Primary
- **电波青 Teal-600**(#0b7a72):亮主题主色,白底对比度 5.2:1 过 WCAG AA(2026-07-30 同族精修从 #0c8078 4.8:1 加深微调,余量加大,GH #69)。主按钮、链接、当前导航、聚焦态、「运行中」状态。按下态 active 同步加深为 teal-700(#095f59),brand/hover/active 三阶阶差保持(brand↔active 对比 1.45,优于旧 1.36)。暗主题主色提亮为 teal-500(#0faea2)不变——电波带(teal-400/500)在白底对比度不足,只作暗色主色与图形点缀,永不作白底文字。
- **电波青浅底 Teal-50**(#effcfa):品牌浅底,选中行/高亮块/品牌区衬底;暗主题为 teal-900(精修后 #053836)。

### Neutral(2026-07-30 同族精修:加深提纯——中性更「墨」,降彩度微加深,保持青灰家族感,GH #69)
- **青灰 Gray-900**(#0b151a):主文字,低彩度同族青灰,非纯黑。暗主题 #e2e8f0(不变)。
- **青灰 Gray-700**(#2c3b42):常规正文。暗主题 #c9ced4。
- **青灰 Gray-500**(#5a6a71):次要说明、标签、辅助;暗主题 #9298a2。暗色 info 与灰阶解耦为 #6b7d84(gray-500 加深后暗底可读性下降,暗 info 单独提亮,暗卡上 4.1:1)。
- **青灰 Gray-400**(#8a99a0):占位/禁用/等待中。暗主题 #5c616a(不变)。
- **青灰 Gray-50/100/200**(#f3f6f7 / #e9eef0 / #dde4e7):页面底、悬浮/轨道、描边——表面三层拉开(page↔card↔hover 两两可分辨)。**hairline(border-light)= #edf2f4**:介于 page 与 hover 之间的发丝线,与 hover 解耦(精修前与 gray-100 同值),白卡上 1.13:1 可辨不抢;暗主题 #1e2227(暗卡上 1.10:1)。暗主题表面三层为 #0c0e11 / #17191d / #22262b,描边 #2a2d33(不变)。

### Functional(亮暗基本同值,warning 例外分档,承担状态语义)
- **Success**(#059669):正常 / 已完成 / 分数升——**图形填充专用**(状态圆点、榜单条形、24h 分段格、soft 浅底、徽章),本体白底 3.77:1 过图形 3:1 门槛,不动。
- **Success-Text**(`--hs-success-text`,亮 #047857 / 暗 #10b981):**一切文字场景的 success**——正常状态词、绿档分数、升箭头。亮值 emerald-700 白底 5.48:1 过 AA(与 EP dark-2 派生 #047854 天然同值);暗值 emerald-500 暗卡 6.94:1。2026-07-30 登记(GH #69 T2;GH #62 落点,裁决见 ui-guidelines 附录 B #15)。
- **Warning**(#a16207，亮主题;暗主题保留 #d97706):降级 / 可逆警示。亮值 2026-07-29 修订(appendix B #14):旧值 #d97706 白底实测仅 3.19:1 不过 AA,改 yellow-700(4.92:1)且色相比旧值更远离 failing 橙。
- **Danger**(#dc2626):宕机 / 失败 / 分数降 / 不可逆操作。
- **Info**(亮 #40525a / 暗 #6b7d84):中性提示,与青灰同族(2026-07-30 精修:亮值随 gray-600 加深,暗值与 gray-500 解耦提亮)。

### The Failing Exception
- **告警橙 Orange-700**(#c2410c,亮主题;暗主题 orange-400 #fb923c):failing 告警专属,「调色板外不引入新色相」纪律的唯一具名例外——告警辨识度 = 告警可信度。白底 5.1:1 过 AA;与 warning 差一个色相族、与 danger 明度接近而色相向橙偏移,在黄红之间建立第三紧急档。永不泛化为装饰色,永不用于批次/运行状态。

### 同族精修登记(2026-07-30,GH #69 批,/impeccable shape 定稿)

编辑感精密工具路线下的调色板精修:不引入任何新色相,功能四色与 failing 橙不动,全部变化在电波青与青灰两族之内。旧值 → 新值(亮/暗双份):

| 令牌 | 旧值(亮/暗) | 新值(亮/暗) | 实测 |
|---|---|---|---|
| brand(teal-600) | #0c8078 / #0faea2 | **#0b7a72** / 不变 | 白底 4.80 → 5.20:1 |
| brand-active(teal-700) | #0a6963 / teal-600 | **#095f59** / 随档 | brand↔active 1.36 → 1.45 |
| teal-800 / teal-900 | #085350 / #063f3d | **#074842 / #053836** | 刻度尾段同步加深 |
| bg-page(gray-50) | #f7fafb / #0f1115 | **#f3f6f7 / #0c0e11** | 与白卡 1.049 → 1.086 |
| bg-hover(gray-100) | #eff4f5 / #1f2227 | **#e9eef0 / #22262b** | 与白卡 1.109 → 1.170 |
| border(gray-200) | #e0e8ea / #2a2d33 | **#dde4e7 / 不变** | 与白卡 1.243 → 1.286 |
| border-light(hairline) | =gray-100 / #23262b | **#edf2f4 / #1e2227** | 与 hover 解耦;白卡 1.13 / 暗卡 1.10 |
| text-primary(gray-900) | #0f1b20 / #e2e8f0 | **#0b151a / 不变** | 17.5 → 18.5:1 |
| text-regular(gray-700) | #324249 / #c3c7cd | **#2c3b42 / #c9ced4** | 10.4 → 11.6 / 10.4 → 11.1 |
| text-secondary(gray-500) | #617379 / #8a8f98 | **#5a6a71 / #9298a2** | 4.96 → 5.62 / 5.42 → 6.06 |
| text-placeholder(gray-400) | #91a3a8 / #5c616a | **#8a99a0 / 不变** | 2.62 → 2.94(占位档刻意 muted) |
| info(gray-600) | #45565c / =gray-500 | **#40525a / #6b7d84** | 7.67 → 8.16 / 3.55 → 4.10(解耦提亮) |
| success-text(新增) | — | **#047857 / #10b981** | 白底 5.48 / 暗卡 6.94 |
| gray-300(刻度插值) | #c7d3d6 / 同值 | **#c3ced2** / 同值 | 零消费方,check T1 LOW-2 批末补登 |
| gray-800(刻度插值) | #1e2b31 / 同值 | **#1a262c** / 同值 | 零消费方,check T1 LOW-2 批末补登 |

精修纪律:① brand 只许加深不许提亮(AA 红线 4.5:1,精修后 5.20 有余量);② hairline 必须保持在白卡与暗卡上可辨(投屏/远距红线,亮 ≥1.10 / 暗 ≥1.08);③ 暗色表面三层两两可分辨(page .0043 / card .0097 / hover .0190 亮度阶);④ 暗色 soft 浅底 color-mix 基底随 bg-page 同步(#0f1115 → #0c0e11);⑤ BrandMark/favicon 渐变 stop 表现属性随 teal-700 同步(#0a6963 → #095f59,令牌链兜底纪律不变);⑥ **chartColors 镜像同步清单**(utils/chartColors.ts,check 按本清单逐字段验收):LIGHT 六字段——brand `#0c8078`→`#0b7a72`、textRegular `#324249`→`#2c3b42`、textSecondary `#617379`→`#5a6a71`、placeholder `#91a3a8`→`#8a99a0`、border `#e0e8ea`→`#dde4e7`、bgHover `#eff4f5`→`#e9eef0`;DARK 三字段——textRegular `#c3c7cd`→`#c9ced4`、textSecondary `#8a8f98`→`#9298a2`、bgHover `#1f2227`→`#22262b`。不动项:success / warning / danger / failing 亮暗双份、dark 侧 brand / placeholder / border;不增设 successText 镜像字段(图表内无 success 文字场景)。

### Named Rules

**The One Voice Rule.** 品牌电波青在任何单屏上是稀缺资源:主按钮一处、当前导航一处、聚焦态若干。它不渲染大背景、不染卡片、不作标题色——它的稀有性就是它的信号强度。

**The Double-Encoding Rule.** 状态永远色 + 词双编码同场:灯(圆点/图标)承担色,文字承担词。词随灯着色(GH #71)后,着色词自身即双编码的文本形态——聚合/重复场景允许 dotless 着色词单独出场(灯与词分层,见「信号墙灯与词分层」节);实体信号位(卡片头灯、详情页/弹窗 Badge)灯不缺席。任何只用颜色表达状态的呈现都是缺陷(静态导出物料用着色文字转译,不复制状态灯)。

**The Blink-Is-Failing Rule.** 全站唯一动画是 failing 告警的 hs-blink 闪烁。运行状态、字标、装饰元素一律静止——「有东西在闪」的直觉解读必须是「有情况」。本条管**持续/循环/环境动效**;用户触发的单次状态过渡(披露、hover、聚焦)走 `--hs-transition`,不受本条限制(2026-07-29 /impeccable animate 评审澄清)。

## Typography

**Body Font:** 系统字栈(system-ui / PingFang SC / Microsoft YaHei 回退),零外部字体文件——与单二进制交付一致。
**Mono Font:** 系统等宽字栈(ui-monospace / SF Mono / Consolas),仅用于 Wordmark 字标(字重 700)。

**Character:** 无字体个性的刻意选择——工具产品的字形权威来自层级与字重,不来自字体。字重只用 400/600/700(700 仅限 Wordmark);行高默认 1.5,数字类 1.2。

### Hierarchy
- **Display**(600, 28px, 1.2):消费页主视觉大数字专用——健康横幅大字结论、可用率大数字。禁用于管理台标题与正文。
- **Headline**(600, 24px, 1.2):品牌区标题(「HubScope 服务状态」)。
- **Title**(600, 20px, 1.2):页面标题、关键数字(平均延迟)、分组标题(2026-07-30 组头升档,GH #74——分组标题是状态板次层级锚点)。
- **Subtitle**(600, 16px, 1.5):卡片标题。
- **Body**(400, 14px, 1.5):正文基准、表单、表格主列。
- **Body-Small**(400, 13px, 1.5):次要正文、状态词。
- **Label**(400, 12px, 1.5):辅助/标签/时间戳。

### Named Rules

**The Display Anchor Rule.** display 档是 3 秒读者的视觉锚点,一页至多一两处,且永远是「最值得记住的那个数字」。把它用在标题或正文上,等于拆掉锚点。

## Layout

内容区 max-width 1200px 居中,桌面优先——不为手机做专门适配,窄屏不阻断即可。4px 基准网格,间距消费 space-1..8 刻度;区块间距 16px,页面上下 24px。

**密度按读者分档:** 消费页(状态板、评估榜单、分享报告)卡片内边距 16px;管理台(配置与排查场景)紧凑档 12px——档位由读者决定,不由登录态决定。

页面结构:顶部标题/操作区 → 内容区(卡片或表格)→ 必要时底部辅助区。弹性列宽优先于固定宽度;卡片内容不得溢出、不得出现横向滚动条;管理台表格的弹性内容列必须显式 min-width,使全表最小宽度可算术核对。

### Named Rules

**The Density-By-Reader Rule.** 密度档位看读者不看登录态:3 秒场景给呼吸,作业场景给密度。同一张卡片不因「在哪」而变密度,只因「给谁」而变。

## Elevation & Depth

Flat-by-default。深度由 1px 描边与表面色阶(page → card → hover 三层)承担;阴影不是装饰,是交互语义——它只回答一个问题:「这东西能点吗 / 这东西浮在页面上吗」。

### Shadow Vocabulary(亮主题基准,暗主题同名令牌换黑基底)
- **sm**(`0 1px 2px rgba(15,27,32,.05)` / 暗 `rgba(0,0,0,.3)`):最轻浮起,少用。
- **md**(`0 2px 8px rgba(15,27,32,.08)` / 暗 `0 2px 12px rgba(0,0,0,.4)`):可点卡片 hover 反馈。
- **lg**(`0 8px 24px rgba(15,27,32,.12)` / 暗 `0 6px 24px rgba(0,0,0,.5)`):浮层——弹窗、抽屉、下拉。

### Named Rules

**The Flat-By-Default Rule.** 静态卡片 `shadow="never"` + 1px 描边。阴影出现当且仅当:hover 的可点卡片,或浮层。静态元素吃阴影 = 谎报可点性。

### Overlay Scrim(2026-07-29 登记)
- **`--hs-overlay-bg`**(亮 `rgba(15,27,32,.40)` / 暗 `rgba(0,0,0,.55)`):全站唯一浮层衬底色,EP overlay 经 ep-theme.css 统一映射,不存第二套衬底语言。
- **雾化(backdrop-filter blur 8px)是速览弹窗专属例外**:blur 承载「卡片墙还在后面」的连续性隐喻;管理台弹窗是作业工具,不雾化。blur 值为实现常量,不进令牌刻度。(原登记「与翻转编舞同源」的表述随翻转编舞 2026-07-30 退役删除——雾化本身保留,用户从未否定。)

## Shapes

圆角四档,按元素层级分配,无默认档(6px 已退役):

- **xs(2px)**:分段条/时间条类填充元素(24h 可用率条格),仅限此类。
- **sm(4px)**:控件默认——按钮、输入框、tag、评分徽标。
- **lg(8px)**:面板层级——卡片、弹窗、抽屉。
- **full(999px)**:胶囊元素(计数 chip),按需。

形态语言是「克制的方」:直边、细描边、小圆角,无大圆角、无胶囊按钮、无异形裁剪。分段条是唯一的填满式时间形态:24 格等分填满轨道,2px 间距,格即一小时。

### Named Rules

**The Radius-By-Layer Rule.** 圆角跟着层级走,不跟着喜好走:控件 4、面板 8、时间格 2、胶囊 999。新元素先问自己属于哪一层,再取圆角。

## Motion

动效是克制的:全站持续动画只有一例(failing 闪烁,Blink-Is-Failing Rule),其余动效全部是用户触发的单次过渡,且只有一档速度。

### Transition Scale(tokens.css)
- **default**(`--hs-transition`,0.2s cubic-bezier(0.4, 0, 0.2, 1)):全站唯一过渡档——hover/focus 反馈、披露容器、状态切换,以及弹窗入场(安静入场:opacity + scale(0.96→1) 中心淡入,无位移无翻转)。

**focal 档已退役(历史登记,防复活):** `--hs-transition-focal`(0.32s cubic-bezier(0.16, 1, 0.3, 1),焦点入场档)随速览弹窗 morph 编舞于 2026-07-30 被用户实机裁决整体退役——「不做翻转放大吧」「动画也很丑」;令牌已随零消费方从 tokens.css 移除,编舞延迟常量(80/70ms 搭接、perspective 1600px)同批清理。方向定稿:工具风「反馈在,表演不在」——入场只给存在感,不给表演。任何「主体变形进入视野中心」的编舞提案须先过设计评审并正面回应本次否决理由。

### Named Rules

**The Single-Speed Rule.** 全站只有一档过渡速度:0.2s 给一切反馈与入场(含弹窗安静入场)。不引入第二档;拿不准就用 default。(2026-07-30 前为 Two-Speed Rule,0.32s focal 焦点入场档随速览弹窗 morph 编舞同日退役——历史登记见上。)

**The Gate-The-Phases Rule.** reduced-motion 下 CSS 过渡由全局归零兜底,但 JS 阶段时序(setTimeout/rAF 编舞延迟)必须单独门控:reduced-motion 时全部延迟归零、终态直呈——延迟不是装饰,是等待。(首例速览弹窗编舞已随 2026-07-30 退役,现无在编多阶段动效;本条作为未来编舞的纪律保留。)

## Components

组件体系的通则;页面级构成规格在各 surface brief。

### Buttons
- **Shape:** 控件圆角(4px),默认高 32px。
- **Primary:** 电波青底白字;hover 提亮一档(teal-500),按下加深一档(teal-700)。
- **Text / link 型:** 无色底,文字电波青;用于卡片内操作与图标按钮(分享入口、主题切换)。
- **反馈:** 点击类操作请求期间给 loading 或禁用态,不静默等待。
- **焦点单语言(2026-07-29 登记,全站唯一焦点样式):** `:focus-visible` = `outline: none` + 1px brand 内嵌描边(`box-shadow: inset 0 0 0 1px var(--hs-brand)`),可点元素可叠加 `--hs-shadow-md`(与 hover 反馈配对);EP 组件焦点态由 ep-theme.css 统一映射,自定义可点元素(button 化的快捷项、role=button/link 的卡片)一律用此语言,不引入第二种焦点样式。

### Cards / Containers
- **Corner Style:** 面板圆角(8px)。
- **Background:** 卡面 `#ffffff`(暗 #17191d),页面底低一档。
- **Shadow Strategy:** Flat-by-default——静态卡 shadow="never" + 1px 描边;可点卡 hover md 阴影(见 Elevation)。
- **Internal Padding:** 消费页 16px / 管理台 12px,走 `--el-card-padding` 变量,禁止覆写卡身。

### Inputs / Fields
- **Style:** 1px 描边、控件圆角、卡面底。
- **Focus:** 电波青描边/聚焦态。
- **管理台限宽:** 长输入(URL/webhook)560px,标准输入 320px,数字/开关/下拉用组件默认宽——表单控件不拉满整卡。

### Tags
- **Style:** 控件圆角、size small、effect light;语义色走 `type` 属性,不自着色。
- **用途纪律:** 角色 tag(primary/info)、协议 tag(success/warning/info)、判定方式 tag(info)各有集中映射函数;同一语义域内词表封闭。

### Navigation
- **Style:** 顶部 AppHeader:品牌位(BrandMark + Wordmark)在左,导航项居中偏左,操作区在右。
- **激活态:** 当前导航电波青 + 下划线;导航按登录态过滤,未登录只见公开页项。

### Signature: StatusBadge(唯一状态灯)
- 全站唯一的 endpoint 状态展示组件:语义色圆点 + 状态词,四态(正常/降级/宕机/告警),告警态橙 + hs-blink 闪烁;degraded 可挂成因副标签(「· 可用性」「· 延迟」),副标签无独立圆点/底色。需要展示状态处一律复用,禁止第二个状态灯实现。
- **词随灯着色(2026-07-30,GH #71,shape 定稿):** 状态词颜色 = 状态语义色(degraded=warning / down=danger / failing=failing 本体,白底均过 AA;healthy=`--hs-success-text` 深阶)——色+词双编码从「点色词墨」升级为「点词同色」。圆点尺寸 sm 9px / md 11px(自 10/12 微调)。
- **dotless 变体(2026-07-30 实机迭代批 GH #80,shape 定稿;GH #81/#82):** 词随灯着色后,着色词自身已满足色+词双编码,聚合/重复场景允许去点渲染(仅着色词),防一屏多灯稀释信号。封闭适用清单(仅三处,其余消费方一律带点):① EndpointCard 状态行(头行信号墙灯已是全卡唯一状态灯);② Hero 带计数行;③ 组头计数 chips。详情页、速览弹窗、管理台表格等消费方保持带点——它们是实体信号位或独立信号场景,灯不缺席。failing dotless 词不闪烁——闪烁是灯的语言,词只着色。语义与闪烁位置清单见下节「信号墙灯与词分层」。

### Signature: 信号墙灯与词分层(2026-07-30,GH #72,shape 定稿;同日实机迭代批升级为「灯与词分层」完整语义)
- EndpointCard 头行右端的内联状态圆点:9px,状态色,**仅 status ≠ healthy 渲染**——健康通道灭灯,异常才亮灯;failing 灯走 `--hs-blink` 闪烁(闪烁=failing 独占语义在信号墙上的正主)。灯是卡片级状态标记:无词、不可点、`aria-hidden`(状态词由 StatusBadge 承担,a11y 树不重复报状态),**不是第二个 StatusBadge**;聚合层(计数行/组头)永不使用。
- **灯与词分层(2026-07-30 实机迭代批 GH #80,用户实机反馈「灯看花眼」;GH #81/#82):** 状态表达分两层——**灯 = 实体信号位**,只在「该对象的唯一/首要状态信号」处出现:卡片头灯、详情页与速览弹窗的 Badge 圆点;**着色词 = 聚合/重复场景的文本证据**:卡片状态行 StatusBadge、Hero 计数行、组头计数 chips(三处 dotless,封闭清单见 StatusBadge 节,禁止向第四处扩散)。**每卡一灯:** 头行灯是全卡唯一状态灯,状态行 Badge 去点。**闪烁位置封闭清单:** 卡片头灯(failing)+ Hero 带 alert-dot(failing)+ 带点 Badge 的 failing 点(详情页/弹窗/管理台等一切带点消费方)——全站不新增第四处,dotless 词永不闪烁。

### Signature: Hero 指挥台带(2026-07-30,GH #73,shape 定稿)
- 状态板首屏的单表面指挥带:大字结论 + 24h 可用率大数字 + 异常端点 chips + 计数行合一,取代「banner 卡 + stats strip」两段。**构图(2026-07-30 实机迭代批修订,用户实机反馈「可用率沉底」;GH #81):** 行 1 = 大字结论 + 可用率大数字右端同基线;「24h 可用率」label 与 meta(更新于/stale)在其下;异常 chips 与计数行全宽居下两行——原「右列垂直居中沉底」两列构图废弃(锚点沉底违反「视觉权重 = 业务严重度」)。带底沿用 tone-soft 四态浅底(空态/首载中性 bg-page),**无圆角无描边盒,底部 1px border-light hairline 收边**。**Display 锚点纪律重申:** 带内 display 档仅两处——大字结论与可用率大数字,本页不再新增任何 display 消费。页面级构成细则见 dashboard surface brief。

### Signature: 24h 分段条
- 24 格填满式时间条(格高 16px/8px,xs 圆角,2px 间距),格 = 一小时,三档着色(≥95% 绿 / <95% 黄 / 0% 红 / 无数据灰);聚合口径 = 按小时求和,禁止按端点简单平均。交互面(EndpointCard、速览弹窗)一律消费共享组件 EndpointUptimePanel;StatusCard 静态物料保持独立实现(快照渲染契约,ticket 76 先例)。

## Do's and Don'ts

### Do:
- **Do** 只消费语义令牌(`--hs-*` 语义层);调具体色值改 tokens.css,调暗色观感改 semantics.css 的 html.dark 块。
- **Do** 让严重度驱动组织:最紧急的信息最先被看见,视觉权重 = 业务严重度。
- **Do** 用描边与表面色阶分层,静态元素保持 flat。
- **Do** 状态色 + 词双编码;长文本截断 + hover 全显;空/加载/错误三态齐备。
- **Do** 导出物料恒亮主题渲染,离屏捕获用 `position: absolute; left: -10000px`。

### Don't:
- **Don't** 硬编码色值、书写 `--el-*`(唯一允许处 ep-theme.css)、使用刻度外 z-index/间距/圆角字面量。
- **Don't** 引入调色板外新色相(failing 橙是唯一具名例外,且永不泛化)。
- **Don't** 给运行状态、字标、装饰元素加动画——闪烁是 failing 告警的独占语义。
- **Don't** 用渐变装饰、大圆角、彩色背景块、营销物料(光斑/插画);BrandMark 渐变是唯一允许的渐变。
- **Don't** 让局部集合冒充全局结论:任何汇总结论必须标注统计范围,零匹配用中性「暂无数据」,永不显示「全部正常」。
