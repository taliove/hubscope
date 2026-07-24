# UI Guidelines(设计规范)

> HubScope 前端唯一设计规范,由 `design-owner` 代理维护。`frontend-checker` 的 UI 自查以本文件为依据。修改本文件的语义映射(状态色、词表)属于承重语义变更,须在设计评审中说明理由。
> 视觉基线(ticket 73)并入 ProxyHub「现代极简工具风」(电波青 teal 品牌色、三层令牌、暗色一等公民);HubScope 词表、状态语义、防作假约定全部保留,仅做色值与刻度映射。

## 1. 产品形态与读者

- **双形态:** 公开状态板(Dashboard、EndpointDetail,无需登录)+ 管理台(EvalCenter、TaskCenter、Admin,需登录)。
- **两类读者:** 状态板读者要「3 秒看懂健不健康」——状态优先,操作入口让位;管理台读者要「高效完成配置与排查」——信息密度优先,操作直达。
- **桌面优先:** 内容区 `max-width: 1200px` 居中(Dashboard 先例),不为手机做专门适配,窄屏不阻断使用即可。
- **全站工具风:** 并入 ProxyHub「现代极简工具风」审美——灰阶为主、用色克制、轻阴影靠描边分层、无多余装饰(无渐变装饰、无大圆角、无彩色背景块)。公开页(状态板、登录页)同为工具风基调,不做营销装饰(不引入光斑背景、插画等营销物料);BrandMark 渐变是唯一允许的渐变(图形标识,非装饰)。

## 2. 视觉基线

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
| `--hs-bg-hover` | gray-100 `#eff4f5` | `#1f2227` | 行/菜单悬浮 |
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

- **暗色一等公民:** 暗色不是反色滤镜,表面/文本/描边/阴影/主色均有独立暗色取值(§2 语义令牌表);暗色只覆盖 semantics.css 的 `html.dark` 块,页面代码零改动——页面只消费语义令牌是硬约束。
- **切换入口:** AppHeader 右栏操作区(批次进度入口左侧)放亮/暗切换按钮(link 型图标按钮,Sun/Moon 图标),未登录态同样可用(状态板读者无差别)。当前主题下只显示目标态图标(亮主题显示 Moon)。
- **持久化与防闪:** 选择存 `localStorage` 键 `hs:dark`(1/0);`index.html` 内联首屏防闪脚本在挂载前同步读该键加/去 `html.dark` class(键名两处保持一致,注释互指)。**默认亮主题、不跟随系统偏好**(v1 裁决:二态切换语义最简单,公开状态板常被投屏/截图,主题确定性优先;系统偏好跟随留作后续增强,届时改三态「跟随系统/亮/暗」需设计评审登记)。
- **导出物料固定亮主题:** StatusCard(PNG/PDF)、分享报告页导出一律强制亮主题渲染——物料是对外传播的静态快照,亮主题保证打印/转发可读性;实现上导出画布渲染前临时去 `html.dark` class 或在离屏容器内以亮主题令牌渲染,禁止把暗色像素烤进物料。
- **ECharts 暗色:** JS 镜像色板亮暗双份(见 §3 ECharts 条目),按当前主题取份;主题切换时图表重渲染(watch 主题状态,setOption 全量替换)。
- **暗色验收:** 暗色下功能色文字场景(表格内着色文字、明细行状态词)需抽查对比度;failing 闪烁动画暗色下不调整(闪烁是语义,非装饰)。

## 2b. 品牌标识(BrandMark / Wordmark)

