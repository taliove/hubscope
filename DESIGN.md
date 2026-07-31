---
# 摘要索引面(非穷举)——normative 唯一来源是正文,见正文首段声明(2026-07-30 裁决,check LOW-4;v2 重写沿置)。
name: HubScope
description: 对内 LLM 端点监控与质量评估——状态概览答「健不健康」,Benchmark 答「哪个模型好」
colors:
  primary: "#007aff"
  primary-hover: "#0a84ff"
  primary-active: "#0062cc"
  primary-soft: "#eaf3ff"
  success: "#34c759"
  success-text: "#1f7a33"
  warning: "#ff9500"
  warning-text: "#b35c00"
  danger: "#ff3b30"
  danger-text: "#d70015"
  info: "#6e6e73"
  page-bg: "#f5f5f7"
  surface: "#ffffff"
  surface-hover: "#e8e8ed"
  text-primary: "#1d1d1f"
  text-regular: "#3a3a3c"
  text-secondary: "#86868b"
  text-placeholder: "#a1a1a6"
  border: "#d2d2d7"
  border-light: "#e5e5ea"
  overlay: "rgba(0,0,0,0.3)"
typography:
  hero:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'PingFang SC', 'Microsoft YaHei', sans-serif"
    fontSize: "72px"
    fontWeight: 600
    lineHeight: 1.0
  headline:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'PingFang SC', 'Microsoft YaHei', sans-serif"
    fontSize: "32px"
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
  mono:
    fontFamily: "ui-monospace, 'SF Mono', 'Cascadia Mono', Consolas, monospace"
    # fontSize intentionally null: mono is sized per usage — Wordmark uses an
    # em base (the host scales it via font-size), the version stamp uses the
    # xs tier; no single tier value applies.
    fontSize: null
    fontWeight: 400
    lineHeight: 1.5
rounded:
  xs: "2px"
  sm: "6px"
  lg: "12px"
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
  space-9: "96px"
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
    padding: "24px"
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

