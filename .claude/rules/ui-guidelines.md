# UI Guidelines(业务语义手册)

> **本文件 = 前端业务语义手册(GH #47 四层映射定稿,2026-07-29):** 只承载业务语义——两套状态词表、降级成因副词表、语义色映射关系(引用 DESIGN.md 令牌名)、防作假约定、分享面信息边界、成本中性化。视觉与布局权威在 [DESIGN.md](../../DESIGN.md)(令牌/字阶/圆角/阴影/布局/暗色/品牌);页面与组件构成规格在 surface briefs(`web/.impeccable/surfaces/`,状态板三页已建,其余页面随批次迁入);产品形态与读者模型在 [PRODUCT.md](../../PRODUCT.md)。三处均由 `plan` agent 维护,`check` agent 按多源验收。
> **语义映射(状态色、词表)变更仍属承重语义变更,须在设计评审中说明理由。** 标注「保留备查,不再更新」的章节(§1/§2/§2a/§2b/§4 与 §5 部分条目)是迁移档案,内容以各自落点为准。
> 视觉基线(ticket 73)并入 ProxyHub「现代极简工具风」(电波青 teal 品牌色、三层令牌、暗色一等公民);HubScope 词表、状态语义、防作假约定全部保留,仅做色值与刻度映射。

## 1. 产品形态与读者

> **已迁 `PRODUCT.md`(GH #48)**——产品形态、读者模型(含 2026-07-29 确认的第三受众「外部接收者」)、桌面优先、工具风基调以 PRODUCT.md 为准;本节保留备查,不再更新。

- **双形态:** 公开侧(状态板 Dashboard、EndpointDetail + 公开榜单页 /board,无需登录,spec 0010)+ 管理台(EvalCenter /eval、TaskCenter、Admin,需登录)。
- **两类读者:** 状态板读者要「3 秒看懂健不健康」——状态优先,操作入口让位;管理台读者要「高效完成配置与排查」——信息密度优先,操作直达。
- **桌面优先:** 内容区 `max-width: 1200px` 居中(Dashboard 先例),不为手机做专门适配,窄屏不阻断使用即可。
- **全站工具风:** 并入 ProxyHub「现代极简工具风」审美——灰阶为主、用色克制、轻阴影靠描边分层、无多余装饰(无渐变装饰、无大圆角、无彩色背景块)。公开页(状态板、登录页)同为工具风基调,不做营销装饰(不引入光斑背景、插画等营销物料);BrandMark 渐变是唯一允许的渐变(图形标识,非装饰)。

## 2. 视觉基线

> **已迁 `DESIGN.md`(GH #49)**——令牌三层架构、字阶、圆角、阴影、间距、z-index、过渡的视觉规格以 DESIGN.md(frontmatter 为 normative)为准;本节保留备查,不再更新。

- **Element Plus 组件体系 + 三层令牌架构,浅色/暗色双主题一等公民。** 令牌分三层,位于 `web/src/styles/`:

  ```
  tokens.css      原始刻度(主题无关的"尺",只放两主题同值的刻度)
  semantics.css   语义令牌(页面与组件只消费这一层;暗色 = html.dark 只覆盖本层)
  ep-theme.css    Element Plus 映射(全仓唯一允许书写 --el-* 变量的文件,
                  亮暗两块用 color-mix 从语义令牌生成 EP 派生阶)
  ```

  修改纪律:调具体色值改 tokens.css;调暗色观感改 semantics.css 的 `html.dark` 块;调 EP 组件观感改 ep-theme.css;**页面/组件只消费 semantics.css 的语义令牌,禁止直接引用原始刻度、禁止写 `--el-*`、禁止硬编码色值**。唯二豁免:① `--el-card-padding` 密度档设置(§2 间距条目的既定机制);② BrandMark 消费原始刻度与刻度外细节(§2b,图形标识非语义表达:渐变 stop、字形白色描线 `#fff`)。引入顺序(main.ts):`element-plus/dist/index.css` → `element-plus/theme-chalk/dark/css-vars.css` → tokens.css → semantics.css → ep-theme.css → print.css。ep-theme.css 必须含 `html.dark` 块(EP dark/css-vars.css 的 `html.dark` 选择器特异性高于 `:root`,需在其后加载并重复声明全部映射,亮主题 light-N 混白/dark-2 混黑 20%,暗主题反转)。

- **命名前缀:** 沿用 `--hs-`(HubScope)前缀,**不**改 `--ph-`——降低存量 diff 与 Review 风险,避免多 Hub 产品生态混读;语义与层级完全对齐 ProxyHub 架构,仅前缀不同。

- **原始刻度(tokens.css,主题无关):**

| 类别 | 刻度 | 值 |
|---|---|---|
| 品牌电波青 9 级 | `--hs-teal-50..900` | `#effcfa` / `#d1f6f0` / `#a3ece1` / `#68dccf` / `#30c4b8` / `#0faea2` / `#0c8078` / `#0a6963` / `#085350` / `#063f3d` |
| 中性青灰 10 级 | `--hs-gray-50..900` | `#f7fafb` / `#eff4f5` / `#e0e8ea` / `#c7d3d6` / `#91a3a8` / `#617379` / `#45565c` / `#324249` / `#1e2b31` / `#0f1b20` |
| 功能色基色 | `--hs-success-base` / `--hs-warning-base` / `--hs-danger-base` / `--hs-info-base` | `#059669` / `#d97706` / `#dc2626` / `#45565c` |
| failing 橙刻度(独立例外,见 §3) | `--hs-orange-500` / `--hs-orange-700` / `--hs-orange-400` | `#f97316` / `#c2410c` / `#fb923c` |
| 间距 4px 基网 | `--hs-space-1..8` | 4 / 8 / 12 / 16 / 24 / 32 / 48 / 64 |
| 圆角 | `--hs-radius-xs/sm/lg/full` | 2 / 4 / 8 / 999(§2 圆角节) |
| 阴影(亮主题基准,gray-900 基底) | `--hs-shadow-sm/md/lg` | `0 1px 2px rgba(15,27,32,.05)` / `0 2px 8px rgba(15,27,32,.08)` / `0 8px 24px rgba(15,27,32,.12)` |
| 字号 | 见 §2 字阶节 | — |

- **语义令牌(semantics.css,亮/暗双值;页面唯一消费层):**

| 语义令牌 | 亮主题 | 暗主题 | 用途 |
|---|---|---|---|
| `--hs-brand`(主色) | teal-600 `#0c8078`(白底对比度 4.8:1,过 WCAG AA) | teal-500 `#0faea2` | 主按钮、链接、当前导航、聚焦态、运行中状态 |
| `--hs-brand-hover` | teal-500 `#0faea2` | teal-400 `#30c4b8` | hover 阶 |
| `--hs-brand-active` | teal-700 `#0a6963` | teal-600 `#0c8078` | 按下态 |
| `--hs-brand-soft` | teal-50 `#effcfa` | teal-900 `#063f3d` | 品牌浅底:选中行、高亮块、BrandMark 旁衬底 |
| `--hs-success` | `#059669` | 同值(暗底 3.5:1,状态灯+词双编码可用;文字场景降级用 dark-2 阶) | 正常/已完成/升 |
| `--hs-warning` | `#d97706` | 同值 | 降级/可逆警示 |
| `--hs-danger` | `#dc2626` | 同值 | 宕机/失败/降/不可逆操作 |
| `--hs-info` | `#45565c`(gray-600) | gray-500 `#617379`(暗底提一档保可读) | 中性提示 |
| `--hs-status-failing` | orange-700 `#c2410c`(白底 5.1:1 过 AA;§3 裁决) | orange-400 `#fb923c`(暗底 ≈7:1) | 告警,唯一动画状态 |
| `--hs-success-soft` / `--hs-warning-soft` / `--hs-danger-soft` / `--hs-info-soft` | 功能色混白 90%(EP light-9 等价) | 功能色混暗底 82% | 功能浅底:横幅底、选中态、高亮填充 |
| `--hs-bg-page` | gray-50 `#f7fafb` | `#0f1115` | 页面全局背景、hero panel 中性浅底 |
| `--hs-bg-card`(表面) | `#ffffff` | `#17191d` | 卡片/表格/弹窗表面 |
| `--hs-bg-hover` | gray-100 `#eff4f5` | `#1f2227` | 行/菜单悬浮;榜单条形轨道 |
| `--hs-text-primary` | gray-900 `#0f1b20` | `#e2e8f0` | 标题、正文主文字 |
| `--hs-text-regular` | gray-700 `#324249` | `#c3c7cd` | 常规正文 |
| `--hs-text-secondary` | gray-500 `#617379` | `#8a8f98` | 次要说明、标签、辅助 |
| `--hs-text-placeholder` | gray-400 `#91a3a8` | `#5c616a` | 占位/禁用、等待中状态 |
| `--hs-border` | gray-200 `#e0e8ea` | `#2a2d33` | 卡片边、输入框边 |
| `--hs-border-light` | gray-100 `#eff4f5` | `#23262b` | 浅分割、hairline |
| `--hs-shadow-sm/md/lg`(暗色覆盖 tokens 同名刻度) | 见 tokens | `0 1px 2px rgba(0,0,0,.3)` / `0 2px 12px rgba(0,0,0,.4)` / `0 6px 24px rgba(0,0,0,.5)` | 阴影 |

- **字阶七档(display 为消费页大数字专用,非标题不用):**

| 令牌 | 值 | 用途 |
|---|---|---|
| `--hs-text-xs` | 12px | 辅助/标签/时间戳 |
| `--hs-text-sm` | 13px | 次要正文、StatusBadge |
| `--hs-text-md` | 14px | 正文基准、表单、表格主列 |
| `--hs-text-lg` | 16px | 卡片/分组标题 |
| `--hs-text-xl` | 20px | 页面标题、关键数字(如 StatusCard 平均延迟) |
| `--hs-text-2xl` | 24px | 品牌区标题(「HubScope 服务状态」) |
| `--hs-text-display` | 28px | 消费页主视觉大数字:HealthBanner 大字结论、StatusCard 24h 可用率大数字 |

  说明:并入前 HubScope 六档(xs..2xl)平移保留;新增 display 档对齐 ProxyHub display-sm(28px),把「健康横幅大字结论」与「StatusCard 可用率大数字」从 2xl 24px 升档——两者都是消费页读者「3 秒场景」下的第一视觉锚点,工具风层级靠字号/字重表达,大数字应承担锚点职责;display 档禁用于管理台标题与普通文本。字重只用 400/600/700(700 仅限 Wordmark 等宽字标);行高默认 1.5,数字类 1.2。

- **圆角四档(撤销原 `--hs-radius` 6px 默认档):**

| 令牌 | 值 | 用途 |
|---|---|---|
| `--hs-radius-xs` | 2px | 分段条/时间条类填充元素(24h 可用率条格),仅限此类元素,存续 |
| `--hs-radius-sm` | 4px | 控件默认(按钮/输入框/tag/评分徽标),覆盖 `--el-border-radius-base` |
| `--hs-radius-lg` | 8px | 卡片、面板、弹窗(el-card/el-dialog/el-message-box) |
| `--hs-radius-full` | 999px | 胶囊形元素(计数 chip 等),按需 |

  迁移映射:原卡片/弹窗 6px → 8px(面板层级);原按钮/输入框 6px → 4px(控件层级,对齐高密度工具基调);原 tag/徽标 4px 不变;分段条 2px 不变。EP 侧由 ep-theme.css 统一映射(`--el-border-radius-base: var(--hs-radius-sm)`,卡片圆角在 ep-theme.css 内以 EP 类选择器微调),页面不写圆角字面量。

- **阴影语义:** 只表达「可点/浮层」,不用于装饰——静态卡片靠 1px 描边分层,用 `shadow="never"` + 边框;可点卡片 hover `--hs-shadow-md`;浮层(弹窗/抽屉/下拉)`--hs-shadow-lg`。原 `--hs-shadow-card`(静态轻阴影)撤销——工具风「轻阴影靠描边分层」,静态卡片不吃阴影。

- **间距:** 4px 基准网格不变,消费 `--hs-space-1..8` 令牌(ticket 73 落地刻度,存量 px 字面量按后续批次渐进迁移,新代码一律用令牌);卡片内边距统一走 `--el-card-padding` 变量,**密度档位按读者类型划分**:消费页(状态板、评估榜单 /eval、分享报告页)16px,管理台(/admin 全部 tab,含评估运营与题库)紧凑档 12px——档位由读者决定,不由登录态决定;禁止 `:deep(.el-card__body)` 覆写;区块间距 16px,页面上下 24px;内容区 `max-width: 1200px` 居中。导出物料(StatusCard 等独立画布组件)不受 el-card 16/12px 密度档约束,内边距按物料设计定(StatusCard 为 40px 横向 / 32px 纵向),区块间距沿用 4px 基准网格。

- **z-index 刻度:** 对齐 ProxyHub 八档——`--hs-z-sticky` 100 / `--hs-z-fixed` 500 / `--hs-z-dropdown` 1000 / `--hs-z-overlay` 2000 / `--hs-z-drawer` 2010 / `--hs-z-dialog` 2020 / `--hs-z-message` 3000 / `--hs-z-tooltip` 4000;页面代码不允许出现刻度外 z-index 字面量(EP 内部 z-index 由其自管理,不在此约束)。

- **过渡:** 统一 `--hs-transition: 0.2s cubic-bezier(0.4, 0, 0.2, 1)`(主题无关,归语义层暴露);hs-blink 告警闪烁动画除外(见 §3)。

## 2a. 暗色主题

> **已迁 `DESIGN.md`(GH #49)**——暗色机制(语义层覆盖、切换入口、持久化、导出恒亮、ECharts 双镜像)以 DESIGN.md 为准;本节保留备查,不再更新。

- **暗色一等公民:** 暗色不是反色滤镜,表面/文本/描边/阴影/主色均有独立暗色取值(§2 语义令牌表);暗色只覆盖 semantics.css 的 `html.dark` 块,页面代码零改动——页面只消费语义令牌是硬约束。
- **切换入口:** AppHeader 右栏操作区(批次进度入口左侧)放亮/暗切换按钮(link 型图标按钮,Sun/Moon 图标),未登录态同样可用(状态板读者无差别)。当前主题下只显示目标态图标(亮主题显示 Moon)。
- **持久化与防闪:** 选择存 `localStorage` 键 `hs:dark`(1/0);`index.html` 内联首屏防闪脚本在挂载前同步读该键加/去 `html.dark` class(键名两处保持一致,注释互指)。**默认亮主题、不跟随系统偏好**(v1 裁决:二态切换语义最简单,公开状态板常被投屏/截图,主题确定性优先;系统偏好跟随留作后续增强,届时改三态「跟随系统/亮/暗」需设计评审登记)。
- **导出物料固定亮主题:** StatusCard(PNG/PDF)、分享报告页导出一律强制亮主题渲染——物料是对外传播的静态快照,亮主题保证打印/转发可读性;实现上导出画布渲染前临时去 `html.dark` class 或在离屏容器内以亮主题令牌渲染,禁止把暗色像素烤进物料。
- **离屏捕获定位约定(snapdom 修复登记):** 导出物料的离屏捕获双份(EvalShareDialog / StatusShareDialog 的 `.capture-source` 及 `withLightCapture` 临时 wrapper)一律 `position: absolute; left: -10000px`,**禁用 `position: fixed`**——snapdom 对 fixed 元素按视口宽度重排舞台,卡片被拉宽、flex 轨道重伸展而 % 宽度已冻结 px 的子元素不动,导致条形/分数/涨跌列错位;absolute 离屏不触发该重排,舞台宽度与预览一致。
- **ECharts 暗色:** JS 镜像色板亮暗双份(见 §3 ECharts 条目),按当前主题取份;主题切换时图表重渲染(watch 主题状态,setOption 全量替换)。
- **暗色验收:** 暗色下功能色文字场景(表格内着色文字、明细行状态词)需抽查对比度;failing 闪烁动画暗色下不调整(闪烁是语义,非装饰)。

## 2b. 品牌标识(BrandMark / Wordmark)

> **已迁 `DESIGN.md`(GH #49)**——品牌标识视觉规格以 DESIGN.md 为准;本节保留备查,不再更新。

- **BrandMark 是唯一图形标:** 共享组件 `components/BrandMark.vue`,64×64 viewBox 内联 SVG——圆角 rect(rx=14)填 teal-400→teal-700 渐变(渐变 stop 消费 `--hs-teal-*` 原始刻度,此为页面消费原始刻度的唯一豁免:图形标识非语义表达),白色**瞄准镜字形**(圆环 + 十字准星刻度 + 中心脉冲点——监控隐喻:盯住 Hub 上每个 endpoint;ticket 73 后续修订,取代初版与 ProxyHub 同构的 hub 辐条字形,因用户反馈「不合群」——与 ProxyHub 的区分由图形标承担,不再只靠字标);em 尺寸宿主可控(`width/height: 1em`)。**捕获兜底属性(snapdom 修复登记):** `<stop>` 元素同时携带 `stop-color` 表现属性(`#30c4b8` / `#0a6963`,镜像 tokens.css teal-400/teal-700)——snapdom 对 SVG 内部元素(stop/defs/linearGradient 等)从不内联计算样式,仅写在 scoped CSS 的 `var(--hs-teal-*)` 会在克隆中丢失、落回默认黑;表现属性是导出物料保真的兜底,**不是第二色值来源**——浏览器内 CSS 优先级高于表现属性,`var()` 令牌链路仍是权威;tokens.css 改色时两处必须同步(BrandMark.vue 注释与本条互指)。favicon 与 AppHeader 左侧品牌位同源;`web/public/logo.png` 已删除,AppHeader/LoginView/StatusCard 一律使用 BrandMark。
- **Wordmark 是唯一字标:** 共享组件 `components/Wordmark.vue`——「HubScope」PascalCase 文本 + 主色**脉冲小圆点**(常亮,与 BrandMark 瞄准镜中心点同源的图形呼应),字体用**系统等宽字栈**(`ui-monospace, "SF Mono", "Cascadia Mono", Consolas, monospace`),不引入任何外部字体文件(零依赖、离线可用,与单二进制交付一致),字重 700;字号以 em 为基准,宿主用 `font-size` 覆盖等比缩放。使用点:AppHeader 品牌位(BrandMark + Wordmark 横排,默认 `--hs-text-lg` 16px)、LoginView 品牌区(`--hs-text-display` 28px)、StatusCard 品牌区(随物料画布定字号,不用令牌字面量)。**字标完全静止(2026-07-24 修订,取代初版闪烁终端光标):「闪烁=failing 告警专属」是 W5 承重语义,字标作为全站唯一持续运动元素挂在每页左上角,用户对「有东西在闪」的直觉解读就是「有情况」——豁免条款救得了规则救不了直觉;脉冲点常亮后,failing 重新独占全站唯一动画,无任何豁免。**
- **LoginView 品牌区构成:** BrandMark(40px)+ Wordmark(display 档)横排居中,替代原 logo.png 40px 图块;登录卡保持工具风,不加装饰背景。

## 3. 语义色映射(核心约定)

颜色承载业务语义,**映射关系只在本文件定义**,组件引用同一映射,禁止各组件自造颜色语义:

| 业务语义 | 颜色 | 说明 |
|---|---|---|
| healthy 正常 | success(`--hs-success`) | endpoint 状态 |
| degraded 降级 | warning(`--hs-warning`) | endpoint 状态 |
| down 宕机 | danger(`--hs-danger`) | endpoint 状态 |
| failing 告警 | failing(`--hs-status-failing`,亮 orange-700 / 暗 orange-400)+ 闪烁 | 比 down 更紧急,唯一允许动画的状态(见 StatusBadge) |
| 评分/百分比档位 | 绿/黄/红阈值 | 阈值以后端口径为准,前端不自定分界线 |

- **failing 色选档裁决(本版登记):** failing 语义是「比宕机更紧急、需立即处理」,旧版橙红(历史值 #FF4500)的「最刺眼」地位必须保留。ProxyHub functional 四色中 warning(amber-600 族)与 danger(red-600 族)之间没有足够的同系加深空间——amber-700(历史候选值 #b45309)与 warning 同族,并排时(HealthBanner、状态板状态列、图例)仅明度差一档,降级/告警相邻场景辨识度不足,且「更深的黄」直觉上并不比「更红的红」更紧急。故选 **orange 系**:亮主题 orange-700(`--hs-status-failing` 亮值,白底对比度 ≈5.1:1,过 WCAG AA,可承载文字;与 warning 差一个色相族、与 danger 明度饱和度接近而色相向橙偏移,在红黄之间建立「第三紧急档」),暗主题 orange-400(`--hs-status-failing` 暗值,暗底对比度 ≈7:1,暗色下功能色惯例提亮)。**这是「调色板外不引入新色相」纪律的唯一例外,在本条登记**:告警是 HubScope 监控域的最高紧急度语义,amber 加深无法满足「与 warning/danger 三档可分辨」的业务硬需求,例外收益(告警可辨识度=告警可信度,W5 语义)大于纪律成本;除本例外外禁止任何新色相。具体色值以 DESIGN.md frontmatter 为准。
- ECharts 系列色从上述调色板取,正文/轴文字用 `--hs-text-primary`/`--hs-text-secondary` 等值(图表内走 JS 镜像 const,**亮暗双份**:`CHART_COLORS_LIGHT` / `CHART_COLORS_DARK`,与 semantics.css 双值逐一同步,按当前主题取份;主题切换时图表 setOption 重渲染),不引入调色板外的新色相(failing 橙为例外登记,同入镜像)。两份镜像现状(TimeSeriesChart/TrendChart 各一份)在迁移期合并为单一来源(如 `utils/chartColors.ts`),两个图表组件复用。
- 同一语义在状态板与管理台必须同色同词。
- **分数涨跌指示:** 升=success 绿、降=danger 红,与「分数高=好」同向;持平、无上批次、**跨 Suite 版本断点(分数不可比)一律不显示涨跌箭头**,用 `--hs-text-placeholder` 占位并标注「题目已变更」(忠实 ADR 0007:禁止把题库变化呈现为模型降级,这是本产品的防作假核心语义)。
- **榜单条形(0–100):** 按评分档位阈值着色(绿/黄/红,阈值以后端口径为准),与得分徽标同一映射,不为榜单另定分界线;涨跌箭头是独立维度,不替代条形档位色。档色同时承担维度格子与总分条的强弱表达(spec 0009)。
- **静态导出物料(PNG/PDF)中的语义表达(批 56 约定):** 动画不可用时,failing 闪烁转译为双编码——`--hs-status-failing` 橙(亮主题固定取值,物料恒亮主题)实心圆点/文字 + 「含 N 个告警」文字标注(橙描边 chip),禁止用 danger 红替代橙,禁止静默省略 failing 维度。blink 动画在静态导出会定格在半透明帧,故静态物料明细行的状态词用**语义色文字着色**表达(告警=橙、宕机=danger、降级=warning),不使用 StatusBadge 圆点徽章——这是「禁止第二个状态灯实现」对静态物料的例外,仅限导出画布,页面内仍一律复用 StatusBadge。静态物料无 hover,长文本截断长度按完整可读性折中,不依赖 `title` 全显。
- **批次/运行状态色映射(ticket 52 登记,与 endpoint 状态映射并列,词表与语义不混用):**

| 业务语义 | 颜色 | 说明 |
|---|---|---|
| 运行中 | brand teal(亮 `--hs-brand` teal-600 / 暗 teal-500) | 进行中强调,禁闪烁;禁用 warning 黄(黄=降级专属) |
| 等待中 | 中性灰 `--hs-text-placeholder` | 未开始 |
| 已完成 | success 绿 | 与 endpoint 域同向(好) |
| 失败 | danger 红 | 与 endpoint 域同向(坏) |

  绿/红在两语义域同向复用;闪烁动画仍为 failing 告警专属,运行状态不得使用。
- **24h 可用率三档着色(批 59 登记,状态板既定口径,源自 EndpointCard 分段条):** 单小时格与 24h 聚合可用率共用同一阈值——≥95% success 绿、<95% warning 黄、0%(有探测且全失败)danger 红、无探测数据 `--hs-border` 灰;该阈值同时适用于分段条单元格、StatusCard 可用率大数字与明细行可用率列,不为任何单一场景另定分界线。聚合口径=按小时对齐求和 total/failures(与后端探测加权一致),禁止按端点简单平均。阈值是评估业务口径,并入后不动,仅映射色值。

## 4. 布局规范

> **已迁 `DESIGN.md`(GH #49)**——布局与密度档(1200px、16/12px、表单限宽)以 DESIGN.md 为准;本节保留备查,不再更新。

- 页面结构:顶部标题/操作区 → 内容区(卡片或表格)→ 必要时底部辅助区。
- 内容容器首选 `el-card`;列表数据首选 `el-table`;管理页多功能分区用 `el-tabs`(**不加 `lazy`**,保轮询事件,ticket 19)。
- 详情抽屉/弹窗:`el-dialog` 用于需要聚焦的详情与表单(见 EvalRunDetailDialog),不在页面内嵌套多层展开区。
- 卡片内容**不得溢出、不得出现横向滚动条**;弹性列宽优先于固定宽度(历史 bug:24h 小点)。管理台 `el-table` 的弹性内容列(无固定 width 的列)必须显式声明 `min-width`(先例:AuditLogs 详情列 `min-width="220"`),使全表最小宽度可算术核对,杜绝「默认 min-width 兜底、溢出与否靠运气」。
- **管理台表单控件限宽档(ticket 102 设计评审登记,2026-07-28):** 管理台(12px 密度档)表单输入控件禁止 100% 拉满整卡,按内容长度两档限宽——**长输入档 560px**(URL/webhook/token 类,如飞书 Webhook);**标准输入档 320px**(标识/短文本,如裁判模型名);`el-input-number` / `el-switch` / `el-select` 用组件默认宽度,不另限。输入框 + 行内操作按钮(如「发送测试」)必须同行:走 el-form-item content 的默认 flex,输入框 `flex: 0 1 auto` + 限宽、按钮 `flex: none`,仅容器宽度不足时才允许折行。label 宽度按本表单最长标签定档:≤4 字沿用 90px 既有惯例,≤6 字用 120px,不为单个长标签无度拉宽。多值紧凑排布(如评估集权重)走 `flex-wrap` 横排,标签 + 控件成组、间距消费 `--hs-space-*` 令牌,禁止每值独占一行竖排堆叠。公开消费页表单(登录页等)已有自布局,不受此限。

## 5. 组件使用规范

- **Element Plus 组件优先**,不引入新 UI 库,不自造表单、弹窗、表格、分页。
- **StatusBadge 是唯一的状态展示组件**【规格已迁 surface brief(GH #50),保留备查不再更新】,需要展示 endpoint 状态处一律复用,禁止第二个状态灯实现。
- **StatusBadge 降级成因副标签**【已迁 surface brief(GH #50)】(ticket #7 登记,spec 0013):degraded 状态可挂结构化成因副标签——可选 prop `causes?: DegradeCause[]`,非空且 status 为 degraded 时在状态词后行内渲染「· 可用性」「· 延迟」,双命中「· 可用性 + 延迟」(顺序固定:可用性在前,与后端 `degrade_causes` 字段顺序一致)。副标签是 Badge 文字的一部分:**无独立圆点/图标/底色/动画,不是第二个状态灯**(唯一状态灯纪律不破);字号与 Badge 同档 `--hs-text-sm`,颜色 `--hs-text-secondary`(状态词主、成因辅的层级),状态色仍由圆点承载(§3 映射不动,不引入新色相)。分隔符固定「· 」(前缀)与「 + 」(双成因连接);成因词映射集中 `utils/degradeCauses.ts` 纯函数(与 `role.ts` 集中原则同例),组件内禁止写词字面量。防御规则:causes 非空但 status ≠ degraded 时副标签不渲染;未传 causes 的消费方渲染不变;聚合场景(Dashboard 汇总条、分组头部计数 chips、HealthBanner)永不传 causes——成因是端点粒度信息,聚合层不下钻(spec 0013 范围外声明)。
- **LatencySparkline 是 EndpointCard 延迟曲线的唯一组件**【已迁 surface brief(GH #50)】(ticket #8 登记,spec 0013):24h 分段条下方与 dots 行同构的曲线行,按小时桶 P50 绘制(桶 p50 仅统计成功探测,失败探测超时值不污染曲线;全失败桶分段条显红、曲线断线,两轴视觉自洽)。形态:**纯 SVG polyline,不引 ECharts**(卡片矩阵每卡一实例过重;纯 SVG 对未来物料导出零风险);行高 28px,**每卡恒渲染**——完全无数据时同高占位(1px `--hs-border` 灰轨道,tooltip 各区「无数据」),卡片高度全矩阵一致。**x 轴与 24h 分段条构造性对齐(硬约束):** ① 两行标签统一固定宽 26px、flex:none;② SVG 经 ResizeObserver 实测 strip 像素宽渲染,桶中心 x 由几何纯函数按 flex+gap 公式计算(slot=(W−23×GAP)/24),与 dots strip 布局公式同源——**`GAP=2px` 是 dots strip CSS 与 sparkline 几何的唯一共享常量,改动必须同步**;禁止固定比例 viewBox + preserveAspectRatio="none"(槽缝比随宽度变化,数学上不可对齐)。曲线规范:stroke `--hs-text-secondary` 1.5px round,无逐点圆点,**孤立单点段渲染 r=1.5 圆点**(单点 polyline 不可见的边界兜底);null 桶断线分段(与 TrendChart 同纪律);**曲线下方面积浅填充 `--hs-bg-hover` 实心**(2026-07-28 修订新增:功能性面积强调,非装饰;选表面令牌而非文字令牌叠透明度——令牌类别正确、亮暗双值构造性成立、零新色相;填充随段断,孤立点不填充);**曲线中性色不承载状态语义**,状态由卡片左边条与 StatusBadge 承担。**量程(2026-07-28 修订,附录 B 第 11 项):** 数据量程驱动——`yMax = max(峰值 × 1.25, MIN_Y_RANGE_MS=1000ms)`,曲线形态满幅可见;1s 下限防极端小值把亚秒抖动放大成伪形态(不夸大的诚实表达),亚秒端点曲线停于量程下带是「快」的正确读法;原「y 轴上界 max(阈值, 桶 P50 max)×1.1」条款由本修订取代——阈值恒在导致量程被阈值锚定、曲线贴底蠕动、毛刺/爬升形态不可见(用户实机反馈,截图证据:P50 2.22s vs 阈值 ~7s),1.1 余量随 0 锚定量程一并退役。降级阈值虚线 = 2× 7天 P50 基线:`--hs-warning` 1px `dasharray 4 3` 全宽水平线;**按需出现(2026-07-28 修订,取代恒在)——出现 ⟺ 阈值 ≤ yMax(构造性条件,等价峰值 ≥ 阈值×0.8,零魔幻常数;不出现时零残余指示,阈值/基线数值由 hover tooltip 恒兜底;出现/消失随数据滑窗跳动不加迟滞,与 dots 颜色跳变同属数据驱动表达)**;基线 null(样本<5)不画线,tooltip 基线段同省略。hover 走 strip 顶层 24 列透明 overlay(与 dots strip 同一 flex 构造)分列 el-tooltip:「HH:mm 时段 · P50 X · 基线 Y」;**p50 为 null 的桶按桶事实二分措辞(2026-07-28 裁决,取代 ticket #8「空桶/全失败桶同词」登记):** 桶无探测(total=0)→「HH:mm 时段 · 无数据」;桶有探测但全失败 →「HH:mm 时段 · 探测全部失败,无延迟样本」——tooltip 是精确读数层,全失败桶有探测数据,与空桶同词「无数据」会和分段条红格 tooltip「成功 0/N」互证断裂;该规则统一覆盖全失败卡片占位轨道情形与混合卡片单桶情形,**无整卡级特判**;全失败卡片的占位轨道保持灰色占位不变:不加色、不文字化——曲线行不承载状态语义,失败语义由分段条与 StatusBadge 承担。纯展示不可点,下钻走整卡点击;几何计算抽 `utils/latencySparkline.ts` 纯函数(vitest 覆盖断线分段/阈值 y/全空输入/单点段),组件只渲染。零硬编码色值、零 `--el-*`、暗色随语义令牌生效。
- **HealthBanner 是 Dashboard 的全局健康横幅组件**【已迁 surface brief(GH #50)】(批 2 登记):四态(全部正常/N 个端点降级/N 个端点异常含告警闪烁/加载 skeleton),数据只反映全局、永不受页面过滤器影响;仅异常态可点(应用状态过滤并滚动定位)。其他页面不得复刻其结论文案模式。大字结论用 `--hs-text-display` 档(§2 字阶)。
- **Leaderboard 是评估榜单的唯一排行展示组件**(/eval 与 /report/{token} 复用;ticket 78 矩阵化重构,spec 0009):每模型一行——`名次 │ 模型(+family tag)│ 总分 │ 涨跌 │ 各维度列`,全表 CSS grid 定宽(名次 32 / 模型 260 / 总分 96 / 涨跌 80,维度列 `minmax(0,1fr)` 等分剩余宽,**列宽与权重无关**——权重只影响总分,后端口径不动),列头与行共用同一 grid 模板,**每列 x 位置全表恒定**(对齐是矩阵的构造属性,取代 flex + 变宽 family tag 的漂移);维度格子由共享组件 ScoreCell 渲染(见下条),模型名截断 + `title` hover 全显;**排序走点列头**(总分与维度列头可点,降序,当前列带 ↓ 指示;再点当前列回总分降序——服务端仅降序语义,禁止前端 reverse 制造第二排序口径;列头 role/tabindex/Enter/Space 键盘可达,与行下钻对齐),family 过滤走榜单上方工具条,不进单元格;行下钻不内嵌行内展开,走 `el-dialog`(§4 既有约定,EvalRunDetailDialog 模式);工具条收敛为 family 筛选 + baselineNote + 分享按钮(维度切换 radio 与排序 select 已废——矩阵下所有维度分同屏常显,「聚焦 + 变灰」无意义);**涨跌列常显**(总分列常显,涨跌贴着总分永不误导;行级口径不变:▲绿 ▼红,持平/不可比/无基准 `–`);**表格化精致(ticket 82,spec 0010):** 列头下 1px `--hs-border` hairline(padding-bottom `--hs-space-2`)+ 行间 1px `--hs-border-light` hairline(行间距由 hairline 承担,不再用裸 gap)+ 行高恒 46px(落地定稿:15 行一屏的算术最优,区间 44–48 内);行 hover/focus 填满行盒(表格语义,无负 margin 溢出——hairline 与列头线严格对齐);**前 3 名仪式感:** 行左缘 3px `--hs-brand` 竖条(描边语言,**不用背景色块**;非前 3 名行保留透明竖条位,列 x 位置恒定)+ 名次数字 `--hs-text-lg`/600 brand teal,其余 secondary;live 模式(rank `–`)不渲染竖条。
- **Leaderboard 运行中半成品模式**(ticket 52 登记,ticket 78 随矩阵化重构修订 ②④⑤,GH #40 修订 ①②④):未完成批次下榜单可查看但——① **(GH #40 修订,取代名次恒 `–` 占位)** 半成品总分非 null 的行显示**实时名次**(行序位次数字,弱化样式:`--hs-text-sm` + `--hs-text-secondary`,**禁前 3 名竖条/大字/徽章**——`isTopRank` live 恒 false 的既有构造保持);半成品总分 null(尚无任何维度判分)的行名次列仍 `–` 占位(`--hs-text-placeholder`);② **(GH #40 修订,取代固定字典序)** 行序按**半成品总分降序**(ADR-0005 既有口径:已判维度加权平均,未判维度不进分子分母——禁止改为「含 null 维度按 0 计」的第二口径),同分 model_id 字典序,null 总分沉底(组内字典序,前端不重排,后端保证);**列头仍禁点**(无指针/hover/↓ 指示,摘除 role/tabindex——实时榜只有总分降序一个排序口径,禁列头切换制造第二口径),保留 family 过滤;轮询引起的行重排不做动画(数据驱动,与条款⑥同源);③ 涨跌箭头列整列隐藏(不占位);④ 已判分维度格子照常(档色数字 + 细条 + 水印);未判分格子 **(GH #40 修订,取代裸 `–`)** value 槽显**批次状态词**「进行中/等待中/失败」(`--hs-text-xs`,按 §3 批次/运行状态色映射:进行中 brand、等待中 placeholder、失败 danger;**纯文字禁圆点禁闪烁**——圆点 + 词是进度网格的形态,分数视图用文字避免第二状态灯观感)+ 空轨道,tooltip 不变(「能力点名 · 等待中/进行中/失败」批次词表)——维度实时进度由此在分数视图内联可见;**边缘措辞(澄清登记):run 完成(done)但全部判分失败、score 仍为 null 的格子,状态词显「未判分」**——cellStatusText 中性回落(`--hs-text-placeholder`),「未判分」是中性兜底词、**非第五状态词**,不借批次词表、不挂状态色,tooltip 同口径「能力点名 · 未判分」;⑤ 总分列墨色数字 + **空轨道不染档色、不填充**——半成品总分(如只判完 2/5 维度的 40 分)构造上不可能被档色读作「差」(spec 0004 半成品边界在视觉层的镜像);行尾(末列之后)灰字注「N 个维度进行中」(`--hs-text-xs` + `--hs-text-placeholder`),有失败 run 追加「· N 个失败」(danger 色)——标注不占列宽、不进总分;⑥ settle 后名次弱化样式直接升级为正式名次样式(前 3 名竖条 + 大字生效),不做强调动画,转场提示由 ElMessage 承载。
- **Leaderboard 判分不完整模式**(ticket 92 登记,spec 0014 决策 A;与「运行中半成品模式」并列且互斥——live 行构造性无 `complete` 键,settle 行无 live-note):settle(done/failed)批次中 `complete=false` 的行(判分不完整:批次覆盖且当前有启用 case 的 suite 中有 `missing_suites` 个未判上分;门槛只收排名资格,不收维度分,W7 口径不动)——
  - ① **名次列 `–` 占位**(`--hs-text-placeholder`,形态复用 live 占位);rank-top 竖条与前三大字按 **`row.complete !== false && index < 3`** 判定——禁纯 index 判定(完整模型不足 3 个时 index<3 命中的是不完整行,服务端沉底只保证「完整组优先」不保证「完整组 ≥3」)。
  - ② **总分列 `–` + 空轨道不染档色不填充**(服务端 total_score=null,`formatScore` 既有 null 口径与 `total-fill` 的 null 守卫天然覆盖;与 live 半成品总分同精神——无总分即无档色,`–` 永不被读作「差」)。
  - ③ **涨跌列 `–` 占位**(total_delta=null 既有逻辑覆盖);tooltip 改「**判分不完整,不参与排名与涨跌**」——取代通用「批次 #N 无该模型分数」(不完整行有维度分,仅无总分,通用文案不准确)。
  - ④ **维度格子照常渲染真实分数**(档色数字 + 细条 + 覆盖率水印不变)——不掩盖已判分事实,行内可见分数是水印 N/M 口径的对证。
  - ⑤ **水印挂模型列、模型名之下第二行**(2026-07-28 设计评审修订,取代票面「总分列 dash 之下」;裁决见附录 B 第 12 项):文案定稿「**判分不完整,缺 N/M 维度**」——N = `missing_suites`(契约字段,未判上分的 suite 数),M = missing_suites + 该行非 null suite_scores 数(行级自洽推导:门槛分母无现成字段,该口径与行内可见分数恒一致;「判完后题库清空」边缘把已清空但有分的维度计入 M,与该维度分数在行内展示的可见事实一致,且恒有 N ≤ M);「**缺**」字必须保留——裸「N/M」与 ScoreCell 覆盖率水印「·8/10」(分子=已判数)的既有分子口径冲突,会误读为「已判 N/M」。规格 `--hs-text-xs` + `--hs-text-secondary` + font-weight 400 + opacity 0.85(对齐 ScoreCell 覆盖率水印弱化规格),单行优先、物料窄列允许自然换行(white-space: normal);**禁截断 + title 兜底**(静态物料无 hover,截断即信息丢失)。模型列 260px(页面)/150px(物料)单行可容(典型文案 ≈124px),水印行 + 模型名行 ≈39px,不破 ticket 82「行高恒 46px」不变式。
  - ⑥ **两模式可区分性(票风险 1 裁决):** live 半成品与 settle 不完整共享名次 `–` 形态,区分由三重构造性差异承担——live 总分是半成品墨色数字 + 行尾动态注「N 个维度进行中」(批次词表),settle 不完整总分是 `–` + 静态水印判定结论(判定词表);live 涨跌列整列隐藏,settle 涨跌列常显;二者永不同时出现(live 行无 complete 键,settle 行无 live-note)。同形态不构成误读,登记为允许复用占位形态的依据。**(GH #40 后附记)** live 半成品名次升级为弱化数字后,两模式名次形态亦不再共享占位(弱化数字 vs `–`),可区分性进一步增强,原三重差异登记不撤——null 总分的 live 行仍 `–`,此时区分回落到原三重差异。
  - ⑦ **排序:** `complete=false` 恒沉底(与排序列无关,总分或任一 suite 列同),不完整组内 model_id 字典序;/board 客户端 `sortRows` 镜像同一口径(`complete === false` 视为不完整,`undefined`(live/旧后端)视为完整——与后端 `rankable()` 同口径;不完整组内字典序显式 tiebreak,不依赖数组原序或 sort 稳定性),禁第二排序口径(vitest 覆盖:不完整沉底 × total/各 suite 排序键 × 组内字典序 × 全不完整)。
  - ⑧ **全部模型不完整时榜单照常渲染**(行保留、全体 dash),禁空态文案冒充「暂无上榜模型」;live 模式呈现零变化(所有新分支以 `row.complete === false` 为条件,live 行构造性不触发,回归由既有测试 + vitest 镜像承担)。
  - ⑨ **EvalCard 静态物料同步:** EvalCardRow 增 `complete`/`missingSuites`,`rank` 改 `number | null`(不完整行 null → 名次 `–`;rank-top-rail 按 `rank !== null && rank <= 3` 判定);水印同款同位置(staticMode 无 tooltip,水印必须在物料内完整可见——150px 模型列单行可容,更宽 N/M 允许自然换行);总分 `–` + 空轨道、涨跌 `–` 与页面同构。
  - ⑩ **暗色:** 占位 dash 与水印均消费 `--hs-text-placeholder`/`--hs-text-secondary` 语义令牌,亮暗双值构造性成立;无新增色、无调色板外操作,不另登记。
- **ScoreCell 是榜单维度格子的唯一共享组件**(ticket 78 登记,spec 0009;Leaderboard 行与 ticket 79 EvalCard 静态模式双端消费——共享接缝从「堆叠条」换成「格子」,双端一致性守住):`formatScore` 数字(`--hs-text-md`/600,**档色着色**)+ 其下 4px 档色细条(轨道 `--hs-bg-hover`,`--hs-radius-xs`)。语义:
  - **轨道中性化(ticket 82 定稿,spec 0010):** 细条与总分条轨道一律 `--hs-bg-hover`——轨道是「卡面之上一步的中性填充」,bg-hover 正是该语义(暗色下恰为卡面最小可见阶跃);`--hs-border-light` 是 hairline 分隔语义,不作填充(不留第二处解释);档色(绿/黄/红)是全榜唯一跳色元素。
  - **条刻度恒 0–100(W7 绝对分制的视觉镜像):** 87.5 的条永远比 100 短一截,不做行内/列内归一化,跨行跨批次可比;档色按 §3 分数档位(≥80 success / ≥50 warning / <50 danger,阈值口径不动),档色同时承担强弱表达(五色分类色已整体退役,见本条末废止登记);null(未判分)→ `–` 占位(`--hs-text-placeholder`)+ 空轨道。
  - **覆盖率水印(防作假硬约束,ticket 75 条款语义沿用):** done 且 `judged < expected` 的格子分数后随压缩水印「·8/10」(同色弱化:font-weight 400 + opacity 0.85),**宽度自适应可见——格子宽度 ≥80px(阈值 ticket 79 定稿:md/600 数字「100.0」约 38px + 水印「·10/10」约 34px + 余量)才渲染,不够则省略,完整置信信息由 hover tooltip 恒兜底**「能力点名 · 分数 · 判分 X/Y 题 · 采样 N 次 · 耗时 X · Token Y」(ticket 51 confidence 口径 + GH #42 成本段——与进度网格 cell tooltip 同口径同源字段,成本段中性不着色,规格见「评估成本指标呈现约定」条);未判分格子 tooltip 为「能力点名 · 等待中/进行中/失败」(批次词表)。静态物料(EvalCard)无 hover:水印按页面同一宽度规则渲染(staticMode 只测一次宽度、不观察;物料维度列约 56–68px,低于阈值,水印在物料自然不渲染),tooltip 信息不进物料,视为物料读者的已知信息差。
  - props 接缝(ticket 78 设计评审定稿):`{ name, score, cell, staticMode? }`——传拆好字段而非整 row(EvalCard 快照构造复用时不受 ReportRow 形状耦合);`name` 供 tooltip 能力点名,`staticMode` 为静态物料模式(无 tooltip、宽度测一次不观察)。
  - 总分列是 ScoreCell 的同族变体而非复用:`--hs-text-xl`(20px)/600 **墨色不染档色** + 6px 档色条(比维度 4px 粗一档,0–100 同刻度),层级靠字号与条粗不靠颜色;live 下总分空轨道不染档色不填充(见「Leaderboard 运行中半成品模式」条)。
  - **superseded 登记:** ticket 75 堆叠条条目(段宽归一口径「条长 ≡ 总分」、段内数值 44px 暂定阈值、维度切换 ~40% 高亮联动、原 props 接缝)由本条目取代——段宽随分数变使固定标尺对齐成为结构性不可能(SuiteRuler 只对第一行准确),段宽亦不承担强弱表达;覆盖率水印「宽度自适应 + tooltip 兜底」与基准不可比三分支、「不可比一律不显涨跌箭头」口径保留不变(§3)。ticket 77(五色分类色 `--hs-suite-1..6` + SuiteLegend/SuiteRuler)**整条废止**:① 对齐结构性不可能(family tag 变宽致 bar 左缘漂移);② 强弱不可见(分类色只表达「哪个维度」不表达「好不好」);③ 功能色撞车(suite-3=danger 红 / suite-4=warning 橙 / suite-5=success 绿,满分读作「危险/警告」)。能力点名字从三处(radio/图例/标尺)收敛为一处:**列头**。
- **EvalProgressGrid 是批次进度矩阵的唯一组件**(ticket 52 登记):模型 × 能力点状态矩阵,运行中/等待中批次的默认视图;首列模型名截断 + `title` hover,能力点列 flex 等宽,行随模型数纵向滚动,**不开横向滚动豁免**(§4 无特例);单元格四态用 §3 批次/运行状态色映射,纯展示不可点;批次级进度汇总置于网格上方同卡片内;「进度网格 / 实时分数」视图切换走卡片顶部 el-radio-group。**分享页只读模式(ticket 54 登记):** /report/{token} 运行中批次复用本组件,但不渲染视图切换(组件须支持隐藏切换的只读用法),网格是分享面运行中的唯一视图。**批次成本汇总(GH #42 登记;汇总格式 main 裁决 2026-07-29 修订):** card-top 批次进度行追加成本汇总「判分耗时 X · 批次用时 Y · Token Z(输入 I / 输出 O)」——**判分耗时 = Σ latency_ms**(成本视角的判分累计),**批次用时 = wall-clock**(finished_at − started_at 推导,running 批次取当前时刻),两者并列各司其职(判分耗时对账成本、批次用时答「这批跑了多久」,禁互相冒充);Token 输入/输出分列(`--hs-text-sm` + `--hs-text-secondary`,与 batch-note 同档;运行中随轮询累计增长,settle 后为终值;成本指标中性色不挂档色,规格见「评估成本指标呈现约定」条);cell tooltip 扩展为「状态词 · X/Y 题 · 耗时 X · Token Y」。**只读模式(分享面)不渲染成本汇总与成本段**——成本是运营数据,超出 ticket 54「运行状态与判分覆盖」的公开范围(见「评估成本指标呈现约定」条边界款)。
- **EvalLiveFeed 是批次判分动态流的唯一组件**(issue #17 登记,spec 0004):运行中批次的题目级判分事件流面板——一题判完一行,行内六要素:时间 / 模型 / 评估集 / 题目(`#ID + prompt`)/ 判定方式 / 得分 + 耗时;最新在前(倒序渲染,API 升序翻页服务游标)。**行内展开详情(GH #41 登记,2026-07-29 设计评审;否掉弹窗——feed 是流式监控面板,弹窗打断「盯着流」的场景):** 行可点击展开/收起(hover 反馈 + 行尾展开指示,再点收起;§6 即时反馈),展开区在行下方**内联**渲染四块,纵向堆叠「label + 内容」——① 题目全文(case prompt 完整文本,`--hs-text-sm` regular,`white-space: pre-wrap`);② 期望答案,label 随判定方式分叉:**rule 题 = 「期望答案」(rule_expected 标准答案),judge 题 = 「评分要点」(rubric)**——行内判定方式 tag 既有区分,期望块 label 与之同口径;③ 模型作答(answer_text 原文,null 显「无作答记录」placeholder);④ 裁判结果(得分 + verdict_detail 判定明细,得分着色沿用行内档色)。label 统一 `--hs-text-xs` + `--hs-text-secondary`;展开区整体 `max-height: 240px` + 纵向滚动(防单条爆长撑破 feed 布局),**无横向滚动**(§4 不破)。详情**按需拉取**(点击展开才按 result id 请求,轮询载荷不膨胀;已拉取详情按 id 缓存,重复展开/收起不重拉),展开区自带加载/错误/重试三态(§6,错误只影响该展开区,不污染流本体)。**展开态按 entry id 记录(keyed by id),轮询新条目前插不塌展开态**;行内容六要素不变,展开不额外占列。**边界(与 #17 同口径登记):控制台专属**——期望答案/rubric 是题库内容(spec 0004/W7),详情接口走会话 + Hub 隔离(与 live-feed 同口径),永不出分享面与公开页;settle 后 live feed 卸载,历史批次的判分明细走管理台 EvalRunDetailDialog,不在本组件复活。
- **评估成本指标(耗时/Token)呈现约定(GH #42 登记,2026-07-29 设计评审):** 耗时与 Token 是**成本指标,不是质量指标**——一律中性色(`--hs-text-secondary`,表格内数字 regular),**禁止与 §3 档色/状态色挂钩、禁止闪烁、前端不自定「快/省 = 好」分界线**(§3「阈值以后端口径为准」纪律在成本域的镜像;耗时/token 无业务阈值,永不着色)。格式集中(utils/format.ts,§7 集中原则,组件内禁自写):题级/格子级耗时走既有 `formatMs`(ms/s);批次级累计耗时走 `formatDuration`(<60s「12.3s」;<60min「3 分 12 秒」;≥1h「1 小时 5 分」);Token 走 `formatTokens`(<1000 原样;≥1000 缩写「12.3k」一位小数;≥1M「1.2M」),input/output 分列时标注「输入/输出」。**位置(两层粒度):** ① 批次汇总 = EvalProgressGrid card-top(运行中累计、settle 终值,见该条目);② 模型×评估集明细 = 进度网格 cell tooltip + ScoreCell tooltip(双视图同口径同源字段)+ 报告页(CampaignReportView)榜单下方新增「运行成本明细」表(行 = 模型×评估集 run,列 = 模型/评估集/状态/耗时/输入 Token/输出 Token,el-table,§4 首选;settle 报告页的成本汇总置该表标题行右侧)。**「运行成本明细」表仅 settle 批次渲染**——运行中批次的成本信息由 card-top 汇总 + 双 tooltip 承载,不为运行中批次渲染明细表(明细是 settle 结论的一部分;运行中明细行逐条跳变,与榜单半成品边界同纪律)。**榜单矩阵本体不加成本列**——ticket 78 列 x 恒定不变式,且成本与分数同列暗示质量相关,违反本条中性化约定。**边界:控制台专属**——分享面(/report/{token} 只读网格与 settle 榜单)不渲染任何成本指标(成本是运营数据,超出 ticket 54「运行状态与判分覆盖」公开范围);静态物料(EvalCard)不进成本段(与 tooltip 置信信息同例的登记信息差);成本汇总口径(main 裁决 2026-07-29,评审后修订):**判分耗时** = Σ eval_results.latency_ms(成本视角,与 Token 求和同源);**批次用时** = wall-clock(finished_at − started_at,running 批次取当前时刻;started_at 缺失显 `-`),两者在汇总行并列、各司其职,**禁把 wall-clock 当判分耗时、禁把判分耗时当批次用时**;Token 口径不变 = Σ input_tokens / output_tokens,token null 的 run 按 0 计入求和但在明细表显 `-`。
  - **定位与挂载:** 仅挂在控制台运行中批次视图(EvalLeaderboardView `isUnfinished` 分支,进度网格/实时分数两视图之下);批次 settle 即卸载,历史批次永不展示——「settle 后停止增长」的简单形态由挂载条件承担,组件内无状态清理逻辑。
  - **数据与轮询:** 数据走 `GET /api/campaigns/{id}/live-feed` 游标(since_id)增量拉取;游标、增量合并与轮询 timer **全部由父级视图持有,组件自身零 setInterval**(§6 轮询纪律)——组件是纯展示单元,props 进(entries/loading/error)、`retry` 事件出;增量合并的按 id 去重与封顶(`LIVE_FEED_CAP=200`,超封顶丢弃最旧事件,事件仍可从 API 拉取)走 `utils/liveFeed.ts` 纯函数(vitest 覆盖空/追加/去重/封顶),与 format/degradeCauses 集中原则同例。
  - **呈现:** 时间列 `--hs-text-xs` placeholder(tabular-nums);模型/评估集/题目截断 + `title` hover 全显(§6);**判定方式词表集中 `verdictTypeLabel`(utils/liveFeed.ts)**——规则/裁判/未知或已清除 `-`,tag 用 `el-tag type="info"` 中性灰:**判定方式是类别属性非健康信号**,禁用 §3 状态色;得分 `formatScore` 0–100 + `scoreBand` 档色(≥80/≥50 阈值,§3 同一映射,原始 0~1 分不出 API 层),**null 分(裁判失败)渲染 `-` placeholder(`--hs-text-placeholder`、常规字重)+ hover「裁判失败,未判分」,永不读作 0 分**(W7「裁判失败不计 0 分」的视觉镜像);耗时 `--hs-text-xs` secondary;列表 max-height 320px 纵向滚动,**不开横向滚动**(§4 无特例);行间 1px `--hs-border-light` hairline(ticket 82 表格化精致同构)。
  - **边界:** **控制台专属**——分享页 /report/{token} 与 /board 零调用,逐题判分事件属会话内运行细节,不外流(spec 0004 半成品边界与 ticket 54 分享面信息边界同向);动态流是过程事件而非评估结论,W7 评估口径不受影响。
  - **三态(§6):** 首载 skeleton(仅 entries 为空时,后续轮询 tick 保持已累积列表不打断阅读)、空态「暂无判分动态:等待第一题判分完成」、错误态 el-alert 带原因 + 重试按钮(已累积条目不消失);密度走消费页 16px 档(`--el-card-padding` 既定机制,§2)。
- **分享报告页运行中信息边界(ticket 54 登记,HIGH-1 口径的细化):** 分享面(/report/{token})未完成批次只公开**运行状态与判分覆盖**(模型 × 能力点四态 + X/Y 题),不公开任何分数、名次、涨跌——「实时分数」为登录控制台专属,分享页不提供切换入口;批次 settle 后分享面照常渲染完整榜单(既有行为不变)。依据:状态/覆盖率是运行元数据而非评估结论,不构成 spec 0004 所防的「半成品分数外流」;模型名单 settle 后本就公开;token 高熵、可撤销、走审计(ADR 0006 控制面不变)。已知并接受的增量:运行中分享面可见 per-model 失败归因(settle 后分享榜单从不点名失败模型)——失败是运行事实,不掩盖;该暴露瞬时、token 可控。分享页不新增任何依赖会话接口的交互入口。
- **TrendChart 是趋势类折线图的唯一通用组件**(批 32 登记):裸图表(不带卡片,布局由父级负责),默认在 null 点断线(未判分批次不得连成假分),支持竖向断点标注线(占位灰虚线,如「v2 起题目变更」);ECharts 色板 JS 镜像与 TimeSeriesChart 同一来源(§3 ECharts 条目,迁移期合并)。
- **ModelTrendDialog 是报告页行下钻的唯一趋势弹窗**(批 32 登记,ticket 51 修订):按模型按需拉取 `/api/campaigns/{id}/trends`,分数线(版本断点标注「vN 起题目变更」+ 判分口径断点标注「判分口径已变更」,同一位置双断点合并为一行标注,均复用 TrendChart 灰虚线断点机制)+ 探测成功率/延迟并列;已删除模型带「已删除」tag;加载/空/错误三态齐全。
- **AppHeader 导航按登录态过滤【已迁 surface brief(GH #50)】:** 未登录只渲染公开页导航项(状态总览 + 评估榜单→/board,spec 0010),不渲染会被路由门禁弹走的项(任务、管理入口);登录态导航 = 状态总览 + 评估榜单(→/eval)+ 任务中心(anonOnly 项登录即隐),登录态随路由切换重检(沿用 refreshAuth watch 先例),不写死 mount 时一次性判断。**未登录态 header 一律不渲染登录按钮(ticket 90,用户裁决):** 醒目登录按钮对公开读者传递「内容要账号」的错误信号;判定走 route `meta.public` 通用位,不设单页特例(匿名读者经路由门禁本就只可达公开页);登录入口统一由 PublicFooter 承担(见下条)。
- **PublicFooter 是公开三页页脚管理入口的唯一组件**【已迁 surface brief(GH #50)】(ticket 90 登记,/board 页脚模式推广):hairline(`--hs-border-light`)+ 一行左右分置(2026-07-28 用户裁决,取代居中独行)——左 `© 2026 HubScope` 版权,右「管理登录」→ /login(均 `--hs-text-xs` placeholder,链接 hover brand;间距走 `--hs-space-*` 令牌);状态总览、EndpointDetail、/board 三页一律复用,禁止各处自造页脚登录入口;/login 页自身不渲染;登录态照常渲染(与 /board 先例一致)。**豁免:** /report/:token 分享页不挂页脚——接收者非管理读者,外部物料不出现任何登录入口。
- **/board 公开榜单页【已迁 surface brief(GH #50)】(ticket 81 登记,spec 0010):** 榜单与状态板并列为公开侧第二页,未登录直达。页头构成:`评估榜单` 标题行(`--hs-text-xl`/600,工具风层级,非营销 hero)+ 内容区。边界:恒显**最新 settle 批次**矩阵榜单(复用 Leaderboard 组件,`shared` 口径「保存图片」),列头排序 + family 筛选 + 保存图片**全部客户端完成**(公开端点一次性返回完整 report,不接参数;`sortRows` 镜像服务端口径——null 沉底/降序/model_id 字典序 + 判分不完整恒沉底(ticket 92,口径见「Leaderboard 判分不完整模式」子条目⑦),禁第二口径);**无批次切换、无行下钻(行不可点,Leaderboard `selectable` prop)、无 live 榜单、无轮询**;运行中批次时榜单上方一行静态中性提示「新一批评估进行中,当前展示已完成批次 #N」(`--hs-text-sm` secondary,无底色无边框);无 settle 批次 → 空态「暂无已完成的评估批次」(running 时附提示行)。**登录入口(ticket 90 修订,取代 ticket 81「/board 与状态板有差异」口径):** 所有公开页 header 一律无登录按钮,登录入口统一为 PublicFooter「管理登录」页脚(见 PublicFooter 条)。未登录态网络面板零 session API 调用(auth status 除外)。
- **StatusCard 是状态分享卡的唯一渲染模板**(批 56 登记,批 59 重设计构成):720px 逻辑宽、2x 导出的竖版品牌物料,自上而下固定构成——① 品牌区(`--hs-brand` 4px 品牌条 + `--hs-brand-soft` 浅底 + BrandMark + Wordmark + 「服务状态」`--hs-text-2xl`/600——「HubScope」字样由 Wordmark 承担,标题词不再重复品牌名);② 范围行(无筛选纯文本「全部端点」,有筛选逐项 chips:描边 + `--hs-radius-sm`,前缀灰 label + 值,状态 chip 值用语义色;分组卡首位恒为分组 chip,与筛选 chips 并存,一个不漏);③ hero panel(可用率优先,批 59 第二轮迭代,替代原「tone-tinted 结论块 + 独立指标行」两区块;用户反馈顶部太告警化、优先展示异常端点):结论与指标合并为单一 panel——`--hs-bg-page` **中性浅灰底(无 tone tint)** + `--hs-radius-lg` + padding 16px 20px,左右两列 + 1px `--hs-border` 竖分隔。左列自上而下:「24h 可用率」`--hs-text-xs` 标签 → `--hs-text-display`/600 大数字按 §3 三档着色 + 小号次级「%」(`--hs-text-md` secondary)——可用率大数字当主标题,其三档着色承担严重度信号,顶部不再用告警化色块;其下 verdict 文案(与 HealthBanner 同源结论词,如「5 个端点降级」,tone 着色但 `--hs-text-sm` 次级)+ failing 静态双编码(橙实心点 + 「含 N 个告警」橙描边 chip);再下**完整分布串**「正常 N · 降级 N · 宕机 N · 告警 N」四段恒列,`--hs-text-xs`,非零段状态词语义色/600 + 数字 `--hs-text-primary`,零计数段整段 `--hs-text-placeholder`。右列:「平均延迟」同构标签 + `--hs-text-xl`/600 数字,`--hs-text-primary` 不着色。**防作假不变:verdict 与四段恒列分布串仍在,异常不掩盖——只是不再当头条、不再有 tone tint 色块;空态(tone-empty)可用率渲染 `-` + 「24h 内无探测数据」,verdict 与分布串均不渲染,中性灰底保证「无数据」永不读作「全部正常」。** null 延迟 → `-` placeholder + 「24h 内无探测数据」`--hs-text-xs`;④ 24h 分段可用率条(组内聚合,口径见 §3 三档条目,24 格分段填满式、格高 16px、`--hs-radius-xs`、2px 间距;条下两端「24 小时前」「现在」`--hs-text-xs` placeholder;聚合函数抽 utils 纯函数,禁按端点简单平均);⑤ 异常明细(封顶 10 条,严重度排序 告警>宕机>降级,**三段式行**:行 1 = 状态词语义色 `--hs-text-sm`/600 + 「模型 · 协议」`--hs-text-md` 截断 + 右侧单端点 24h 可用率 `--hs-text-sm` 同档着色(null → `-`),行 2 = status_reason `--hs-text-xs` secondary、最多两行截断,reason 为空则不渲染行 2,**行 3 = 单端点 24h 打点条**——全宽 24 格分段填满式、格高 8px、`--hs-radius-xs`、2px 间距、**无轴标**,左缩进与 reason 对齐(margin-left 40px),复用 §3 三档着色与 dotTier;打点条让维护受众一眼看出故障时段——「最近 1 小时炸的」vs「全天半死」,单点可用率数字做不到这个;行间 hairline 分隔;overflow 收尾「另有 N 个异常端点未列出,详见状态板」);⑥ 正常端点汇总行(「其余 N 个端点正常 · 24h 可用率区间 min%–max%」,「正常」success 色、余文 secondary;min==max 显示单值,全部 null 则附「(24h 内无探测数据)」;全正常态此行替代异常明细,措辞「全部 N 个端点正常」);⑦ 一句话总结(见下方独立条目);⑧ 页脚(hairline 分隔,左「生成于 YYYY-MM-DD HH:mm」+「另有 N 个已停用」`--hs-text-xs` placeholder,右 location.origin)。空态沿用批 56:零匹配/全停用时范围 chips 保留、hero panel 中性灰底 + 可用率 `-`(verdict 与分布串不渲染,永不读作「全部正常」)、分段条全占位、明细「暂无匹配的 Endpoint / N 个端点均已停用」、不渲染总结与正常汇总行。结论判定仍与 HealthBanner 同源,统计集合=快照范围;明细状态词按 §3 静态物料约定用着色文字,不复制状态灯实现。**导出画布恒亮主题(§2a),物料内令牌引用亮主题取值。**
- **StatusCard 单模型模式**(ticket 60.5 落地,2026-07-24 设计评审登记):单模型分享(EndpointDetailView / DashboardView 单端点入口,`createSingleModelSnapshot` 创建,`hubName` 字段为标记)走同一物料模板的第二种构成——不是新物料,不是全局版的开关切换;构成元素(品牌区/hero panel/分段条/明细/页脚)与全局版同源。
  - **判定口径:** `entries.length === 1 && snapshot.hubName !== undefined`。筛选后只剩一个端点的全局/分组快照不算单模型卡(范围 chips 描述子集,hero panel 仍按聚合模式渲染 verdict + 分布串)。
  - **范围区:** 不渲染「全部端点」纯文本行(单模型快照下它把单端点子集读作全集,违反批 56 防作假约定),渲染三枚 chips——`模型 · {model_id}`、`协议 · {protocol}`、`Hub · {hubName}`。状态 chip 不出现(当前状态在 hero panel 状态陈述行,不重复语义)。防作假不变式保持:chips 与卡片所有数字同源(都来自唯一 enabled entry),比「全部端点」更精确地标注了统计范围。
  - **hero panel:** 复用 StatusCardSingleModelMetrics 组件(与全局版 StatusCardMetrics 兄弟关系,StatusCard 按判定口径二选一挂载)。构成:可用率大数字(`--hs-text-display` 档,与全局版同字阶,不降档)+ 单状态陈述行(替换全局版 verdict + 分布串,`--hs-text-sm`/600 tone 着色)+ failing 双编码(橙实心点 + 「含告警」橙描边 chip,不带数字)| 平均延迟 | 24h 分段条,全部沿用全局版样式与着色。四段分布串「正常 N · 降级 N · 宕机 N · 告警 N」在单模型模式下不渲染(0/1/0/0 全为噪音,单状态陈述已承担同样信息)。
  - **单状态陈述行文案:** `正常 · 24h 可用率 X%` / `正常 · 24h 可用率仅 X%,低于 95%` / `降级 · 24h 可用率 X%` / `宕机 · 24h 可用率 X%` / `告警 · 24h 可用率 X%`(可用率 null 时后半段改「24h 内无探测数据」或省略后半段)。状态词严格用 §7 endpoint 词表四词(正常/降级/宕机/告警),不新增概括词;陈述行不重复 Hub 名/模型名(范围 chips 已承担)。
  - **评估区(hero panel 右栏):** 评估总分(label「评估总分」`--hs-text-xs` + `--hs-text-xl`/600 数字,`formatScore` 0–100 整数)+ 能力 suite tags(`el-tag size="small"`,`suite_name + formatScore(score)`,封顶 6 个,超出以 `+N` 计数 chip 收尾,不撑爆卡片)。无评估数据时右栏渲染一行「暂无评估数据」`--hs-text-xs` placeholder,右栏不消失(布局稳定)。评估数据为最近一次已 settle 批次的 ModelEvalSummary,运行中批次半成品分数不进卡(与 ticket 54 分享面信息边界同向——卡片是对外物料,半成品分数不外流)。卡片不标注「评估于」时间戳(避免与页脚「生成于」语义冲突)。
  - **明细区:** 组件复用 StatusCardDetail;单模型异常时仍渲染单条三段式明细行(含 status_reason 与打点条,异常不掩盖)。「其余 N 个端点正常」汇总行不渲染(单模型卡没有「其余」);全正常态汇总行措辞改「当前状态正常」(避免数量词)。
  - **小结措辞(单模型版,位置/样式沿用「StatusCard 一句话总结」条目):** failing →「触发告警,建议立即处理」;down →「宕机,建议优先排查」;degraded(有持续信号)→「持续降级约 N 小时,建议排查上游」(N 口径同全局版,连续证据约束不变);degraded(无持续信号)→「降级,建议关注,暂不紧急」;healthy 且可用率 <95% →「状态正常,但 24h 可用率仅 X%,建议持续观察」;healthy 且可用率 ≥95% →「近 24 小时运行平稳,无需处理」。无 24h 数据时沿用全局版后缀规则。「不掩盖异常」约束保持:单状态陈述行 tone 着色,告警双编码完整保留,禁止输出掩盖异常的措辞。
  - **生成规则纯函数:** 单模型小结文案走 `singleModelSummaryText(entry, availability)`、单状态陈述行走 `singleModelStatement(entry, availability)` 纯函数,与全局版 `summaryText` 并列收在 `utils/statusCardSummary.ts`,组件内不写文案字面量。
- **分组独立分享入口【已迁 surface brief(GH #50)】(批 59 登记):** OverviewGroupSection 标题行右端(group-metrics 之后)放 text 型 `el-button`(Share 图标 + 「分享」文字,不用裸图标按钮——状态板读者 3 秒场景下图标语义不够直白),`@click.stop` 拦截冒泡、不触发整行折叠,hover 走 Element Plus text 按钮品牌色反馈;点击复用 StatusShareDialog(弹窗本体不变),快照范围 = 该分组条目 ∩ 当前页面筛选,scope chips 首位恒为分组 chip(label「分组」,值「厂商/能力/协议 · 组名」,维度词表 family→厂商、capability→能力、protocol→协议),其后列全部生效筛选条件。**卡片所有数字一律从快照 entries(enabled)计算,与范围 chips 恒一致(批 59 口径修订):筛选快照不得引用 OverviewGroup/Overview 的未筛选聚合字段,否则数字描述全集、chips 声明子集,自相矛盾,违反批 56 防作假约定;且 Overview 全局无 avg_latency_ms 字段,透传路径本就不完整。** 两个标量口径:① 24h 可用率 = 快照内 enabled entries 的 dots_24h 按小时求和 ok/total(探测加权,与 `internal/server/overview.go` groupAccumulator 同定义,无筛选时与后端数字构造性相等,口径见 §3 批 59 条目);② 平均延迟 = enabled entries 的 p50_ms 均值(前端无法从 dots 复现探测加权延迟,这是唯一 scope 恒一致的口径;已知代价:与分组标题行「均延」的探测加权 mean latency 数值可能略有差异——卡片内部自洽优先于与页面逐字相等);StatusCardSnapshot 只扩展 `group` 字段,不携带任何聚合标量(statusCardSnapshot.ts)。Dashboard 全局「分享状态」入口保留现状(筛选行主按钮,不动)。
- **StatusCard 一句话总结(批 59 登记):** 位置=明细区与页脚之间,hairline 分隔后;形态=「小结」前缀标签(`--hs-text-xs` placeholder)+ 一句话(`--hs-text-sm` secondary、常规字重,无底色无边框无语义色)——视觉权重明确次于 hero panel 主结论区,句式以行动建议动词(建议/无需)收尾,与 verdict 的陈述句式区分。生成规则抽纯函数(utils/statusCardSummary.ts),优先级自上而下命中即止:① 有告警 →「有 N 个端点触发告警,建议立即处理」;② 有宕机 →「N 个端点宕机,建议优先排查 {首个宕机模型}」;③ 有降级且存在连续非绿时段 →「{模型} 持续降级约 N 小时,建议排查上游」(N = 该端点 dots_24h 自最新向前连续「有探测且非绿」格数——只有 fail/partial 计入,无数据灰格与绿格一样中断计时;取全组最长者。**持续时长必须有连续证据支撑**:灰格计入会让稀疏数据端点一路数到窗口起点、恒输出「约 24 小时」,属无证据的时长宣称,违反本条「不掩盖异常」的对偶约束——不夸大异常,对维护受众是狼来了);④ 有降级无持续信号 →「N 个端点降级,建议关注,暂不紧急」;⑤ 全正常但 24h 可用率 <95% →「状态全部正常,但 24h 可用率仅 X%,建议持续观察」;⑥ 全正常且有数据 →「近 24 小时运行平稳,无需处理」;⑦ 无 24h 数据 → 在命中句后追加「暂无 24 小时探测数据」。空态(零匹配/全停用)不渲染总结行。**总结不得掩盖异常:** 只要存在非正常端点,总结首句必须指向最严重的异常(告警>宕机>降级),禁止输出「运行平稳」类措辞——这是批 56 防作假约定在文案层的镜像。
- **结论必须标注统计范围(防作假约定,批 56):** 任何呈现汇总结论的导出/分享物料,结论旁必须显式列出统计范围——无筛选标「全部端点」,有筛选逐项列出全部生效条件(一个不漏),零匹配时范围仍需保留且结论用中性「暂无数据」;禁止把局部集合呈现为全局结论,禁止零匹配显示「全部正常」(ADR 0007 防作假语义在状态板域的镜像)。批 59 补充:分组也是范围,分组卡必须带分组 chip(见上条),分组卡片不得省略分组维度词。ticket 76 补充:评估域镜像——EvalCard 范围行 chips 逐项列出批次/family 筛选/非默认排序/涨跌基准(见 EvalCard 条目),空筛选结果 chips 保留 + 中性「暂无匹配模型」,禁止读作「全部上榜」类结论。
- **分享卡片外框约定(ticket 76 登记,StatusCard/EvalCard 两物料同构):** 导出分享物料的外框固定三段——① 品牌区(`--hs-brand` 4px 品牌条 + `--hs-brand-soft` 浅底 + BrandMark + Wordmark + 物料标题 `--hs-text-2xl`/600,标题词不重复品牌名);② 范围行 chips(描边 + `--hs-radius-sm`,前缀灰 label + 值,防作假约束见「结论必须标注统计范围」条);③ 页脚(hairline 分隔,左「生成于 YYYY-MM-DD HH:mm」,右 location.origin)。两物料各自实现外框、**不抽公共子组件**(ticket 76 设计评审裁定):StatusCard 外框与单模型/汇总双模式耦合,抽取需重构批 56/59 重规格物料,回归风险大于去重收益;共享的本质是设计约定(即本条),代码重复量小;出现第三张分享物料时再评估抽取。
- **EvalCard 是榜单分享卡的唯一渲染模板**(ticket 76 登记,spec 0007;ticket 79 随 spec 0009 矩阵化修订):720px 逻辑宽、2x 导出、竖版品牌物料,**导出画布恒亮主题**(§2a),物料内令牌引用亮主题取值。卡片 = 当前所见:批次 + family 筛选 + 排序全部生效;chips 与卡片全部数字同源(同一份 report 响应,禁止引用页面其他聚合)。快照由纯函数 `buildEvalCardSnapshot(report, query)` 生成(utils/evalCardSnapshot.ts),打开弹窗即冻结。自上而下五区:
  - ① **品牌区:** 外框约定同款,标题「评估榜单」。
  - ② **范围行 chips(批 56 同构,逐项一个不漏):** `批次` chip 恒列(label「批次」,值 `#N · 定时/手动 · 已完成/失败`,词表用 §7 批次/运行状态四词,中性不着色——failed 的强调由警示行承担,done 是常态无需着色);有 family 筛选 → label「系列」值 family 名;非默认排序 → label「排序」值能力点名;基准 chip 恒列(baseline 存在时):label「涨跌基准」,值 `较批次 #M` 或不可比原因(`题目已变更,分数不可比` / `判分口径已变更,分数不可比` / `考核口径不同,分数不可比`,与 Leaderboard baselineNote 同词),baseline 缺失(首个已完成批次)不渲染基准 chip。chip 顺序固定:批次 → 系列 → 排序 → 涨跌基准(矩阵无非总分视图,「维度」chip 与排序特判已随 ticket 79 退役)。
  - ③ **failed 警示行(仅 failed 批次):** 「批次有 N 个评估运行失败,榜单仅统计已完成的评估集」,与页面 alert 同口径同措辞,`--hs-text-sm` + `--hs-warning` 着色文字,无底色无边框。
  - ④ **榜单行(ticket 79 矩阵构成,与页面同构;ticket 82 表格化同步):** 静态列头(名次/模型/总分/涨跌/能力点名列,不可点无排序指示,能力点名字唯一出现处,列头下 1px `--hs-border` hairline)+ 矩阵行——行间 1px `--hs-border-light` hairline + 行高 46px 同页面节奏;名次(前 3 名 3px `--hs-brand` 竖条 + `--hs-text-lg`/600 brand,余 secondary,非前 3 名透明竖条位)+ 模型名(截断;物料无 hover,不依赖 `title` 全显)+ 总分(`--hs-text-xl`/600 墨色 + 6px 档色条,0–100 同刻度,轨道 `--hs-bg-hover`)+ 涨跌箭头(列渲染条件 = **基准可比**;条件不满足整列不渲染、不占位;列内行级口径与页面一致:delta 非零显 ▲▼ + `formatScoreDelta`,null/持平显 `–` 占位)+ 维度格子(ScoreCell `staticMode`:无 tooltip,水印按页面同一宽度规则渲染——物料典型列宽低于阈值,水印自然不渲染,置信信息留在页面,已登记信息差)。列宽预算(720px 画布、内容宽 640px):名次 24 / 模型 150 / 总分 64 / 涨跌 60,维度列 `minmax(0,1fr)` 等分(5 列约 56–68px,随涨跌列渲染与否);行不渲染 family tag(family 由范围 chip 或模型名承担)、不可点、无 hover。**判分不完整行(ticket 92):** 名次 `–`(rank 字段 number|null)+ 模型名之下同款水印 + 总分 `–` 空轨道 + 涨跌 `–`,与页面同构,口径见「Leaderboard 判分不完整模式」子条目⑨。**封顶 20 行**,超出收尾一行「另有 N 个模型未列出,详见评估榜单」(`--hs-text-xs` secondary,与 StatusCard 明细 overflow 收尾同构)。
  - ⑤ **页脚:** 外框约定同款(无附加段)。
  空筛选结果:chips 全部保留 + 中性「暂无匹配模型」(`--hs-text-placeholder`),failed 批次空态时警示行仍渲染(原因由警示行承担)。静态物料无 hover:tooltip 置信信息不进卡片(已知信息差,ScoreCell 条目已载);无动画语义冲突。
- **EvalShareDialog 与榜单图片分享入口(ticket 76 登记):** EvalShareDialog 结构机制对齐 StatusShareDialog——预览(限高 62vh 滚动,`align-items: flex-start` 防压扁)+ 离屏双份捕获(规避 snapdom 祖先 overflow 裁剪)+ 复制图片/下载 PNG + 复制降级(非安全上下文置灰 + 「当前环境不支持复制图片,请使用下载」,下载永可用,§6 批 56 条)+ 暗色会话捕获前脱离 `html.dark` 级联(恒亮捕获);失败不锁按钮(按钮即重试路径)。入口 = Leaderboard 工具条右端 text 型 `el-button`(Share 图标 + 文字,批 59 先例),**仅 settle(done/failed)批次渲染——实现口径 `!live`**:live 模式工具条不渲染该按钮,运行中/等待中批次三处均无入口(spec 0004 半成品分数不外流边界不变;公开分享页运行中本就只渲染进度网格,无 Leaderboard 挂载点)。三处生效:/eval、控制台报告页 `/campaigns/:id/report`、公开分享页 `/report/:token`。文案:控制台两页「分享图片」,公开分享页「保存图片」(读者是接收方;Leaderboard 以 `shared` prop 切换,弹窗标题随入口文案)。**报告页 header 既有「分享」(铸链接,ADR 0006)改名「复制链接」**——与工具条图片分享消歧,行为不变(铸 token + 复制 + 手动复制降级)。公开分享页「保存图片」是共享面第一个主动作按钮:**纯客户端生成**(props 快照 + snapdom + 本地下载/剪贴板),不引入任何会话接口依赖,ADR 0006 不变式守住。导出文件名 `hubscope-eval-批次N-YYYYMMDD-HHmm.png`(scope 恒为批次号;范围细节由图内 chips 承担,不进文件名)。
- **AppHeader 批次进度入口**【已迁 surface brief(GH #50)】(ticket 52 登记):仅登录态渲染(与导航同一过滤时机),位于 header-right 操作按钮左侧;文案「批次运行中 X/Y」(X/Y 口径与榜单进度一致),点击跳 /eval;发现靠 mount + 路由切换重检(refreshAuth watch 先例),仅存在未完成批次时 3s 轮询,settle 即停并隐藏,卸载必清理;禁用橙(failing 专属)与闪烁(闪烁为 failing 告警专属)。
- **角色 tag 是用户身份角色的唯一展示单元**(ticket 62 登记):AppHeader 右栏当前用户、AdminView 用户列表、UserManager 一律复用,禁止各处自造角色色。词表固定四词,不新增同义词:super_admin→「超级管理员」、admin→「管理员」、operator→「操作员」、viewer→「观察者」(与 spec 0005 角色定义一致)。**角色语义是「权限层级」非「健康度」**,禁用 §3 的 success/warning/danger 状态色——红黄绿是 endpoint 与批次健康信号,状态板读者眼里红=异常、绿=正常,把 super_admin 染红会读作「告警」、operator 染绿会读作「健康」,属语义错位(本映射经设计评审登记,与 §3 两套状态色并列、语义域不混用)。
- **角色色映射:**

| 角色 | el-tag type | 语义 |
|---|---|---|
| super_admin / admin | `primary`(brand teal) | 管理权(全局或 Hub 内全权) |
| operator / viewer | `info`(中性灰) | 非管理(操作 / 只读) |

  管理权=brand 与 §2「品牌主色=主按钮/链接/当前导航/聚焦态」同向(强调=可支配),非管理=info 中性灰。super_admin 与 admin 同色:区分靠词表(「超级管理员」vs「管理员」)+ 数据域(全局不绑 Hub vs Hub 内),不靠颜色加权——super_admin 稀有但染红代价(告警串扰)高于收益(视觉强调)。与 §3「等待中 中性灰」同为中性语义但词表与语义域不同(角色 vs 批次等待),靠上下文与词表消歧,禁止把等待中灰借用到角色域。并入说明:`primary` type 随主色从蓝 #3B5BFD 切换为 teal(亮 teal-600/暗 teal-500),`info` type 映射到 `--hs-info`,组件零改动,仅 ep-theme.css 映射值变化。
- **实现集中:** `roleLabel(role)` + `roleTagType(role)` 抽 `web/src/utils/role.ts` 纯函数,供 AppHeader / AdminView / UserManager 复用(口径同 `utils/format.ts` 的集中原则);el-tag 走 `type` 属性语义色、`size="small"`、默认 `effect="light"`,不自着色、不硬编码色值、不引入调色板外色相;未知/未登录角色回落 `info` + 「未知用户」占位(三态精神)。未登录态不渲染角色 tag(与「AppHeader 导航按登录态过滤」同时机)。
- **协议 tag 是契约家族区分的唯一展示单元**(GH #34 登记,spec 0016):EndpointCard、EndpointTable、EndpointDetailView 三处一律复用同一映射,禁止各组件自写三元表达式。**例外(GH #54 登记):** Dashboard 分组模式下,组内筛选后 entries 协议全部相同时,卡片协议 tag 收敛为组头一枚(组名即协议的 protocol 分组不渲染组头 tag 且卡片同样收敛;flat 模式与混搭组不收敛)——收敛是屏幕密度优化,映射与词表不动,规格见 dashboard surface brief。词表 = 协议原值(anthropic / openai / images_generation / images_edit),不翻译、不缩写。**映射表:**

| 协议 | el-tag type | 语义 |
|---|---|---|
| anthropic | `success`(绿) | chat 契约家族 A(存量配色保留) |
| openai | `warning`(黄) | chat 契约家族 B(存量配色保留) |
| images_generation | `info`(中性灰) | 图像契约(spec 0016) |
| images_edit | `info`(中性灰) | 图像契约(spec 0016,前瞻登记,随后端落地自动生效) |

  **配色语义是「契约家族区分」非「健康度」**——与角色 tag 先例(ticket 62)同构的语义域隔离:success/warning 在协议域读作「两个 chat 家族的区分色」,上下文是协议词而非状态词,不构成 §3 状态色的借用;failing 橙与 danger 红仍专属状态域,协议 tag 永不使用。**整体迁出状态色的替代方案被否(GH #34 设计评审裁决):** 全部改 info 会让 anthropic/openai 失去颜色区分(区分度只剩文字),且推翻已批 spec 0016 与 issue AC「anthropic/openai 配色不变」、动用户已建立的视觉习惯——消除理论张力的收益小于迁移成本;存量 chat 配色与状态色的张力登记为已知并接受,出现同卡串扰实证再议。图像双协议同 info 色:区分靠词表原值(images_generation vs images_edit 词本身不同),不为协议域引入新色相;与角色 tag 的 info 灰跨页面不相遇(协议 tag 在状态板/端点详情,角色 tag 在 header/管理台用户域),靠语义域与词表消歧。暗色:info 映射 `--hs-info`(暗色提 gray-500),success/warning 暗色同值,四协议可分辨性 = 色(绿/黄/灰)+ 词,双主题构造性成立。**实现集中:** `protocolTagType(protocol)` 抽 `web/src/utils/protocol.ts` 纯函数(role.ts 先例),未知协议回落 `info`(防御 + 与未来 images_edit 自动兼容);el-tag 走 `type` 属性 + `size="small"`,不自着色。StatusCard 范围 chips(描边文本 chip,含「协议 · {protocol}」)不消费本映射、保持不变。**附记(GH #34 同票):** ModelAdder 提示文案随本映射修订为「添加时系统自动试通该模型的候选协议(chat 模型:anthropic / openai;图像模型:另加 images_generation / images_edit),试通成功的协议自动建立 Endpoint,全部不通则拒绝添加。」——前端添加时不知道 capability(分类在后端 trialProtocolsFor),动态按 capability 措辞不可行,故用中性准确措辞;原「自动建立 anthropic 与 openai 两条 Endpoint」双重失真(试通才建不一定两条;image 模型另加图像协议)。(2026-07-29 修订,GH #43:images_edit 试通随 GH #32 落地,措辞从「另加 images_generation」同步为「另加 images_generation / images_edit」。)
- 反馈三件套:
  - 操作结果 → `ElMessage`(成功/失败/警告,见 HubManager 用法);
  - 破坏性操作(删除、禁用)→ `ElMessageBox.confirm` 二次确认;
  - 表单校验失败 → 表单内联提示,不用弹窗。
- 新组件若为通用展示单元(非业务组合),放 `components/` 并在本文件登记;业务一次性组件就近放视图内。

## 6. 交互规范

> **视觉交互部分已迁 `DESIGN.md`(GH #49)**;三态/轮询纪律(visibilityPoll、settle 转场口径)属工程行为约定,迁移期仍以本节为准,随语义手册定稿(GH #59)归位。

- **三态必备:** 加载态(skeleton 或 loading)、空态(空数据说明 + 引导操作)、错误态(错误原因 + 重试入口),任何数据区块缺一不可。
- **长文本:** 模型名/Hub 名/错误信息一律截断 + `title` hover 全显。
- **轮询:** `setInterval` 必配对清理(组件卸载时);可点汇总卡有反馈态且可再点取消(fix fc8bdb6)。
- **轮询可见性感知(GH #22 登记,spec 0015 决策 5):** 所有页面轮询一律走共享封装 `utils/visibilityPoll.ts` 的 `createVisibilityPoll`,禁止各处自造 `visibilitychange` 监听、禁止第三套轮询实现。口径:标签页隐藏时——状态板 overview 轮询 10s 降频为 60s(`useOverview.HIDDEN_POLL_INTERVAL_MS`;挂大屏/后台标签页场景降频不停摆,读者切回时数据不至长时间陈旧);批次类轮询(AppHeader 批次进度 3s、/eval 3s、报告页 3s、EvalOpsPanel 批次追踪 1.5s)整段暂停;`visibilitychange` 回前台立即触发一次刷新再恢复原周期(`refreshOnVisible` 默认开)。settle 转场口径不变:批次在隐藏期间 settle 时,回前台的立即刷新即「观察到 settle 的那次响应」,照常停轮询并走 ElMessage 提示。清理纪律不变:卸载时调 `handle.clear()`(替代原 `clearInterval` 对),封装内部同时停表并移除 visibility 监听。
- **即时反馈:** 点击类操作在请求期间给 loading 或禁用态,不静默等待。
- **榜单/报告类消费页三态与轮询:** 批次切换器空态(无任何批次 → 空态 + 引导文案)、榜单空态、下钻趋势加载态缺一不可;选中等待中/运行中批次时榜单区呈现进度态(进度 + 批次状态词),不显示半成品名次,失败批次给错误态 + 原因;仅当选中未完成批次时才轮询进度,完成后停轮询并刷新榜单,卸载必清理。
- **导出物料的复制降级(批 56):** 「复制图片」依赖 `navigator.clipboard.write`,非安全上下文(HTTP 裸 IP)必须置灰 + 提示降级路径(「当前环境不支持复制图片,请使用下载」);下载能力不得受安全上下文影响,永远可用。
- **批次 settle 转场轮询口径(ticket 52):** 观察到 done/failed 的那次轮询响应即停轮询,渲染以该次数据为准,不再补拉;完成提示走 ElMessage(带批次号与原因,成功用 success、失败用 warning)。同一口径适用于所有渲染未完成批次的消费页:/eval、/campaigns/{id} 报告页与 /report/{token} 分享页(ticket 54),三处转场提示文案与轮询停止语义一致。
- **自适应展开区约定(ticket 89 设计评审登记,登录验证码区首例):** 条件触发的表单展开区(默认不渲染、后端信号到达才出现)遵守三条——① **禁默认态预留空白占位**(隐藏态零占位,默认布局与无展开区时像素级一致);② **展开走平滑高度过渡**(grid `0fr→1fr` 或 max-height + opacity,时长走 `--hs-transition`,禁瞬现瞬移的布局跳动;整页 flex 居中容器因此产生的重排随过渡平滑完成,可接受);③ **展开区尽量压扁振幅**(同行复合布局优先于堆叠,减少页面重心位移量)。收起同理(登录成功即跳路由的场景可省略收起过渡)。
- **位图物料渲染纪律(ticket 89 设计评审登记,图形码首例):** 后端生成的位图(图形码 PNG 等 data URI)是物料性质的图片,**不走语义令牌、不做暗色适配、禁 CSS 滤镜**(invert/hue-rotate 等)——滤镜改动配色属调色板外操作且损害人类可读性;统一渲染规约 = 固定尺寸容器(尺寸按后端契约声明为常量并注释互指,不属 px 字面量禁令)+ `1px solid var(--hs-border)` 描边 + `--hs-radius-sm` 圆角(控件层级),容器在加载/失败态保持同尺寸防布局位移,失败态容器内给原因 + 重试入口(§6 三态)。暗色下直接渲染亮底位图可接受,描边容器使其读作有边界的图片而非残留亮块。

## 7. 文案规范

- 界面一律简体中文;**状态词表分两套,互不混用:** endpoint 状态=**正常 / 降级 / 宕机 / 告警**(与 StatusBadge LABELS 一致,不新增同义词);批次/运行状态=**等待中 / 运行中 / 已完成 / 失败**(沿用 campaignStatusLabel 既有口径)。禁止把一套词借用到另一语义域(如批次失败不得称「宕机」)。
- **降级成因副词表**(ticket #7 登记,spec 0013;endpoint 状态域,与两套状态词表并列、互不混用):**可用性**(24h 成功率低于 95%:时通时不通,宕机前兆,偏紧急)/**延迟**(24h P95 超过 2× 7 天 P50 基线:次次都通但变慢,偏观察)。仅作为「降级」的成因修饰出现(用法见 §5 StatusBadge 降级成因副标签条),**不是第五状态词**;endpoint 四状态词表不变;批次/运行域不借用;评分域既有文案(如 score.go「性能降级,封顶 80 分」)不受本词表管辖、不改动。
- **分数展示统一 0–100 整数**(null → `-`),`formatScore` 集中于 `utils/format.ts`,组件内禁止自写 `toFixed` 分数格式;0~1 原始分只存在于 API 层。
- 按钮用动词短语(「触发同步」「新建 Hub」),不用「确定/提交」以外的泛词;错误消息必须带原因,不只说「失败」。
- 数字与时间格式统一走 `utils/format.ts`,不在组件内各写格式化;评估成本格式同此集中(GH #42 登记):题级耗时 `formatMs`(ms/s)、批次累计耗时 `formatDuration`(分/小时档)、Token `formatTokens`(k/M 缩写一位小数),规格见 §5「评估成本指标呈现约定」条。

## 8. 规范的维护

- 本文件(业务语义手册)由 `plan` agent 维护;设计评审中做出的新约定按层回写——视觉/布局 → DESIGN.md,页面/组件 → surface brief,业务语义 → 本文件(回写前置与批次节奏见 collaboration.md §3),不回写视为未约定。
- 本文件与 [load-bearing-walls.md](./load-bearing-walls.md) 的关系:本文件管「业务语义一致性」,承重墙管「系统语义」;冲突时以承重墙为准(如状态机红黄绿语义由 W5 决定,本文件只做语义映射与词表约定)。
- 与 ProxyHub 上游规范的关系:令牌刻度与审美纪律源自 proxyhub `docs/design-frontend.md`(视觉层已在 DESIGN.md 登记);HubScope 侧业务语义(状态词表、防作假约定、failing 例外、导出物料)以本文件为准,不回写上游;上游刻度修订时由 `plan` agent 评估同步(落 DESIGN.md)。

---

## 附录 A:旧 token → 新 token 完整映射表

> **已迁 `DESIGN.md`(GH #49)**——令牌取值以 DESIGN.md frontmatter 为唯一 normative 来源;本表保留为 ticket 73 品牌并入的历史映射档案,不再更新。

| 旧令牌(单层 tokens.css) | 新令牌 | 取值变化 |
|---|---|---|
| `--hs-brand` `#3B5BFD` | `--hs-brand`(semantics) | 亮 teal-600 `#0c8078` / 暗 teal-500 `#0faea2` |
| `--hs-brand-hover` `#6180FF` | `--hs-brand-hover` | 亮 teal-500 `#0faea2` / 暗 teal-400 `#30c4b8` |
| —(原无) | `--hs-brand-active`(新增) | 亮 teal-700 `#0a6963` / 暗 teal-600 `#0c8078` |
| `--hs-brand-soft` `#EEF2FF` | `--hs-brand-soft` | 亮 teal-50 `#effcfa` / 暗 teal-900 `#063f3d` |
| `--hs-text-primary` `#1F2329` | `--hs-text-primary` | 亮 gray-900 `#0f1b20` / 暗 `#e2e8f0` |
| `--hs-text-regular` `#3E4450` | `--hs-text-regular` | 亮 gray-700 `#324249` / 暗 `#c3c7cd` |
| `--hs-text-secondary` `#646A73` | `--hs-text-secondary` | 亮 gray-500 `#617379` / 暗 `#8a8f98` |
| `--hs-text-placeholder` `#9CA3AF` | `--hs-text-placeholder` | 亮 gray-400 `#91a3a8` / 暗 `#5c616a` |
| `--hs-border` `#E5E6EB` | `--hs-border` | 亮 gray-200 `#e0e8ea` / 暗 `#2a2d33` |
| —(原无,原 lighter/extra-light 走 --el 派生) | `--hs-border-light`(新增) | 亮 gray-100 `#eff4f5` / 暗 `#23262b` |
| `--hs-bg-page` `#F7F8FA` | `--hs-bg-page` | 亮 gray-50 `#f7fafb` / 暗 `#0f1115`(hero panel 中性浅底同源) |
| `--hs-bg-card` `#FFFFFF` | `--hs-bg-card` | 亮 `#ffffff` / 暗 `#17191d` |
| —(原无) | `--hs-bg-hover`(新增) | 亮 gray-100 `#eff4f5` / 暗 `#1f2227` |
| `--hs-status-failing` `#FF4500` | `--hs-status-failing` | 亮 `#c2410c` / 暗 `#fb923c`(§3 裁决) |
| `--hs-text-xs/sm/md/lg/xl/2xl` 12/13/14/16/20/24 | 同名保留 | 不变;新增 `--hs-text-display` 28px |
| `--hs-radius` 6px | **撤销** | 卡片/弹窗 → `--hs-radius-lg` 8px;按钮/输入框 → `--hs-radius-sm` 4px |
| `--hs-radius-sm` 4px | `--hs-radius-sm` 4px | 不变(tag/徽标,扩为控件默认) |
| `--hs-radius-xs` 2px | `--hs-radius-xs` 2px | 不变(分段条专用) |
| —(原无) | `--hs-radius-full` 999px(新增) | 胶囊 chip |
| `--hs-shadow-card` 双段轻阴影 | **撤销** | 静态卡片改 `shadow="never"` + 1px `--hs-border` 描边 |
| `--hs-shadow-hover` `0 4px 12px rgba(31,35,41,.1)` | `--hs-shadow-md` | 亮 `0 2px 8px rgba(15,27,32,.08)` / 暗 `0 2px 12px rgba(0,0,0,.4)` |
| —(原无) | `--hs-shadow-sm` / `--hs-shadow-lg`(新增) | 刻度见 §2 |
| EP 功能色 success `#67C23A` / warning `#E6A23C` / danger `#F56C6C` | `--hs-success` / `--hs-warning` / `--hs-danger`(ep-theme.css 映射) | `#059669` / `#d97706` / `#dc2626`(亮暗同值) |
| 组件内 `--el-color-*`(含 light-9 浅底、dark-2 深阶)消费 | `--hs-*` / `--hs-*-soft` 语义令牌(ticket 73 收尾,~100 处) | 功能浅底亮主题 = 混白 90%、暗主题 = 混暗底 82%;dark-2 深阶直接用功能色本体(语义等价) |
| EP `--el-color-primary` 及预生成派生阶 | ep-theme.css color-mix 映射 | 派生阶不再预生成字面量,改 color-mix(亮混白/暗混黑) |
| `hs-blink` @keyframes | 同名保留 | 不变(全局唯一,semantics 层定义) |
| ECharts `CHART_COLORS`(TimeSeriesChart/TrendChart 各一份) | `utils/chartColors.ts` 单一来源,`CHART_COLORS_LIGHT/DARK` 双份 | 色值按 §3 新映射;brand→teal、failing→橙、文本/描边→青灰 |
| `web/public/logo.png` | **删除** | BrandMark.vue + Wordmark.vue 替代(AppHeader/LoginView/StatusCard/favicon) |

## 附录 B:裁决记录(13 项)

> **归属拆分(GH #59 定稿):** 视觉类裁决(2 字阶 / 3 圆角 / 5 BrandMark / 6 暗色 / 7 令牌架构 / 8 token 映射)规格本体已迁 DESIGN.md,此处保留裁决理由备查;11(LatencySparkline 形态)已迁 dashboard surface brief;语义与评估域裁决(1 failing 例外 / 4 批次运行色 / 9 榜单矩阵化 / 10 api-contract / 12 判分不完整 / 13 实时排名与成本)留存本文件。全部 13 项无丢失。

1. **failing 色选档:** 亮 orange-700 `#c2410c` / 暗 orange-400 `#fb923c`。理由:amber 加深(#b45309)与 warning 同族仅差一档明度,降级/告警并排不可分辨,且「更深的黄」不比红更紧急;orange 在黄红之间建立色相级第三档,亮档白底 5.1:1 过 AA。登记为「不引入新色相」纪律的唯一例外(告警辨识度=W5 告警可信度,收益大于纪律成本)。
2. **字阶:** 原六档平移 + 新增 `--hs-text-display` 28px;HealthBanner 大字结论与 StatusCard 可用率大数字升档(原 2xl 24px)。理由:消费页 3 秒场景的第一视觉锚点应由大数字承担,工具风层级靠字号表达;display 档禁用于管理台标题。
3. **圆角:** 撤销 6px 默认档,最终四档 2(分段条,存续)/ 4(控件)/ 8(卡片面板弹窗)/ 999(胶囊)。理由:对齐 ProxyHub 刻度,6px 无归属——控件偏松、卡片偏紧;分段条 2px 语义特殊(填满式时间条)保留。
4. **批次/运行状态色:** 运行中 = brand teal(亮 teal-600/暗 teal-500),随主色切换;角色 tag `primary` 同理零改动换 teal,`info` 映射 `--hs-info`(暗色提 gray-500)。理由:两域都引用语义令牌而非字面量,主色切换天然生效。
5. **BrandMark 差异化(2026-07-24 修订):** 初版裁决为与 ProxyHub 完全同构(同字形同渐变),理由为用户群不重叠、消歧靠字标;**交付后用户实机反馈「不合群」,修订为差异化字形**——HubScope 用瞄准镜字形(圆环 + 十字准星刻度 + 中心脉冲点,监控隐喻),与 ProxyHub 的 hub 辐条字形在图形层即区分;渐变与圆角方块保持同源(家族感由色板与容器承担,个性由字形承担)。BrandMark 永远与 Wordmark 同场出现(AppHeader/LoginView/StatusCard)的约束不变。
6. **暗色规范:** AppHeader 右栏图标按钮切换(未登录可用),`localStorage('hs:dark')` 持久化 + index.html 首屏防闪内联脚本;默认亮、不跟随系统(投屏/截图场景主题确定性优先);导出物料(StatusCard/分享报告导出)恒亮主题渲染;ECharts 镜像双份 + 主题切换重渲染。
7. **令牌架构:** 迁三层(tokens/semantics/ep-theme),保留 `--hs-` 前缀。理由:降低存量 diff 与 review 风险,多 Hub 产品生态下避免 `--ph-` 混读;层级与纪律完全对齐上游,前缀差异仅为命名空间。
8. **既有 token 映射:** 全表见附录 A;要点——hero panel 中性浅灰底 = `--hs-bg-page`(同源语义「页面级中性底」);`--hs-brand-soft` → teal-50(亮)/teal-900(暗);`--hs-shadow-card` 撤销改描边分层(工具风审美第 2 条);EP 派生阶从预生成字面量改 color-mix。
9. **榜单矩阵化(ticket 78/79,spec 0009):** 堆叠条退役、矩阵列式上榜。理由:① 段宽随分数变使固定标尺对齐成为结构性不可能(SuiteRuler 只对第一行准确,family tag 变宽致 bar 左缘漂移);② 五色分类色只表达「哪个维度」不表达强弱,且与功能色撞车(满分读作「危险/警告」);③ 档色(≥80/≥50 阈值不动)同时承担维度格子与总分条的强弱表达,0–100 恒刻度是 W7 绝对分制的视觉镜像。排序只降序(服务端语义,禁前端 reverse 第二口径);`--hs-suite-1..6` 与 `--hs-text-on-color` 随堆叠条一并退役。
10. **api-contract OverviewEntry 段顺手补齐(ticket #8,main 裁决):** 契约 OverviewEntry 段既存漂移(缺 `dots_24h`/`family`/`capability`/`score`/`score_reasons`/`eval_score` 等字段),本票在新增 `baseline_p50_ms` 与桶 `p50_ms` 文档时顺手把 OverviewEntry 段补齐到实现现状;响应级聚合字段(`by_*`/`enabled_endpoints`/`availability_24h`)仍另议,不在本票范围。
11. **LatencySparkline 形态迭代(ticket #8 后续,用户 2026-07-28 裁决):** 实机反馈「有点丑」(截图证据:P50 2.22s vs 阈值 ~7s,曲线贴底蠕动、阈值虚线喧宾夺主读作边框)。三改:① 量程从「0 锚定 + 阈值恒在」(max(阈值, 峰值)×1.1)改**数据量程驱动** max(峰值×1.25, 1000ms 下限)——曲线形态满幅可见,下限防亚秒抖动放大成伪形态;② 阈值虚线改**按需出现**:出现 ⟺ 阈值 ≤ yMax(构造性条件,等价峰值≥阈值×0.8,零魔幻常数;否掉 60% 触发常数——触发时阈值在 canvas 外,要么扩量程重新压扁曲线、要么画屏外线,均不成立),不出现零残余指示、tooltip 兜底、不加迟滞;③ 曲线加 `--hs-bg-hover` 实心浅填充(表面令牌类别正确、亮暗双值构造性成立、零新色相;否掉 text-secondary 叠 opacity——文字令牌当涂料是类别误用),填充随段断、孤立点不填充。**附记(同日):** 全失败桶 tooltip 措辞修正——p50 null 桶按桶事实二分(无探测→「无数据」;全失败→「探测全部失败,无延迟样本」),取代 ticket #8 空桶/全失败桶同词登记,无整卡级特判,占位轨道形态不变(§5 LatencySparkline 条目)。
12. **判分不完整呈现(ticket 92,spec 0014 决策 A,2026-07-28 设计评审):** ① 水印文案定稿「判分不完整,缺 N/M 维度」——「缺」字保留:裸「N/M」与 ScoreCell 覆盖率水印「·8/10」(分子=已判数)的既有分子口径冲突,会误读为「已判 N/M」;N = missing_suites(契约定稿即冻结,取代票正文「N=有分维度数」),M = missing_suites + 该行非 null suite_scores 数(行级自洽推导,门槛分母无现成字段;「判完后题库清空」边缘把已清空但有分的维度计入 M,与行内可见分数一致,恒有 N ≤ M)。② 水印位置取**模型列模型名之下第二行**,否掉票面「总分列 dash 之下」:语义主语是模型(判分不完整是模型的判定状态,dash 是后果);总分列 96px(页面)/64px(物料)下两至三行折行排版差且顶破 ticket 82「行高恒 46px」不变式,模型列 260/150px 单行可容(≈124px),行高保持 46px。③ 两模式(live 半成品 vs settle 不完整)区分不由名次形态承担(共享 `–` 占位,登记为允许复用),由三重构造性差异承担:半成品墨色数字 + 动态注 vs dash + 静态水印结论;涨跌列整列隐藏 vs 常显;live 行无 complete 键 / settle 行无 live-note,永不同时出现。④ rank-top 仪式感按 `complete !== false` 判定,否掉纯 index 判定(完整模型 <3 时 index<3 命中不完整行)。⑤ /board sortRows 镜像增补「complete===false 恒沉底 × 任意排序键」,`undefined` 视为完整与后端 rankable() 同口径——既有 null 沉底逻辑不充分(不完整行维度分非 null,按 suite 列排序会混入中游)。
13. **实时排名 + 行内展开详情 + 成本指标(GH #40/#41/#42,2026-07-29 设计评审,三票同区域一次评审):** ① **实时排序键 = 半成品总分(ADR-0005 归一化口径:已判维度加权平均,未判维度不进分子分母)**,否掉「含 null 维度按 0 计」——0 计口径让先判完低权重/简单维度的模型被系统性压低,且与后端既有 totalScore 函数双口径;弱化样式定稿 = sm/secondary 位次数字、无前 3 名仪式感(竖条/大字/徽章),null 总分沉底留 `–`;列头保持禁点——实时榜只有总分降序一个口径,禁列头切换制造第二排序口径(与 spec 0009「禁前端 reverse」同纪律)。② **未判分格子内联批次状态词**(进行中/等待中/失败,纯文字禁圆点)取代裸 `–`,解决「维度实时进度在分数视图不可见」——圆点 + 词是进度网格的形态,分数视图用文字避免第二状态灯观感,且不增行高(46px 不变式不破)。③ **行内展开否掉弹窗**(用户决策):feed 是流式监控面板,弹窗打断「盯着流」场景;详情按 result id 按需拉取 + 缓存,防轮询载荷膨胀;展开态 keyed by id,新条目前插不塌。期望答案块 label 随判定方式分叉(rule=「期望答案」/ judge=「评分要点」),与行内判定方式 tag 同口径。④ **成本指标中性化**:耗时/Token 是成本不是质量,禁档色挂钩、禁前端自定分界线;榜单矩阵不加成本列(列 x 恒定不变式 + 防质量暗示),明细走双 tooltip + 报告页独立明细表;成本数据控制台专属(运营数据,超 ticket 54「运行状态与覆盖」公开范围);汇总行形态(main 裁决 2026-07-29,评审后修订)=「判分耗时 X · 批次用时 Y · Token Z(输入 I / 输出 O)」——判分耗时 = Σ latency_ms(成本视角),批次用时 = wall-clock(started/finished 推导,running 取当前时刻),两者并列各司其职,取代评审初稿「总耗时 = 判分累计,非 wall-clock」的单一口径。

14. **warning 亮值对比度修订(2026-07-29,impeccable audit 发现 + plan 设计评审):** `--hs-warning` 亮值 `#d97706` → **`#a16207`**(yellow-700),暗值保留 `#d97706` 分档(tokens.css 新增 `--hs-warning-dark`,semantics.css `html.dark` 块覆盖)。理由:① 旧值白底实测仅 **3.19:1**(相对亮度公式实测,比 audit 估计的 3.9 更糟),小字场景(ScoreCell 黄档分数、降级状态词、组头计数)全不过 WCAG AA 4.5:1;② 新值 4.92:1 有余量且是具名刻度档(非刻度外孤立值);③ **可分辨性不为 AA 让路**:否掉 amber-700(历史候选值,对 failing ΔE 仅 6.9、色相仅差 9°——精确复现附录 B 第 1 项的邻近问题),选 yellow-700 使色相角远离 failing(ΔE 16.3 接近现状 18.4,远高于已登记先例 failing-danger 的 9.6),消歧从明度单通道升级为色相+明度双通道;④ 暗值保留是因暗底实测 5.93/5.52 全达标,改动无收益;暗值升级 yellow-500(ΔE 升至 18.7)登记为后续可选。连带:chartColors LIGHT 镜像同步,Vue 组件零改动(全消费令牌),分段条黄格(图形 3:1 门槛)从压线 3.19 升至 4.92 纯改善。附记:success `#059669` 白底实测 3.77 同类不过 AA,由「success 文字场景用 dark-2 阶」既有条款兜底,同等处置另立票。

## 附录 C:迁移验收 checklist(供品牌迁移 ticket 引用)

**令牌与主题基建**
- [ ] `web/src/styles/` 三层文件就位:tokens.css(原始刻度)/ semantics.css(语义令牌 + `html.dark` 块)/ ep-theme.css(EP 映射,亮暗两块,color-mix 派生)
- [ ] main.ts 引入顺序:element-plus/index.css → element-plus dark/css-vars.css → tokens → semantics → ep-theme → print
- [ ] 页面/组件 scoped style 零硬编码色值(grep `#` 十六进制 + rgb/rgba 字面量,仅允许 utils/chartColors.ts 镜像与 BrandMark 渐变 stop 引用原始刻度)
- [ ] 页面零 `--el-*` 书写、零刻度外 z-index 字面量
- [ ] `hs-blink` 仍全局唯一定义,StatusBadge/HealthBanner 复用

**暗色**
- [ ] AppHeader 主题切换按钮(未登录可用),`hs:dark` 持久化,index.html 防闪脚本键名一致
- [ ] 暗色下全页面抽查:状态板/榜单/报告/管理台/登录页,无亮底残留、无白闪
- [ ] 暗色下 StatusBadge 四态、EvalProgressGrid 四态、24h 分段条三档可分辨
- [ ] 暗色下 ECharts 图表(TimeSeriesChart/TrendChart/ModelTrendDialog)取 DARK 镜像,主题切换即重渲染
- [ ] StatusCard PNG 导出与分享报告页在暗色会话下仍产出亮主题物料

**语义色与组件**
- [ ] StatusBadge 四态色 = success/warning/danger/failing 新值,告警闪烁保留
- [ ] HealthBanner 四态 + 大字结论 display 档;仅异常态可点行为不变
- [ ] 24h 分段条三档 + 无数据灰,格形不变(radius-xs 2px 分段填满式)
- [ ] 榜单条形档位色、涨跌箭头、断点不显示箭头,口径不变
- [ ] EvalProgressGrid 运行中 = teal,禁 warning 黄、禁闪烁
- [ ] 角色 tag 四词表 + primary(teal)/info 映射,AppHeader/AdminView/UserManager 三处一致
- [ ] StatusCard 全部批 56/59 条款回归:范围 chips、hero panel 构成(中性灰底/display 大数字/verdict/分布串)、分段条、三段式明细行、正常汇总行、一句话总结、页脚;空态不读作「全部正常」
- [ ] 分享报告页运行中信息边界(ticket 54)不变;settle 转场轮询口径不变

**品牌标识**
- [ ] BrandMark.vue/Wordmark.vue 就位,AppHeader/LoginView/StatusCard 接入,favicon 重生成
- [ ] `web/public/logo.png` 删除,全仓无残留引用;logo.svg/favicon.png 一并替换
- [ ] Wordmark 完全静止(无动画);字体为系统等宽字栈,全仓无外部字体文件引入(无 woff2/Google Fonts 引用)

**圆角/阴影/密度**
- [ ] 卡片/弹窗 radius-lg 8px、控件 radius-sm 4px、分段条 radius-xs 2px;无 6px 残留
- [ ] 静态卡片 `shadow="never"` + 描边;阴影只在可点 hover 与浮层
- [ ] 消费页 16px / 管理台 12px 密度档不变

**文档**
- [ ] 本草案替换 `.claude/rules/ui-guidelines.md` 本体
- [ ] spec 0003 相关章节标注「已被品牌并入 supersede」或同步更新