- **BrandMark 是唯一图形标:** 共享组件 `components/BrandMark.vue`,64×64 viewBox 内联 SVG——圆角 rect(rx=14)填 teal-400→teal-700 渐变(渐变 stop 消费 `--hs-teal-*` 原始刻度,此为页面消费原始刻度的唯一豁免:图形标识非语义表达),白色**瞄准镜字形**(圆环 + 十字准星刻度 + 中心脉冲点——监控隐喻:盯住 Hub 上每个 endpoint;ticket 73 后续修订,取代初版与 ProxyHub 同构的 hub 辐条字形,因用户反馈「不合群」——与 ProxyHub 的区分由图形标承担,不再只靠字标);em 尺寸宿主可控(`width/height: 1em`)。favicon 与 AppHeader 左侧品牌位同源;`web/public/logo.png` 已删除,AppHeader/LoginView/StatusCard 一律使用 BrandMark。
- **Wordmark 是唯一字标:** 共享组件 `components/Wordmark.vue`——「HubScope」PascalCase 文本 + 主色**脉冲小圆点**(常亮,与 BrandMark 瞄准镜中心点同源的图形呼应),字体用**系统等宽字栈**(`ui-monospace, "SF Mono", "Cascadia Mono", Consolas, monospace`),不引入任何外部字体文件(零依赖、离线可用,与单二进制交付一致),字重 700;字号以 em 为基准,宿主用 `font-size` 覆盖等比缩放。使用点:AppHeader 品牌位(BrandMark + Wordmark 横排,默认 `--hs-text-lg` 16px)、LoginView 品牌区(`--hs-text-display` 28px)、StatusCard 品牌区(随物料画布定字号,不用令牌字面量)。**字标完全静止(2026-07-24 修订,取代初版闪烁终端光标):「闪烁=failing 告警专属」是 W5 承重语义,字标作为全站唯一持续运动元素挂在每页左上角,用户对「有东西在闪」的直觉解读就是「有情况」——豁免条款救得了规则救不了直觉;脉冲点常亮后,failing 重新独占全站唯一动画,无任何豁免。**
- **LoginView 品牌区构成:** BrandMark(40px)+ Wordmark(display 档)横排居中,替代原 logo.png 40px 图块;登录卡保持工具风,不加装饰背景。

## 3. 语义色映射(核心约定)

颜色承载业务语义,**映射关系只在本文件定义**,组件引用同一映射,禁止各组件自造颜色语义:

| 业务语义 | 颜色 | 说明 |
|---|---|---|
| healthy 正常 | success `#059669` | endpoint 状态 |
| degraded 降级 | warning `#d97706` | endpoint 状态 |
| down 宕机 | danger `#dc2626` | endpoint 状态 |
| failing 告警 | 亮 orange-700 `#c2410c` / 暗 orange-400 `#fb923c` + 闪烁 | 比 down 更紧急,唯一允许动画的状态(见 StatusBadge) |
| 评分/百分比档位 | 绿/黄/红阈值 | 阈值以后端口径为准,前端不自定分界线 |

- **failing 色选档裁决(本版登记):** failing 语义是「比宕机更紧急、需立即处理」,现行橙红 #FF4500 的「最刺眼」地位必须保留。ProxyHub functional 四色中 warning=#d97706(amber-600)与 danger=#dc2626(red-600)之间没有足够的同系加深空间——amber-700 #b45309 与 warning 同族,并排时(HealthBanner、状态板状态列、图例)仅明度差一档,降级/告警相邻场景辨识度不足,且「更深的黄」直觉上并不比「更红的红」更紧急。故选 **orange 系**:亮主题 orange-700 **#c2410c**(白底对比度 ≈5.1:1,过 WCAG AA,可承载文字;与 warning 差一个色相族、与 danger 明度饱和度接近而色相向橙偏移,在红黄之间建立「第三紧急档」),暗主题 **orange-400 #fb923c**(暗底对比度 ≈7:1,暗色下功能色惯例提亮)。**这是「调色板外不引入新色相」纪律的唯一例外,在本条登记**:告警是 HubScope 监控域的最高紧急度语义,amber 加深无法满足「与 warning/danger 三档可分辨」的业务硬需求,例外收益(告警可辨识度=告警可信度,W5 语义)大于纪律成本;除本例外外禁止任何新色相。
- ECharts 系列色从上述调色板取,正文/轴文字用 `--hs-text-primary`/`--hs-text-secondary` 等值(图表内走 JS 镜像 const,**亮暗双份**:`CHART_COLORS_LIGHT` / `CHART_COLORS_DARK`,与 semantics.css 双值逐一同步,按当前主题取份;主题切换时图表 setOption 重渲染),不引入调色板外的新色相(failing 橙为例外登记,同入镜像)。两份镜像现状(TimeSeriesChart/TrendChart 各一份)在迁移期合并为单一来源(如 `utils/chartColors.ts`),两个图表组件复用。
- 同一语义在状态板与管理台必须同色同词。
- **分数涨跌指示:** 升=success 绿、降=danger 红,与「分数高=好」同向;持平、无上批次、**跨 Suite 版本断点(分数不可比)一律不显示涨跌箭头**,用 `--hs-text-placeholder` 占位并标注「题目已变更」(忠实 ADR 0007:禁止把题库变化呈现为模型降级,这是本产品的防作假核心语义)。
- **榜单条形(0–100):** 按评分档位阈值着色(绿/黄/红,阈值以后端口径为准),与得分徽标同一映射,不为榜单另定分界线;涨跌箭头是独立维度,不替代条形档位色。
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