> **本文件的 normative 结构(2026-07-30 裁决,check LOW-4;v2 重写沿置):** frontmatter 是 impeccable 工具链消费的**摘要索引面,非穷举**——`*-soft` 功能浅底、z-index 刻度、动效令牌等只在正文登记,不在 frontmatter。**normative 唯一来源是正文**:正文登记的令牌与规则即生效,不以是否入 frontmatter 为条件。
> **v2 重写声明(2026-08-01,GH #122,spec 0018 / ADR 0015):** 本文件由建成世界重写——旧「信号墙」体系(teal 电波青、四态、闪烁独占)正式作废,历史仅存于 git。值与机制以 `web/src/styles/` 三层实现为准(ground truth over intention);spec 0018 是决策档案,ADR 0015 是三处承重语义推翻记录。

## Overview

**Creative North Star: "AI 基础设施操作系统 (The AI Infrastructure OS)"**

HubScope 的界面像 Apple 管理一套复杂 AI 系统:打开后 5 秒回答四个问题——整体是否健康、哪些模型有风险、异常影响范围、下一步该处理什么。读者分三种——3 秒看屏的状态板读者、高密度作业的管理台读者、隔着一条分享链接的外部接收者——界面对这三种人都必须诚实:结论永远标注统计范围,没有数据就断线,失败不冒充空态。

审美语调:**冷静、专业、可信、极简、精准**。参考坐标 Apple System Settings / Apple Health / Linear / Vercel / Stripe。大留白、轻容器、局部状态强调;情绪由三态语义色承担,不由装饰承担——不是 Grafana,是操作系统。

**Key Characteristics:**
- 三层令牌架构(原始刻度 → 语义令牌 → Element Plus 映射),暗色键位预留(暗色后置,独立 spec 定稿)
- 轻容器语法:白面 + 1px 描边 + radius-lg + 无阴影;阴影只表达「可点/浮层」
- 220px macOS 设置风侧边栏外壳,签名面自造(侧边栏/Hero/指标区/列表行/详情面板/时间线),Element Plus 保留管理台复杂件
- 状态三态:稳定 / 性能下降 / 服务异常,小圆点 + 状态词双编码,局部强调而非大面积色块
- 动效成体系(v2.0 §15):数字补间、图表入场绘制、页面 Fade+Slight Move、hover 上浮;reduced-motion 全局归零

## Colors

调色板 = 「一族蓝 + 一族零彩度中性灰 + 三功能色文字/图形双阶」:品牌蓝承担交互,中性灰承担全部中性表达,功能三色承担三态语义。**不存在第四色**——告警橙随三态合并整体退役(ADR 0015);`--hs-status-failing` 令牌值为 danger 红,仅为混合期兼容保留,新世界零消费,待物理删除。

### Primary
- **品牌蓝 Blue-600**(#007AFF):主色锚点。主按钮、链接、当前导航、聚焦态、「运行中」批次状态。白底实测 4.02:1,不过 WCAG AA 4.5——用户裁决按简报原文执行(已登记代价,spec 0018);缓解 = 品牌色文字用法从简、关键文本用墨色。
- **hover Blue-500**(#0A84FF)/ **active Blue-700**(#0062CC):hover 提亮、按下加深;blue-700 白底 5.80:1 过 AA,兼 BrandMark 渐变深 stop。blue-500 同时是 Apple 暗色蓝,为暗色 spec 预留(亮主题不消费)。
- **品牌浅底 Blue-50**(#EAF3FF):选中行、高亮块、品牌区衬底;榜单行 hover 填充(--hs-brand-soft)。

### Neutral(Apple 零彩度中性灰 10 步,三锚点)
- **Gray-900**(#1D1D1F):主文字锚点(白底 16.83:1)。
- **Gray-800**(#3A3A3C):常规正文(11.35:1)。
- **Gray-500**(#86868B):次文字锚点(3.62:1,简报值,与品牌色同类登记);次要说明、标签、辅助。
- **Gray-400**(#A1A1A6):占位/禁用(2.57:1,非正文用途)。
- **Gray-50/100/200**(#F5F5F7 / #E8E8ED / #D2D2D7):页面底锚点、hover/轨道、描边——表面三层(page↔card↔hover 两两可分辨)。**hairline(border-light)= #E5E5EA**:介于 page 与 hover 之间的发丝线,白卡上可辨不抢;卡内分隔一律用 hairline,不用 loud border。
- **Gray-600**(#6E6E73):info 中性(5.07:1 过 AA)。

### Functional(三态语义,文字/图形双阶分工)
- **图形阶(本体)**:Success #34C759 / Warning #FF9500 / Danger #FF3B30——状态圆点、分段格、榜单条形、soft 浅底、徽章底色。绿/黄本体白底实测 2.22/2.20:1,不过 3:1 图形门槛(简报原文值,已登记代价,ADR 0015);红本体 3.55:1 过图形门槛。
- **文字阶(`--hs-*-text`)**:Success-Text #1F7A33(5.40:1)/ Warning-Text #B35C00(4.72:1)/ Danger-Text #D70015(5.38:1)——**一切文字场景**:状态词、档色分数、涨跌箭头、可用率数字、警示行。实测均过 AA 4.5。
- **Info**(#6E6E73,gray-600):中性提示。
- **soft 浅底**:功能色混白 90%(EP light-9 等价):选中态、横幅底、高亮填充。

### 浮层衬底
- **`--hs-overlay-bg`** = rgba(0,0,0,0.3):全站唯一 scrim,EP overlay 经 ep-theme.css 统一映射,不存第二套衬底语言。

### Named Rules

**The One Voice Rule.** 品牌蓝在任何单屏上是稀缺资源:主按钮一处、当前导航一处、聚焦态若干。它不渲染大背景、不染卡片、不作标题色——它的稀有性就是它的信号强度。

**The Double-Encoding Rule.** 状态永远色 + 词双编码同场:小圆点承担色,文字承担词。任何只用颜色表达状态的呈现都是缺陷(静态导出物料用着色文字转译,不复制状态灯)。

**The Graphic-Text Division Rule.** 功能色的文字场景一律消费 `*-text` 阶,图形填充消费本体阶——本体阶是简报锚点值(不过 AA),文字阶是实测过 AA 的加深阶。两阶各司其职,禁止跨通道借用。

**The 80/15/5 Rule.** 色彩比例原则:中性灰承担约 80%(页面底、文字、描边),品牌蓝约 15%(导航、链接、聚焦),三态语义色约 5%(状态点、状态词、局部强调)——异常是局部强调,不是大面积色块;页面保持冷静。

## Typography

**Body Font:** 系统字栈(system-ui / PingFang SC / Microsoft YaHei 回退;Apple 设备自然解析为 SF Pro / PingFang SC),零外部字体文件——与单二进制交付一致。
**Mono Font:** 系统等宽字栈(`ui-monospace, "SF Mono", "Cascadia Mono", Consolas, monospace`)——品牌字族之一:Wordmark 字标(字重 700)与版本号;零外部字体文件纪律同上。本登记即该字栈的设计系统出处,detector 的 design-system-font 检查以本条为准。

**Character:** 无字体个性的刻意选择——字形权威来自层级与字重,不来自字体。字重只用 400/600(700 仅限 Wordmark);行高默认 1.5,数字类 1.2;大数字 `tabular-nums`。

### Hierarchy(v2.0 §14)
- **Hero**(600, 72px, 1.0,`--hs-text-hero`):核心大数字专用——健康指数、物料可用率大数字。一页至多一处,永远是「最值得记住的那个数字」;禁用范围同旧 display 锚点纪律。
- **Page Title**(600, 32px,`--hs-text-3xl`):页面标题(v2.0 §14);物料标题同档。
- **Module Title**(600, 20px,`--hs-text-xl`):模块标题、卡片/分组标题、指标区次级大数字(480 紧凑版物料锚点)。
- **Large**(400/600, 16px,`--hs-text-lg`):强调正文、分区小标题。
- **Body**(400, 14px,`--hs-text-md`):正文基准、表单、表格主列。
- **Body-Small**(400, 13px,`--hs-text-sm`):次要正文、状态词。
- **Label**(400, 12px,`--hs-text-xs`):辅助/标签/时间戳。
- **2xl**(600, 24px,`--hs-text-2xl`):刻度中间档——Hero 百分比单位、详情页关键数字、480 紧凑版物料标题。
- **Display**(28px,`--hs-text-display`):**legacy 退役档**——唯一存量消费方 LoginView Wordmark;新世界零新增消费,消费清零后删除刻度(GH #110 登记)。

### Named Rules

**The Hero Anchor Rule.** hero 档是 3 秒读者的第一视觉锚点(健康指数 72px),一页至多一处,且永远是「最值得记住的那个数字」。把它用在标题或正文上,等于拆掉锚点。(旧 Display Anchor Rule 的 v2 沿置,锚点档由 28px 升为 72px。)

## Layout

**外壳(决策 5/8):** 全站 220px macOS 设置风侧边栏(AppSidebar,自造签名面)+ 右侧内容区;`/login` 与 `/report/:token` 走 route `meta.bare` 壳外渲染(无侧边栏)。页面切换 = Fade + ≤10px 轻位移(300ms slow 档,见 Motion)。

内容区 max-width 1200px 居中,桌面优先——不为手机做专门适配,窄屏不阻断即可。4px 基准网格,间距消费 space-1..9 刻度(space-9 96px 为 hero 区块与页面级呼吸预留);**公开页 32–48px 页面留白**(状态概览 padding 32px,spec 0018 IA),管理台页 24/16。

**轻容器语法(Apple 语法,全站容器唯一形态):** 白面(`--hs-bg-card`)+ 1px `--hs-border` 描边 + `radius-lg`(12px)+ **无阴影**;内边距桌面 24/32px(space-5/6),窄屏收紧 16px(space-4,768px 断点,Leaderboard 先例)。静态容器永不吃阴影;hover 上浮 + md 阴影只属可点元素。

**密度按读者分档:** 消费页(状态概览、Benchmark、分享报告)轻容器宽松档;管理台(模型管理/系统设置/评估中心运营区)EP 复杂件 + 紧凑档(`--el-card-padding` 12px 既定机制,禁 `:deep(.el-card__body)` 覆写)——档位由读者决定,不由登录态决定。

页面结构:页面标题(h1 = 侧边栏标签,见 ui-guidelines §4)→ 内容区(轻容器或列表)→ 必要时底部辅助区。弹性列宽优先于固定宽度;卡片内容不得溢出、不得出现横向滚动条;管理台表格的弹性内容列必须显式 min-width,使全表最小宽度可算术核对。

**响应式断点(GH #91 批登记,v2 沿置):** 全站唯一断点 **768px**(`(max-width: 767px)` 生效),消费方封闭清单 = 分享弹窗(StatusShareDialog/EvalShareDialog)、Leaderboard、EvalProgressGrid、/report/:token 页;**窄屏处置原则 = 形态切换,不是横向滚动豁免**——弹窗物料等比缩放、榜单矩阵降级卡片式列表、轻容器内边距收紧;断点判定统一 matchMedia 组件级实现,禁各处自造第二套断点逻辑。

### Named Rules

**The Density-By-Reader Rule.** 密度档位看读者不看登录态:3 秒场景给呼吸,作业场景给密度。同一张卡片不因「在哪」而变密度,只因「给谁」而变。

## Elevation & Depth

Flat-by-default。深度由 1px 描边与表面色阶(page → card → hover 三层)承担;阴影不是装饰,是交互语义——它只回答一个问题:「这东西能点吗 / 这东西浮在页面上吗」。

### Shadow Vocabulary(亮主题基准;暗色覆盖值待暗色 spec 填,键位预留)
- **sm**(`0 1px 2px rgba(0,0,0,0.04)`):最轻浮起,少用。
- **md**(`0 4px 16px rgba(0,0,0,0.07)`):可点元素 hover 反馈(与 2–4px 上浮配对)。
- **lg**(`0 12px 32px rgba(0,0,0,0.12)`):浮层——弹窗、详情面板、下拉。

### Named Rules

**The Flat-By-Default Rule.** 静态容器无阴影 + 1px 描边。阴影出现当且仅当:hover 的可点元素,或浮层。静态元素吃阴影 = 谎报可点性。

## Shapes

圆角四档,按元素层级分配:

- **xs(2px)**:分段条/时间条类填充元素(24h 微条格),仅限此类。
- **sm(6px)**:控件默认——按钮、输入框、tag、评分徽标、侧边栏导航项。
- **lg(12px)**:面板层级——轻容器、卡片、弹窗、详情面板。
- **full(999px)**:胶囊元素(计数 chip),按需。

形态语言是「连续圆角的方」:Apple 语法的 6/12 双档近似连续圆角;无大圆角色块、无胶囊按钮、无异形裁剪。分段条是唯一的填满式时间形态:24 格等分填满轨道,2px 间距,格即一小时。

### Named Rules

**The Radius-By-Layer Rule.** 圆角跟着层级走,不跟着喜好走:控件 6、面板 12、时间格 2、胶囊 999。新元素先问自己属于哪一层,再取圆角。

## Motion

动效成体系(v2.0 §15,决策 4):**禁闪烁**(「闪烁 = failing 独占」语义整体退役,ADR 0015),动效全部服务于「变化被注意到但不惊悚」。

### Motion Vocabulary
- **数字补间(500–800ms):** 仅健康指数 hero 大数字与指标区核心数字;实现 = `utils/numberTween.ts`(600ms 中值,easeOutCubic,终值精确发射),reduced-motion 立即落终值。
- **图表入场绘制(800–1200ms):** 一次性从左向右绘制;实现 = `utils/chartMotion.ts`(1000ms 中值,matchMedia 一次性判断门控);30 天桶(5761 点)超 ECharts animationThreshold 2000 时自动关闭(数据密度本身不需要绘制动画,已登记)。
- **页面切换(Fade + Slight Move):** Fade + ≤10px 位移,200–300ms;实现 = App.vue `page` transition(300ms slow 档 `--hs-transition-slow`,enter 8px 位移、leave 纯 fade);query 变化不重挂载。
- **hover 上浮(2–4px):** 可点元素 `translateY(-2px)` + `--hs-shadow-md`;**表格/矩阵行豁免**(GH #118 裁决:hairline 分隔的行上浮会破坏共享 grid 基线与仪式竖条几何,行可点性由 hover 填充 `--hs-brand-soft` + 指针承担)。
- **局部刷新:** 轮询(10s)只更新变化区域,组件级局部渲染,不整页重载;visibilityPoll 封装与降频纪律不变(ui-guidelines §6)。
- **状态点呼吸(未落地,登记):** spec §15 的「状态变化瞬间呼吸」尚无实现(StatusBadge 注释登记为后续动效票);状态变化当前走 `--hs-transition` 颜色过渡。登记为已知差距,不补写规则书。
- **批次旋转图标:** AppSidebar 批次入口 Loading 图标 1s 线性旋转(组件级 reduced-motion 门控)——全站唯一常驻动画,工具性进度指示,非状态语义。

### Transition Scale(tokens.css)
- **default**(`--hs-transition`,0.2s cubic-bezier(0.4, 0, 0.2, 1)):反馈档——hover/focus、披露容器、状态色过渡、弹层入场。
- **slow**(`--hs-transition-slow`,0.3s 同缓动):页面切换档(v2 §15 200–300ms 窗口)。

### Named Rules

**The Gate-The-Phases Rule.** reduced-motion 下 CSS 过渡由 semantics.css 全局 `transition: none !important` 归零兜底,但 JS 阶段时序(补间、图表绘制、旋转等 animation/rAF 驱动)必须单独门控:matchMedia 一次性判断(chartMotion.ts 模式),reduced-motion 时延迟归零、终态直呈——延迟不是装饰,是等待。

**The No-Blink Rule.** 全站无任何闪烁动画。告警紧急度的显示层表达 = danger 双编码(红点 + 「服务异常」词)+ 物料「含 N 个告警」事件 chip;域模型 failing 档与 Lark 告警管线不动(W5)。(取代旧 Blink-Is-Failing Rule,ADR 0015。)

## Components

组件体系的通则;页面级构成规格在各 surface brief。**组件库混合(决策 8):** Element Plus 保留管理台复杂件(表格/弹窗/表单/分页);签名面自造——侧边栏、Hero、指标区、模型列表行、详情面板、时间线。

### Buttons
- **Shape:** 控件圆角(6px),默认高 32px。
- **Primary:** 品牌蓝底白字;hover blue-500,按下 blue-700。
- **Text / link 型:** 无色底,文字品牌蓝;用于列表行操作与图标按钮(分享入口)。
- **反馈:** 点击类操作请求期间给 loading 或禁用态,不静默等待。
- **焦点单语言(全站唯一焦点样式):** `:focus-visible` = 2px brand outline(自造面;AppSidebar 导航项、列表行 `outline-offset` 按构图取 -2/+1)——EP 组件焦点态由 ep-theme.css 统一映射;不引入第二种焦点样式。(旧 1px inset ring 语言随旧世界退役,GH #112 起自造面统一为 outline。)

### Cards / Containers
- **Corner Style:** 面板圆角(12px)。
- **轻容器语法:** 白面 + 1px 描边 + 无阴影(见 Layout);内边距桌面 24/32、窄屏 16。
- **Shadow Strategy:** Flat-by-default——静态容器永无阴影;可点元素 hover 上浮 + md 阴影(表格行豁免,见 Motion)。

### Inputs / Fields
- **Style:** 1px 描边、控件圆角、卡面底;聚焦品牌蓝。
- **管理台限宽:** 长输入(URL/webhook)560px,标准输入 320px,数字/开关/下拉用组件默认宽——表单控件不拉满整卡(ticket 102 登记,v2 平移,细则见 ui-guidelines §4)。

### Tags
- **Style:** 控件圆角、size small、effect light;语义色走 `type` 属性,不自着色。
- **用途纪律:** 角色 tag(primary/info)、协议 tag(success/warning/info)、判定方式 tag(info)、告警事件 tag(按 kind 映射)各有集中映射函数;同一语义域内词表封闭(ui-guidelines §5/§7)。

### Navigation(侧边栏外壳)
- **Style:** 220px 自造侧边栏:顶部品牌块(BrandMark 24px + Wordmark lg,点击回首页)→ 导航项(线性图标 16px + 文字,radius-sm)→ 批次进度入口(登录态 + 存在未完成批次时)→ 底部(账号行:用户名 + 角色纯文本 + 退出;未登录为「管理登录」链接;版权行;版本号 mono xs)。
- **激活态:** 轻量高亮——hover/激活均 `--hs-bg-hover` 浅底,激活项文字与图标品牌蓝;无强背景块(spec 0018 user story 2)。
- **登录态过滤:** 未登录只见公开项(状态概览、Benchmark);过滤逻辑集中 `utils/sidebarNav.ts`(visibleSidebarItems / isSidebarItemActive 纯函数)。

### Signature: StatusBadge(唯一状态灯)
- 全站唯一的 endpoint 状态展示组件:**小圆点(图形阶)+ 状态词(文字阶)** 双编码,三态(稳定/性能下降/服务异常);词与色槽全部来自显示层映射 `utils/statusDisplay.ts`,组件内禁写状态词字面量;degraded 可挂成因副标签(「· 可用性」「· 延迟」,secondary,不随词着色);零动画(闪烁退役)。规格与消费纪律见 ui-guidelines §5 与 overview surface brief。

### Signature: 24h 微条(UptimeMicroStrip)
- 24 格填满式时间条(格高 14px、xs 圆角、2px 间距),格 = 一小时;三档着色(≥95% 绿 / <95% 黄 / 0% 红 / 无数据 border 灰),tier 与 tooltip 措辞集中 `utils/overviewDots.ts`(与分享物料同源,一致性由构造保证);聚合口径 = 按小时求和,禁止按端点简单平均。模型列表行内唯一时间形态;物料 24h 分段条保持独立实现(快照渲染契约)。

### Signature: 自造模态面三件套
- 自造弹层(详情面板首例,重建的分享弹窗复用)必须三件套齐备:① **focus trap**(`utils/focusTrap.ts`——Tab/Shift+Tab 在面内循环,el-dialog 等价);② **统一关闭路径**(ESC / scrim 点击 / 关闭按钮走同一 emit);③ **焦点归还**(关闭后焦点回触发元素,父级承担)。任何 aria-modal 自造面缺一件 = 缺陷。

### Signature: BrandMark / Wordmark
- **BrandMark 是唯一图形标:** 瞄准镜字形(圆环 + 十字准星刻度 + 中心脉冲点,监控隐喻),蓝渐变(blue-400 #549CFF → blue-700 #0062CC 两 stop,表现属性兜底纪律沿置——snapdom 对 SVG stop 不内联计算样式,`stop-color` 表现属性与 `var()` 令牌链双写,tokens.css 改色两处同步);em 尺寸宿主可控。**与 ProxyHub 的家族同源关系解除**(ADR 0015):渐变随品牌色重建,不再共享色板。
- **Wordmark 是唯一字标:** 「HubScope」PascalCase + 品牌蓝脉冲小圆点(常亮),系统等宽字栈,字重 700,完全静止;em 基准宿主缩放。
- BrandMark 永不裸用,永远与 Wordmark 同场出现(AppSidebar 品牌块、LoginView 品牌区、物料品牌区)。

## 暗色(后置登记,spec 0018 决策 10)

暗色主题不由本版定义;令牌三层保留双值能力,**暗色键位预留清单**(暗色 spec 的填值起点,GH #110 登记):① semantics.css `html.dark` 块全键(值暂沿亮值,`@media screen` 作用域——导出物料恒亮);② chartColors.ts `CHART_COLORS_DARK` 全字段(暂镜像 LIGHT,`useChartColors` 恒供 LIGHT、ComputedRef 签名保留);③ tokens.css 阴影暗色覆盖值(html.dark 块三个阴影键位待暗色 spec 加回);④ index.html 防闪脚本恢复路径(v1 主题开关 utils/theme.ts 已随 AppHeader 退役,暗色 spec 时重建)。暗色 spec 落地时**只改 semantics/css 镜像层,页面零改动**——页面只消费语义令牌是硬约束。

## Do's and Don'ts

### Do:
- **Do** 只消费语义令牌(`--hs-*` 语义层);调具体色值改 tokens.css,调暗色观感(暗色 spec 后)改 semantics.css 的 html.dark 块。
- **Do** 让严重度驱动组织:最紧急的信息最先被看见,视觉权重 = 业务严重度。
- **Do** 用轻容器与表面色阶分层,静态元素保持 flat;文字场景消费 `*-text` 阶。
- **Do** 状态点 + 词双编码;长文本截断 + hover 全显;空/加载/错误三态齐备。
- **Do** 导出物料恒亮主题渲染,离屏捕获用 `position: absolute; left: -10000px`。

### Don't:
- **Don't** 硬编码色值、书写 `--el-*`(唯一允许处 ep-theme.css;`--el-card-padding` 密度档机制除外)、使用刻度外 z-index/间距/圆角字面量。
- **Don't** 引入调色板外新色相——三态世界没有第四色,告警橙已整体退役。
- **Don't** 引入任何闪烁动画;动效只走 Motion 节登记的词汇表。
- **Don't** 用渐变装饰、大面积彩色背景块、营销物料(光斑/插画);BrandMark 渐变是唯一允许的渐变。
- **Don't** 让局部集合冒充全局结论:任何汇总结论必须标注统计范围,零匹配用中性「暂无数据」,永不显示「全部正常/全部稳定」。
