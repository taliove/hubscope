---
name: HubScope
description: 对内 LLM 端点监控与质量评估——状态板答「健不健康」,评估榜单答「哪个模型好」
colors:
  primary: "#0c8078"
  primary-hover: "#0faea2"
  primary-active: "#0a6963"
  primary-soft: "#effcfa"
  success: "#059669"
  warning: "#a16207"
  danger: "#dc2626"
  info: "#45565c"
  failing: "#c2410c"
  page-bg: "#f7fafb"
  surface: "#ffffff"
  surface-hover: "#eff4f5"
  text-primary: "#0f1b20"
  text-regular: "#324249"
  text-secondary: "#617379"
  text-placeholder: "#91a3a8"
  border: "#e0e8ea"
  border-light: "#eff4f5"
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
- **电波青 Teal-600**(#0c8078):亮主题主色,白底对比度 4.8:1 过 WCAG AA。主按钮、链接、当前导航、聚焦态、「运行中」状态。暗主题主色提亮为 teal-500(#0faea2)——电波带(teal-400/500)在白底对比度不足,只作暗色主色与图形点缀,永不作白底文字。
- **电波青浅底 Teal-50**(#effcfa):品牌浅底,选中行/高亮块/品牌区衬底;暗主题为 teal-900(#063f3d)。

### Neutral
- **青灰 Gray-900**(#0f1b20):主文字,低彩度同族青灰,非纯黑。暗主题 #e2e8f0。
- **青灰 Gray-700**(#324249):常规正文。暗主题 #c3c7cd。
- **青灰 Gray-500**(#617379):次要说明、标签、辅助;暗主题 #8a8f98,并兼任暗色 info。
- **青灰 Gray-400**(#91a3a8):占位/禁用/等待中。暗主题 #5c616a。
- **青灰 Gray-50/100/200**(#f7fafb / #eff4f5 / #e0e8ea):页面底、悬浮/轨道、描边。暗主题表面三层为 #0f1115 / #17191d / #1f2227,描边 #2a2d33 / #23262b。

### Functional(亮暗基本同值,warning 例外分档,承担状态语义)
- **Success**(#059669):正常 / 已完成 / 分数升。
- **Warning**(#a16207，亮主题;暗主题保留 #d97706):降级 / 可逆警示。亮值 2026-07-29 修订(appendix B #14):旧值 #d97706 白底实测仅 3.19:1 不过 AA,改 yellow-700(4.92:1)且色相比旧值更远离 failing 橙。
- **Danger**(#dc2626):宕机 / 失败 / 分数降 / 不可逆操作。
- **Info**(亮 #45565c / 暗 #617379):中性提示,与青灰同族。

### The Failing Exception
- **告警橙 Orange-700**(#c2410c,亮主题;暗主题 orange-400 #fb923c):failing 告警专属,「调色板外不引入新色相」纪律的唯一具名例外——告警辨识度 = 告警可信度。白底 5.1:1 过 AA;与 warning 差一个色相族、与 danger 明度接近而色相向橙偏移,在黄红之间建立第三紧急档。永不泛化为装饰色,永不用于批次/运行状态。

### Named Rules

**The One Voice Rule.** 品牌电波青在任何单屏上是稀缺资源:主按钮一处、当前导航一处、聚焦态若干。它不渲染大背景、不染卡片、不作标题色——它的稀有性就是它的信号强度。

**The Double-Encoding Rule.** 状态永远色 + 词双编码同场:圆点/图标承担色,文字承担词。任何只用颜色表达状态的呈现都是缺陷(静态导出物料用着色文字转译,不复制状态灯)。

**The Blink-Is-Failing Rule.** 全站唯一动画是 failing 告警的 hs-blink 闪烁。运行状态、字标、装饰元素一律静止——「有东西在闪」的直觉解读必须是「有情况」。本条管**持续/循环/环境动效**;用户触发的单次状态过渡(披露、hover、聚焦)走 `--hs-transition`,不受本条限制(2026-07-29 /impeccable animate 评审澄清)。

## Typography

**Body Font:** 系统字栈(system-ui / PingFang SC / Microsoft YaHei 回退),零外部字体文件——与单二进制交付一致。
**Mono Font:** 系统等宽字栈(ui-monospace / SF Mono / Consolas),仅用于 Wordmark 字标(字重 700)。

**Character:** 无字体个性的刻意选择——工具产品的字形权威来自层级与字重,不来自字体。字重只用 400/600/700(700 仅限 Wordmark);行高默认 1.5,数字类 1.2。

### Hierarchy
- **Display**(600, 28px, 1.2):消费页主视觉大数字专用——健康横幅大字结论、可用率大数字。禁用于管理台标题与正文。
- **Headline**(600, 24px, 1.2):品牌区标题(「HubScope 服务状态」)。
- **Title**(600, 20px, 1.2):页面标题、关键数字(平均延迟)。
- **Subtitle**(600, 16px, 1.5):卡片/分组标题。
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
- **雾化(backdrop-filter blur 8px)是速览弹窗专属例外**:blur 承载「卡片墙还在后面」的连续性隐喻,与翻转编舞同源;管理台弹窗是作业工具,不雾化。blur 值为实现常量,不进令牌刻度。

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

动效是克制的:全站持续动画只有一例(failing 闪烁,Blink-Is-Failing Rule),其余动效全部是用户触发的单次过渡,且只有两档速度。

### Transition Scale(tokens.css)
- **default**(`--hs-transition`,0.2s cubic-bezier(0.4, 0, 0.2, 1)):一切默认过渡——hover/focus 反馈、披露容器、状态切换。
- **focal**(`--hs-transition-focal`,0.32s cubic-bezier(0.16, 1, 0.3, 1)):焦点入场专用——用户触发的「主体进入视野中心」单次过渡(首例:速览弹窗翻入)。不用于 hover、披露、装饰。

编舞延迟(stagger/delay/搭接)不进刻度:是单次编舞的常量,集中在编舞实现处定义并注释互指(首例:速览弹窗 140ms stagger、perspective 1600px)。

focal 档第二形态登记(2026-07-30,速览弹窗 morph 批):**共享元素 morph**——用户点名的主体从原位连续变形进入视野中心(位移 + 缩放 + 翻转同拍,首例:速览弹窗从被点卡片矩形 morph 进场)。morph 是 focal 档的合法负载,不新增速度档;几何(dx/dy/scale)走纯函数,锚点连续性(起点 = 触发元素矩形、终点 = 恒等终态)优先于路径直线性;动态变换经 CSS 变量注入,fallback 恒为原地入场(reduced-motion 由全局归零 + JS 门控覆盖,不新增分支)。

### Named Rules

**The Two-Speed Rule.** 全站只有两档过渡速度:0.2s 给反馈,0.32s 给焦点入场。不引入第三档;拿不准就用 default——focal 只颁给「用户点名要看的东西正在登场」。

**The Gate-The-Phases Rule.** reduced-motion 下 CSS 过渡由全局归零兜底,但 JS 阶段时序(setTimeout/rAF 编舞延迟)必须单独门控:reduced-motion 时全部延迟归零、终态直呈——延迟不是装饰,是等待。

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