- 页面结构:顶部标题/操作区 → 内容区(卡片或表格)→ 必要时底部辅助区。
- 内容容器首选 `el-card`;列表数据首选 `el-table`;管理页多功能分区用 `el-tabs`(**不加 `lazy`**,保轮询事件,ticket 19)。
- 详情抽屉/弹窗:`el-dialog` 用于需要聚焦的详情与表单(见 EvalRunDetailDialog),不在页面内嵌套多层展开区。
- 卡片内容**不得溢出、不得出现横向滚动条**;弹性列宽优先于固定宽度(历史 bug:24h 小点)。

## 5. 组件使用规范

- **Element Plus 组件优先**,不引入新 UI 库,不自造表单、弹窗、表格、分页。
- **StatusBadge 是唯一的状态展示组件**,需要展示 endpoint 状态处一律复用,禁止第二个状态灯实现。
- **HealthBanner 是 Dashboard 的全局健康横幅组件**(批 2 登记):四态(全部正常/N 个端点降级/N 个端点异常含告警闪烁/加载 skeleton),数据只反映全局、永不受页面过滤器影响;仅异常态可点(应用状态过滤并滚动定位)。其他页面不得复刻其结论文案模式。大字结论用 `--hs-text-display` 档(§2 字阶)。
- **Leaderboard 是评估榜单的唯一排行展示组件**(/eval 与 /report/{token} 复用):每模型一行(排名/模型名/条形/总分/涨跌箭头),模型名截断 + `title` hover 全显;Suite 切换、family 过滤、排序切换走榜单上方工具条,不进单元格;行下钻不内嵌行内展开,走 `el-dialog`(§4 既有约定,EvalRunDetailDialog 模式)。
- **Leaderboard 运行中半成品模式**(ticket 52,修订上一条登记):未完成批次下榜单可查看但——① 名次列显示 `–` 占位(`--hs-text-placeholder`),禁名次徽章;② 行序固定模型名字典序(后端保证,前端不重排),工具条禁用排序切换与 Suite 切换、保留 family 过滤;③ 涨跌箭头列整列隐藏;④ 覆盖率不满的分数带 Coverage 水印「X/Y 题」(`--hs-text-xs` + `--hs-text-secondary`,常规字重,不用颜色区分),覆盖率满不显示;⑤ 未跑能力点显示「进行中」占位(`--hs-text-placeholder`)且不计入总分;⑥ settle 后名次直接替换占位、不做强调动画,转场提示由 ElMessage 承载。
- **Leaderboard 维度分同屏条带与置信标记**(ticket 51 登记,复用 ticket 52 条带形态,settled 与 live 两模式同构):每模型行下方固定渲染能力点条带(能力点名/小条形/分数),总分不再独占一行;未判分能力点显示 `-` 占位且不计入总分。置信标记两件套:覆盖率水印「X/Y 题」沿用 ticket 52 样式(覆盖率满不显示),采样数不新增视觉元素、走条带项 `title` hover(「判分 X/Y 题 · 采样 N 次」);基准不可比文案三分支:题目已变更(suite_changed)/判分口径已变更(profile_changed)/考核口径不同(suite_missing),一律不显涨跌箭头。
- **EvalProgressGrid 是批次进度矩阵的唯一组件**(ticket 52 登记):模型 × 能力点状态矩阵,运行中/等待中批次的默认视图;首列模型名截断 + `title` hover,能力点列 flex 等宽,行随模型数纵向滚动,**不开横向滚动豁免**(§4 无特例);单元格四态用 §3 批次/运行状态色映射,纯展示不可点;批次级进度汇总置于网格上方同卡片内;「进度网格 / 实时分数」视图切换走卡片顶部 el-radio-group。**分享页只读模式(ticket 54 登记):** /report/{token} 运行中批次复用本组件,但不渲染视图切换(组件须支持隐藏切换的只读用法),网格是分享面运行中的唯一视图。
- **分享报告页运行中信息边界(ticket 54 登记,HIGH-1 口径的细化):** 分享面(/report/{token})未完成批次只公开**运行状态与判分覆盖**(模型 × 能力点四态 + X/Y 题),不公开任何分数、名次、涨跌——「实时分数」为登录控制台专属,分享页不提供切换入口;批次 settle 后分享面照常渲染完整榜单(既有行为不变)。依据:状态/覆盖率是运行元数据而非评估结论,不构成 spec 0004 所防的「半成品分数外流」;模型名单 settle 后本就公开;token 高熵、可撤销、走审计(ADR 0006 控制面不变)。已知并接受的增量:运行中分享面可见 per-model 失败归因(settle 后分享榜单从不点名失败模型)——失败是运行事实,不掩盖;该暴露瞬时、token 可控。分享页不新增任何依赖会话接口的交互入口。
- **TrendChart 是趋势类折线图的唯一通用组件**(批 32 登记):裸图表(不带卡片,布局由父级负责),默认在 null 点断线(未判分批次不得连成假分),支持竖向断点标注线(占位灰虚线,如「v2 起题目变更」);ECharts 色板 JS 镜像与 TimeSeriesChart 同一来源(§3 ECharts 条目,迁移期合并)。
- **ModelTrendDialog 是报告页行下钻的唯一趋势弹窗**(批 32 登记,ticket 51 修订):按模型按需拉取 `/api/campaigns/{id}/trends`,分数线(版本断点标注「vN 起题目变更」+ 判分口径断点标注「判分口径已变更」,同一位置双断点合并为一行标注,均复用 TrendChart 灰虚线断点机制)+ 探测成功率/延迟并列;已删除模型带「已删除」tag;加载/空/错误三态齐全。
- **AppHeader 导航按登录态过滤:** 未登录只渲染公开页导航项(状态总览)+ 登录按钮,不渲染会被路由门禁弹走的项(评估、任务、管理入口);登录态随路由切换重检(沿用 refreshAuth watch 先例),不写死 mount 时一次性判断。
- **StatusCard 是状态分享卡的唯一渲染模板**(批 56 登记,批 59 重设计构成):720px 逻辑宽、2x 导出的竖版品牌物料,自上而下固定构成——① 品牌区(`--hs-brand` 4px 品牌条 + `--hs-brand-soft` 浅底 + BrandMark + Wordmark + 「服务状态」`--hs-text-2xl`/600——「HubScope」字样由 Wordmark 承担,标题词不再重复品牌名);② 范围行(无筛选纯文本「全部端点」,有筛选逐项 chips:描边 + `--hs-radius-sm`,前缀灰 label + 值,状态 chip 值用语义色;分组卡首位恒为分组 chip,与筛选 chips 并存,一个不漏);③ hero panel(可用率优先,批 59 第二轮迭代,替代原「tone-tinted 结论块 + 独立指标行」两区块;用户反馈顶部太告警化、优先展示异常端点):结论与指标合并为单一 panel——`--hs-bg-page` **中性浅灰底(无 tone tint)** + `--hs-radius-lg` + padding 16px 20px,左右两列 + 1px `--hs-border` 竖分隔。左列自上而下:「24h 可用率」`--hs-text-xs` 标签 → `--hs-text-display`/600 大数字按 §3 三档着色 + 小号次级「%」(`--hs-text-md` secondary)——可用率大数字当主标题,其三档着色承担严重度信号,顶部不再用告警化色块;其下 verdict 文案(与 HealthBanner 同源结论词,如「5 个端点降级」,tone 着色但 `--hs-text-sm` 次级)+ failing 静态双编码(橙实心点 + 「含 N 个告警」橙描边 chip);再下**完整分布串**「正常 N · 降级 N · 宕机 N · 告警 N」四段恒列,`--hs-text-xs`,非零段状态词语义色/600 + 数字 `--hs-text-primary`,零计数段整段 `--hs-text-placeholder`。右列:「平均延迟」同构标签 + `--hs-text-xl`/600 数字,`--hs-text-primary` 不着色。**防作假不变:verdict 与四段恒列分布串仍在,异常不掩盖——只是不再当头条、不再有 tone tint 色块;空态(tone-empty)可用率渲染 `-` + 「24h 内无探测数据」,verdict 与分布串均不渲染,中性灰底保证「无数据」永不读作「全部正常」。** null 延迟 → `-` placeholder + 「24h 内无探测数据」`--hs-text-xs`;④ 24h 分段可用率条(组内聚合,口径见 §3 三档条目,24 格分段填满式、格高 16px、`--hs-radius-xs`、2px 间距;条下两端「24 小时前」「现在」`--hs-text-xs` placeholder;聚合函数抽 utils 纯函数,禁按端点简单平均);⑤ 异常明细(封顶 10 条,严重度排序 告警>宕机>降级,**三段式行**:行 1 = 状态词语义色 `--hs-text-sm`/600 + 「模型 · 协议」`--hs-text-md` 截断 + 右侧单端点 24h 可用率 `--hs-text-sm` 同档着色(null → `-`),行 2 = status_reason `--hs-text-xs` secondary、最多两行截断,reason 为空则不渲染行 2,**行 3 = 单端点 24h 打点条**——全宽 24 格分段填满式、格高 8px、`--hs-radius-xs`、2px 间距、**无轴标**,左缩进与 reason 对齐(margin-left 40px),复用 §3 三档着色与 dotTier;打点条让维护受众一眼看出故障时段——「最近 1 小时炸的」vs「全天半死」,单点可用率数字做不到这个;行间 hairline 分隔;overflow 收尾「另有 N 个异常端点未列出,详见状态板」);⑥ 正常端点汇总行(「其余 N 个端点正常 · 24h 可用率区间 min%–max%」,「正常」success 色、余文 secondary;min==max 显示单值,全部 null 则附「(24h 内无探测数据)」;全正常态此行替代异常明细,措辞「全部 N 个端点正常」);⑦ 一句话总结(见下方独立条目);⑧ 页脚(hairline 分隔,左「生成于 YYYY-MM-DD HH:mm」+「另有 N 个已停用」`--hs-text-xs` placeholder,右 location.origin)。空态沿用批 56:零匹配/全停用时范围 chips 保留、hero panel 中性灰底 + 可用率 `-`(verdict 与分布串不渲染,永不读作「全部正常」)、分段条全占位、明细「暂无匹配的 Endpoint / N 个端点均已停用」、不渲染总结与正常汇总行。结论判定仍与 HealthBanner 同源,统计集合=快照范围;明细状态词按 §3 静态物料约定用着色文字,不复制状态灯实现。**导出画布恒亮主题(§2a),物料内令牌引用亮主题取值。**
- **分组独立分享入口(批 59 登记):** OverviewGroupSection 标题行右端(group-metrics 之后)放 text 型 `el-button`(Share 图标 + 「分享」文字,不用裸图标按钮——状态板读者 3 秒场景下图标语义不够直白),`@click.stop` 拦截冒泡、不触发整行折叠,hover 走 Element Plus text 按钮品牌色反馈;点击复用 StatusShareDialog(弹窗本体不变),快照范围 = 该分组条目 ∩ 当前页面筛选,scope chips 首位恒为分组 chip(label「分组」,值「厂商/能力/协议 · 组名」,维度词表 family→厂商、capability→能力、protocol→协议),其后列全部生效筛选条件。**卡片所有数字一律从快照 entries(enabled)计算,与范围 chips 恒一致(批 59 口径修订):筛选快照不得引用 OverviewGroup/Overview 的未筛选聚合字段,否则数字描述全集、chips 声明子集,自相矛盾,违反批 56 防作假约定;且 Overview 全局无 avg_latency_ms 字段,透传路径本就不完整。** 两个标量口径:① 24h 可用率 = 快照内 enabled entries 的 dots_24h 按小时求和 ok/total(探测加权,与 `internal/server/overview.go` groupAccumulator 同定义,无筛选时与后端数字构造性相等,口径见 §3 批 59 条目);② 平均延迟 = enabled entries 的 p50_ms 均值(前端无法从 dots 复现探测加权延迟,这是唯一 scope 恒一致的口径;已知代价:与分组标题行「均延」的探测加权 mean latency 数值可能略有差异——卡片内部自洽优先于与页面逐字相等);StatusCardSnapshot 只扩展 `group` 字段,不携带任何聚合标量(statusCardSnapshot.ts)。Dashboard 全局「分享状态」入口保留现状(筛选行主按钮,不动)。
- **StatusCard 一句话总结(批 59 登记):** 位置=明细区与页脚之间,hairline 分隔后;形态=「小结」前缀标签(`--hs-text-xs` placeholder)+ 一句话(`--hs-text-sm` secondary、常规字重,无底色无边框无语义色)——视觉权重明确次于 hero panel 主结论区,句式以行动建议动词(建议/无需)收尾,与 verdict 的陈述句式区分。生成规则抽纯函数(utils/statusCardSummary.ts),优先级自上而下命中即止:① 有告警 →「有 N 个端点触发告警,建议立即处理」;② 有宕机 →「N 个端点宕机,建议优先排查 {首个宕机模型}」;③ 有降级且存在连续非绿时段 →「{模型} 持续降级约 N 小时,建议排查上游」(N = 该端点 dots_24h 自最新向前连续「有探测且非绿」格数——只有 fail/partial 计入,无数据灰格与绿格一样中断计时;取全组最长者。**持续时长必须有连续证据支撑**:灰格计入会让稀疏数据端点一路数到窗口起点、恒输出「约 24 小时」,属无证据的时长宣称,违反本条「不掩盖异常」的对偶约束——不夸大异常,对维护受众是狼来了);④ 有降级无持续信号 →「N 个端点降级,建议关注,暂不紧急」;⑤ 全正常但 24h 可用率 <95% →「状态全部正常,但 24h 可用率仅 X%,建议持续观察」;⑥ 全正常且有数据 →「近 24 小时运行平稳,无需处理」;⑦ 无 24h 数据 → 在命中句后追加「暂无 24 小时探测数据」。空态(零匹配/全停用)不渲染总结行。**总结不得掩盖异常:** 只要存在非正常端点,总结首句必须指向最严重的异常(告警>宕机>降级),禁止输出「运行平稳」类措辞——这是批 56 防作假约定在文案层的镜像。
- **结论必须标注统计范围(防作假约定,批 56):** 任何呈现汇总结论的导出/分享物料,结论旁必须显式列出统计范围——无筛选标「全部端点」,有筛选逐项列出全部生效条件(一个不漏),零匹配时范围仍需保留且结论用中性「暂无数据」;禁止把局部集合呈现为全局结论,禁止零匹配显示「全部正常」(ADR 0007 防作假语义在状态板域的镜像)。批 59 补充:分组也是范围,分组卡必须带分组 chip(见上条),分组卡片不得省略分组维度词。
- **AppHeader 批次进度入口**(ticket 52 登记):仅登录态渲染(与导航同一过滤时机),位于 header-right 操作按钮左侧;文案「批次运行中 X/Y」(X/Y 口径与榜单进度一致),点击跳 /eval;发现靠 mount + 路由切换重检(refreshAuth watch 先例),仅存在未完成批次时 3s 轮询,settle 即停并隐藏,卸载必清理;禁用橙(failing 专属)与闪烁(闪烁为 failing 告警专属)。
- **角色 tag 是用户身份角色的唯一展示单元**(ticket 62 登记):AppHeader 右栏当前用户、AdminView 用户列表、UserManager 一律复用,禁止各处自造角色色。词表固定四词,不新增同义词:super_admin→「超级管理员」、admin→「管理员」、operator→「操作员」、viewer→「观察者」(与 spec 0005 角色定义一致)。**角色语义是「权限层级」非「健康度」**,禁用 §3 的 success/warning/danger 状态色——红黄绿是 endpoint 与批次健康信号,状态板读者眼里红=异常、绿=正常,把 super_admin 染红会读作「告警」、operator 染绿会读作「健康」,属语义错位(本映射经设计评审登记,与 §3 两套状态色并列、语义域不混用)。
- **角色色映射:**

| 角色 | el-tag type | 语义 |
|---|---|---|
| super_admin / admin | `primary`(brand teal) | 管理权(全局或 Hub 内全权) |
| operator / viewer | `info`(中性灰) | 非管理(操作 / 只读) |

  管理权=brand 与 §2「品牌主色=主按钮/链接/当前导航/聚焦态」同向(强调=可支配),非管理=info 中性灰。super_admin 与 admin 同色:区分靠词表(「超级管理员」vs「管理员」)+ 数据域(全局不绑 Hub vs Hub 内),不靠颜色加权——super_admin 稀有但染红代价(告警串扰)高于收益(视觉强调)。与 §3「等待中 中性灰」同为中性语义但词表与语义域不同(角色 vs 批次等待),靠上下文与词表消歧,禁止把等待中灰借用到角色域。并入说明:`primary` type 随主色从蓝 #3B5BFD 切换为 teal(亮 teal-600/暗 teal-500),`info` type 映射到 `--hs-info`,组件零改动,仅 ep-theme.css 映射值变化。
- **实现集中:** `roleLabel(role)` + `roleTagType(role)` 抽 `web/src/utils/role.ts` 纯函数,供 AppHeader / AdminView / UserManager 复用(口径同 `utils/format.ts` 的集中原则);el-tag 走 `type` 属性语义色、`size="small"`、默认 `effect="light"`,不自着色、不硬编码色值、不引入调色板外色相;未知/未登录角色回落 `info` + 「未知用户」占位(三态精神)。未登录态不渲染角色 tag(与「AppHeader 导航按登录态过滤」同时机)。
- 反馈三件套:
  - 操作结果 → `ElMessage`(成功/失败/警告,见 HubManager 用法);
  - 破坏性操作(删除、禁用)→ `ElMessageBox.confirm` 二次确认;
  - 表单校验失败 → 表单内联提示,不用弹窗。
- 新组件若为通用展示单元(非业务组合),放 `components/` 并在本文件登记;业务一次性组件就近放视图内。

## 6. 交互规范

- **三态必备:** 加载态(skeleton 或 loading)、空态(空数据说明 + 引导操作)、错误态(错误原因 + 重试入口),任何数据区块缺一不可。
- **长文本:** 模型名/Hub 名/错误信息一律截断 + `title` hover 全显。
- **轮询:** `setInterval` 必配对清理(组件卸载时);可点汇总卡有反馈态且可再点取消(fix fc8bdb6)。
- **即时反馈:** 点击类操作在请求期间给 loading 或禁用态,不静默等待。
- **榜单/报告类消费页三态与轮询:** 批次切换器空态(无任何批次 → 空态 + 引导文案)、榜单空态、下钻趋势加载态缺一不可;选中等待中/运行中批次时榜单区呈现进度态(进度 + 批次状态词),不显示半成品名次,失败批次给错误态 + 原因;仅当选中未完成批次时才轮询进度,完成后停轮询并刷新榜单,卸载必清理。
- **导出物料的复制降级(批 56):** 「复制图片」依赖 `navigator.clipboard.write`,非安全上下文(HTTP 裸 IP)必须置灰 + 提示降级路径(「当前环境不支持复制图片,请使用下载」);下载能力不得受安全上下文影响,永远可用。
- **批次 settle 转场轮询口径(ticket 52):** 观察到 done/failed 的那次轮询响应即停轮询,渲染以该次数据为准,不再补拉;完成提示走 ElMessage(带批次号与原因,成功用 success、失败用 warning)。同一口径适用于所有渲染未完成批次的消费页:/eval、/campaigns/{id} 报告页与 /report/{token} 分享页(ticket 54),三处转场提示文案与轮询停止语义一致。

## 7. 文案规范

- 界面一律简体中文;**状态词表分两套,互不混用:** endpoint 状态=**正常 / 降级 / 宕机 / 告警**(与 StatusBadge LABELS 一致,不新增同义词);批次/运行状态=**等待中 / 运行中 / 已完成 / 失败**(沿用 campaignStatusLabel 既有口径)。禁止把一套词借用到另一语义域(如批次失败不得称「宕机」)。
- **分数展示统一 0–100 整数**(null → `-`),`formatScore` 集中于 `utils/format.ts`,组件内禁止自写 `toFixed` 分数格式;0~1 原始分只存在于 API 层。
- 按钮用动词短语(「触发同步」「新建 Hub」),不用「确定/提交」以外的泛词;错误消息必须带原因,不只说「失败」。
- 数字与时间格式统一走 `utils/format.ts`,不在组件内各写格式化。

## 8. 规范的维护

- 本文件由 design-owner 代理维护;设计评审中做出的新约定须回写本文件,否则视为未约定。
- 本文件与 [load-bearing-walls.md](./load-bearing-walls.md) 的关系:本文件管「体验一致性」,承重墙管「系统语义」;冲突时以承重墙为准(如状态机红黄绿语义由 W5 决定,本文件只做视觉映射)。
- 与 ProxyHub 上游规范的关系:令牌刻度与审美纪律源自 proxyhub `docs/design-frontend.md`;HubScope 侧业务语义(状态词表、防作假约定、failing 例外、导出物料)以本文件为准,不回写上游;上游刻度修订时由 design-owner 评估同步。

---

## 附录 A:旧 token → 新 token 完整映射表

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

## 附录 B:裁决记录(8 项)

1. **failing 色选档:** 亮 orange-700 `#c2410c` / 暗 orange-400 `#fb923c`。理由:amber 加深(#b45309)与 warning 同族仅差一档明度,降级/告警并排不可分辨,且「更深的黄」不比红更紧急;orange 在黄红之间建立色相级第三档,亮档白底 5.1:1 过 AA。登记为「不引入新色相」纪律的唯一例外(告警辨识度=W5 告警可信度,收益大于纪律成本)。
2. **字阶:** 原六档平移 + 新增 `--hs-text-display` 28px;HealthBanner 大字结论与 StatusCard 可用率大数字升档(原 2xl 24px)。理由:消费页 3 秒场景的第一视觉锚点应由大数字承担,工具风层级靠字号表达;display 档禁用于管理台标题。
3. **圆角:** 撤销 6px 默认档,最终四档 2(分段条,存续)/ 4(控件)/ 8(卡片面板弹窗)/ 999(胶囊)。理由:对齐 ProxyHub 刻度,6px 无归属——控件偏松、卡片偏紧;分段条 2px 语义特殊(填满式时间条)保留。
4. **批次/运行状态色:** 运行中 = brand teal(亮 teal-600/暗 teal-500),随主色切换;角色 tag `primary` 同理零改动换 teal,`info` 映射 `--hs-info`(暗色提 gray-500)。理由:两域都引用语义令牌而非字面量,主色切换天然生效。
5. **BrandMark 差异化(2026-07-24 修订):** 初版裁决为与 ProxyHub 完全同构(同字形同渐变),理由为用户群不重叠、消歧靠字标;**交付后用户实机反馈「不合群」,修订为差异化字形**——HubScope 用瞄准镜字形(圆环 + 十字准星刻度 + 中心脉冲点,监控隐喻),与 ProxyHub 的 hub 辐条字形在图形层即区分;渐变与圆角方块保持同源(家族感由色板与容器承担,个性由字形承担)。BrandMark 永远与 Wordmark 同场出现(AppHeader/LoginView/StatusCard)的约束不变。
6. **暗色规范:** AppHeader 右栏图标按钮切换(未登录可用),`localStorage('hs:dark')` 持久化 + index.html 首屏防闪内联脚本;默认亮、不跟随系统(投屏/截图场景主题确定性优先);导出物料(StatusCard/分享报告导出)恒亮主题渲染;ECharts 镜像双份 + 主题切换重渲染。
7. **令牌架构:** 迁三层(tokens/semantics/ep-theme),保留 `--hs-` 前缀。理由:降低存量 diff 与 review 风险,多 Hub 产品生态下避免 `--ph-` 混读;层级与纪律完全对齐上游,前缀差异仅为命名空间。
8. **既有 token 映射:** 全表见附录 A;要点——hero panel 中性浅灰底 = `--hs-bg-page`(同源语义「页面级中性底」);`--hs-brand-soft` → teal-50(亮)/teal-900(暗);`--hs-shadow-card` 撤销改描边分层(工具风审美第 2 条);EP 派生阶从预生成字面量改 color-mix。

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
