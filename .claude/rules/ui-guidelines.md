# UI Guidelines(设计规范)

HubScope 前端唯一设计规范,由 `design-owner` 代理维护。`frontend-checker` 的 UI 自查以本文件为依据。修改本文件的语义映射(状态色、词表)属于承重语义变更,须在设计评审中说明理由。

## 1. 产品形态与读者

- **双形态:** 公开状态板(Dashboard、EndpointDetail,无需登录)+ 管理台(EvalCenter、TaskCenter、Admin,需登录)。
- **两类读者:** 状态板读者要「3 秒看懂健不健康」——状态优先,操作入口让位;管理台读者要「高效完成配置与排查」——信息密度优先,操作直达。
- **桌面优先:** 内容区 `max-width: 1200px` 居中(Dashboard 先例),不为手机做专门适配,窄屏不阻断使用即可。

## 2. 视觉基线

- **Element Plus 组件体系 + 品牌 CSS 变量覆盖,浅色唯一主题。** 全部 token 集中在 `web/src/styles/tokens.css`(`main.ts` 在 `element-plus/dist/index.css` 之后引入,保证覆盖序);新增代码与当批触及的 scoped style 一律引用 CSS 变量,禁止硬编码调色板色值;存量硬编码按 spec 0003 批 2–5(ticket 37–40)迁移,过渡期内不以旧代码违反本条判 FAIL。
- **颜色 token(值只以 tokens.css 为准):**

| 用途 | Token | 值 | 实现 |
|---|---|---|---|
| 品牌主色(主按钮、链接、当前导航、聚焦态) | `--hs-brand` | `#3B5BFD` | 覆盖 `--el-color-primary` |
| 主色 hover/active 浅阶 | `--hs-brand-hover` | `#6180FF` | 覆盖 `--el-color-primary-light-3` |
| 品牌浅底(选中行、横幅底、高亮块) | `--hs-brand-soft` | `#EEF2FF` | 覆盖 `--el-color-primary-light-9` |
| 标题、正文主文字 | `--hs-text-primary` | `#1F2329` | 覆盖 `--el-text-color-primary` |
| 常规正文 | `--hs-text-regular` | `#3E4450` | 覆盖 `--el-text-color-regular` |
| 次要说明、标签、辅助信息 | `--hs-text-secondary` | `#646A73` | 覆盖 `--el-text-color-secondary` |
| 占位/禁用文字 | `--hs-text-placeholder` | `#9CA3AF` | 覆盖 `--el-text-color-placeholder` |
| 卡片边、分割线、输入框边 | `--hs-border` | `#E5E6EB` | 覆盖 `--el-border-color` / `-light` |
| 页面全局背景 | `--hs-bg-page` | `#F7F8FA` | 覆盖 `--el-bg-color-page`,App.vue 引用 |
| 卡片/面板底 | `--hs-bg-card` | `#FFFFFF` | `--el-bg-color`(默认即白,不覆盖) |
| 告警色(唯一动画状态) | `--hs-status-failing` | `#FF4500` | 自定义变量,唯一来源 StatusBadge |

- Element Plus 派生阶(`--el-color-primary-light-5/7/8`、`--el-color-primary-dark-2`)从 `#3B5BFD` 按 Element Plus 同款 mix 算法预生成在 tokens.css 中,保证 hover/禁用/浅底阶调一致;新代码不直接引用派生阶,用 `--hs-*` 语义变量。
- **字阶六档,不新增:** `--hs-text-xs` 12px(辅助/标签/时间戳)、`--hs-text-sm` 13px(次要正文、StatusBadge)、`--hs-text-md` 14px(正文基准、表单、表格主列)、`--hs-text-lg` 16px(卡片/分组标题)、`--hs-text-xl` 20px(页面标题、关键数字)、`--hs-text-2xl` 24px(健康横幅大字结论,横幅专用;导出物料 StatusCard 沿用——用于结论词与 24h 可用率大数字,物料是横幅语义向静态画布的延伸)。字重只用 400/600;行高默认 1.5,数字类 1.2。
- **圆角:** 全局统一 `--hs-radius` 6px(覆盖 `--el-border-radius-base`,卡片/按钮/输入框/弹窗一律 6px);小元素(tag、评分徽标)用 `--hs-radius-sm` 4px;分段条/时间条类填充元素(如 24h 可用率条)用 `--hs-radius-xs` 2px,仅限此类元素。24h 可用率条为分段填满式(每格填满弹性槽位,读作连续时间条),不得退回离散圆点样式。
- **阴影语义:** 只表达「可点/浮层」,不用于装饰——卡片静态 `--hs-shadow-card`,可点卡片 hover `--hs-shadow-hover`;不可点的信息卡用 `shadow="never"` + 边框。
- **间距:** 4px 基准网格,常用 4/8/12/16/24/32;卡片内边距统一走 `--el-card-padding` 变量,**密度档位按读者类型划分**:消费页(状态板、评估榜单 /eval、分享报告页)16px,管理台(/admin 全部 tab,含评估运营与题库)紧凑档 12px——档位由读者决定,不由登录态决定;禁止 `:deep(.el-card__body)` 覆写;区块间距 16px,页面上下 24px;内容区 `max-width: 1200px` 居中。导出物料(StatusCard 等独立画布组件)不受 el-card 16/12px 密度档约束,内边距按物料设计定(StatusCard 为 40px 横向 / 32px 纵向),区块间距沿用 4px 基准网格。

## 3. 语义色映射(核心约定)

颜色承载业务语义,**映射关系只在本文件定义**,组件引用同一映射,禁止各组件自造颜色语义:

| 业务语义 | 颜色 | 说明 |
|---|---|---|
| healthy 正常 | success 绿 `#67C23A` | endpoint 状态 |
| degraded 降级 | warning 黄 `#E6A23C` | endpoint 状态 |
| down 宕机 | danger 红 `#F56C6C` | endpoint 状态 |
| failing 告警 | 橙红 `#FF4500` + 闪烁 | 比 down 更紧急,唯一允许动画的状态(见 StatusBadge) |
| 评分/百分比档位 | 绿/黄/红阈值 | 阈值以后端口径为准,前端不自定分界线 |

- ECharts 系列色从上述调色板取,正文/轴文字用 `--hs-text-primary`/`--hs-text-secondary` 等值(图表内走 JS 镜像 const,与 tokens.css 同步),不引入调色板外的新色相。
- 同一语义在状态板与管理台必须同色同词。
- **分数涨跌指示:** 升=success 绿、降=danger 红,与「分数高=好」同向;持平、无上批次、**跨 Suite 版本断点(分数不可比)一律不显示涨跌箭头**,用 `--hs-text-placeholder` 占位并标注「题目已变更」(忠实 ADR 0007:禁止把题库变化呈现为模型降级,这是本产品的防作假核心语义)。
- **榜单条形(0–100):** 按评分档位阈值着色(绿/黄/红,阈值以后端口径为准),与得分徽标同一映射,不为榜单另定分界线;涨跌箭头是独立维度,不替代条形档位色。
- **静态导出物料(PNG/PDF)中的语义表达(批 56 约定):** 动画不可用时,failing 闪烁转译为双编码——`--hs-status-failing` 橙红实心圆点/文字 + 「含 N 个告警」文字标注(橙红描边 chip),禁止用 danger 红替代橙红,禁止静默省略 failing 维度。blink 动画在静态导出会定格在半透明帧,故静态物料明细行的状态词用**语义色文字着色**表达(告警=橙红、宕机=danger、降级=warning),不使用 StatusBadge 圆点徽章——这是「禁止第二个状态灯实现」对静态物料的例外,仅限导出画布,页面内仍一律复用 StatusBadge。静态物料无 hover,长文本截断长度按完整可读性折中,不依赖 `title` 全显。
- **批次/运行状态色映射(ticket 52 登记,与 endpoint 状态映射并列,词表与语义不混用):**

| 业务语义 | 颜色 | 说明 |
|---|---|---|
| 运行中 | brand 蓝 `--hs-brand` | 进行中强调,禁闪烁;禁用 warning 黄(黄=降级专属) |
| 等待中 | 中性灰 `--hs-text-placeholder` | 未开始 |
| 已完成 | success 绿 | 与 endpoint 域同向(好) |
| 失败 | danger 红 | 与 endpoint 域同向(坏) |

  绿/红在两语义域同向复用;闪烁动画仍为 failing 告警专属,运行状态不得使用。
- **24h 可用率三档着色(批 59 登记,状态板既定口径,源自 EndpointCard 分段条):** 单小时格与 24h 聚合可用率共用同一阈值——≥95% success 绿、<95% warning 黄、0%(有探测且全失败)danger 红、无探测数据 `--hs-border` 灰;该阈值同时适用于分段条单元格、StatusCard 可用率大数字与明细行可用率列,不为任何单一场景另定分界线。聚合口径=按小时对齐求和 total/failures(与后端探测加权一致),禁止按端点简单平均。

## 4. 布局规范

- 页面结构:顶部标题/操作区 → 内容区(卡片或表格)→ 必要时底部辅助区。
- 内容容器首选 `el-card`;列表数据首选 `el-table`;管理页多功能分区用 `el-tabs`(**不加 `lazy`**,保轮询事件,ticket 19)。
- 详情抽屉/弹窗:`el-dialog` 用于需要聚焦的详情与表单(见 EvalRunDetailDialog),不在页面内嵌套多层展开区。
- 卡片内容**不得溢出、不得出现横向滚动条**;弹性列宽优先于固定宽度(历史 bug:24h 小点)。

## 5. 组件使用规范

- **Element Plus 组件优先**,不引入新 UI 库,不自造表单、弹窗、表格、分页。
- **StatusBadge 是唯一的状态展示组件**,需要展示 endpoint 状态处一律复用,禁止第二个状态灯实现。
- **HealthBanner 是 Dashboard 的全局健康横幅组件**(批 2 登记):四态(全部正常/N 个端点降级/N 个端点异常含告警闪烁/加载 skeleton),数据只反映全局、永不受页面过滤器影响;仅异常态可点(应用状态过滤并滚动定位)。其他页面不得复刻其结论文案模式。
- **Leaderboard 是评估榜单的唯一排行展示组件**(/eval 与 /report/{token} 复用):每模型一行(排名/模型名/条形/总分/涨跌箭头),模型名截断 + `title` hover 全显;Suite 切换、family 过滤、排序切换走榜单上方工具条,不进单元格;行下钻不内嵌行内展开,走 `el-dialog`(§4 既有约定,EvalRunDetailDialog 模式)。
- **Leaderboard 运行中半成品模式**(ticket 52,修订上一条登记):未完成批次下榜单可查看但——① 名次列显示 `–` 占位(`--hs-text-placeholder`),禁名次徽章;② 行序固定模型名字典序(后端保证,前端不重排),工具条禁用排序切换与 Suite 切换、保留 family 过滤;③ 涨跌箭头列整列隐藏;④ 覆盖率不满的分数带 Coverage 水印「X/Y 题」(`--hs-text-xs` + `--hs-text-secondary`,常规字重,不用颜色区分),覆盖率满不显示;⑤ 未跑能力点显示「进行中」占位(`--hs-text-placeholder`)且不计入总分;⑥ settle 后名次直接替换占位、不做强调动画,转场提示由 ElMessage 承载。
- **Leaderboard 维度分同屏条带与置信标记**(ticket 51 登记,复用 ticket 52 条带形态,settled 与 live 两模式同构):每模型行下方固定渲染能力点条带(能力点名/小条形/分数),总分不再独占一行;未判分能力点显示 `-` 占位且不计入总分。置信标记两件套:覆盖率水印「X/Y 题」沿用 ticket 52 样式(覆盖率满不显示),采样数不新增视觉元素、走条带项 `title` hover(「判分 X/Y 题 · 采样 N 次」);基准不可比文案三分支:题目已变更(suite_changed)/判分口径已变更(profile_changed)/考核口径不同(suite_missing),一律不显涨跌箭头。
- **EvalProgressGrid 是批次进度矩阵的唯一组件**(ticket 52 登记):模型 × 能力点状态矩阵,运行中/等待中批次的默认视图;首列模型名截断 + `title` hover,能力点列 flex 等宽,行随模型数纵向滚动,**不开横向滚动豁免**(§4 无特例);单元格四态用 §3 批次/运行状态色映射,纯展示不可点;批次级进度汇总置于网格上方同卡片内;「进度网格 / 实时分数」视图切换走卡片顶部 el-radio-group。**分享页只读模式(ticket 54 登记):** /report/{token} 运行中批次复用本组件,但不渲染视图切换(组件须支持隐藏切换的只读用法),网格是分享面运行中的唯一视图。
- **分享报告页运行中信息边界(ticket 54 登记,HIGH-1 口径的细化):** 分享面(/report/{token})未完成批次只公开**运行状态与判分覆盖**(模型 × 能力点四态 + X/Y 题),不公开任何分数、名次、涨跌——「实时分数」为登录控制台专属,分享页不提供切换入口;批次 settle 后分享面照常渲染完整榜单(既有行为不变)。依据:状态/覆盖率是运行元数据而非评估结论,不构成 spec 0004 所防的「半成品分数外流」;模型名单 settle 后本就公开;token 高熵、可撤销、走审计(ADR 0006 控制面不变)。已知并接受的增量:运行中分享面可见 per-model 失败归因(settle 后分享榜单从不点名失败模型)——失败是运行事实,不掩盖;该暴露瞬时、token 可控。分享页不新增任何依赖会话接口的交互入口。
- **TrendChart 是趋势类折线图的唯一通用组件**(批 32 登记):裸图表(不带卡片,布局由父级负责),默认在 null 点断线(未判分批次不得连成假分),支持竖向断点标注线(占位灰虚线,如「v2 起题目变更」);ECharts 色板 JS 镜像与 TimeSeriesChart 同一份映射。
- **ModelTrendDialog 是报告页行下钻的唯一趋势弹窗**(批 32 登记,ticket 51 修订):按模型按需拉取 `/api/campaigns/{id}/trends`,分数线(版本断点标注「vN 起题目变更」+ 判分口径断点标注「判分口径已变更」,同一位置双断点合并为一行标注,均复用 TrendChart 灰虚线断点机制)+ 探测成功率/延迟并列;已删除模型带「已删除」tag;加载/空/错误三态齐全。
- **AppHeader 导航按登录态过滤:** 未登录只渲染公开页导航项(状态总览)+ 登录按钮,不渲染会被路由门禁弹走的项(评估、任务、管理入口);登录态随路由切换重检(沿用 refreshAuth watch 先例),不写死 mount 时一次性判断。
- **StatusCard 是状态分享卡的唯一渲染模板**(批 56 登记,批 59 重设计构成):720px 逻辑宽、2x 导出的竖版品牌物料,自上而下固定构成——① 品牌区(`--hs-brand` 4px 品牌条 + `--hs-brand-soft` 浅底 + logo + 「HubScope 服务状态」`--hs-text-xl`/600);② 范围行(无筛选纯文本「全部端点」,有筛选逐项 chips:描边 + `--hs-radius-sm`,前缀灰 label + 值,状态 chip 值用语义色;分组卡首位恒为分组 chip,与筛选 chips 并存,一个不漏);③ hero panel(可用率优先,批 59 第二轮迭代,替代原「tone-tinted 结论块 + 独立指标行」两区块;用户反馈顶部太告警化、优先展示异常端点):结论与指标合并为单一 panel——`--hs-bg-page` **中性浅灰底(无 tone tint)** + `--hs-radius` + padding 16px 20px,左右两列 + 1px `--hs-border` 竖分隔。左列自上而下:「24h 可用率」`--hs-text-xs` 标签 → `--hs-text-2xl`/600 大数字按 §3 三档着色 + 小号次级「%」(`--hs-text-md` secondary)——可用率大数字当主标题,其三档着色承担严重度信号,顶部不再用告警化色块;其下 verdict 文案(与 HealthBanner 同源结论词,如「5 个端点降级」,tone 着色但 `--hs-text-sm` 次级)+ failing 静态双编码(橙红实心点 + 「含 N 个告警」橙红描边 chip);再下**完整分布串**「正常 N · 降级 N · 宕机 N · 告警 N」四段恒列,`--hs-text-xs`,非零段状态词语义色/600 + 数字 `--hs-text-primary`,零计数段整段 `--hs-text-placeholder`。右列:「平均延迟」同构标签 + `--hs-text-xl`/600 数字,`--hs-text-primary` 不着色。**防作假不变:verdict 与四段恒列分布串仍在,异常不掩盖——只是不再当头条、不再有 tone tint 色块;空态(tone-empty)可用率渲染 `-` + 「24h 内无探测数据」,verdict 与分布串均不渲染,中性灰底保证「无数据」永不读作「全部正常」。** null 延迟 → `-` placeholder + 「24h 内无探测数据」`--hs-text-xs`;④ 24h 分段可用率条(组内聚合,口径见 §3 三档条目,24 格分段填满式、格高 16px、`--hs-radius-xs`、2px 间距;条下两端「24 小时前」「现在」`--hs-text-xs` placeholder;聚合函数抽 utils 纯函数,禁按端点简单平均);⑤ 异常明细(封顶 10 条,严重度排序 告警>宕机>降级,**三段式行**:行 1 = 状态词语义色 `--hs-text-sm`/600 + 「模型 · 协议」`--hs-text-md` 截断 + 右侧单端点 24h 可用率 `--hs-text-sm` 同档着色(null → `-`),行 2 = status_reason `--hs-text-xs` secondary、最多两行截断,reason 为空则不渲染行 2,**行 3 = 单端点 24h 打点条**——全宽 24 格分段填满式、格高 8px、`--hs-radius-xs`、2px 间距、**无轴标**,左缩进与 reason 对齐(margin-left 40px),复用 §3 三档着色与 dotTier;打点条让维护受众一眼看出故障时段——「最近 1 小时炸的」vs「全天半死」,单点可用率数字做不到这个;行间 hairline 分隔;overflow 收尾「另有 N 个异常端点未列出,详见状态板」);⑥ 正常端点汇总行(「其余 N 个端点正常 · 24h 可用率区间 min%–max%」,「正常」success 色、余文 secondary;min==max 显示单值,全部 null 则附「(24h 内无探测数据)」;全正常态此行替代异常明细,措辞「全部 N 个端点正常」);⑦ 一句话总结(见下方独立条目);⑧ 页脚(hairline 分隔,左「生成于 YYYY-MM-DD HH:mm」+「另有 N 个已停用」`--hs-text-xs` placeholder,右 location.origin)。空态沿用批 56:零匹配/全停用时范围 chips 保留、hero panel 中性灰底 + 可用率 `-`(verdict 与分布串不渲染,永不读作「全部正常」)、分段条全占位、明细「暂无匹配的 Endpoint / N 个端点均已停用」、不渲染总结与正常汇总行。结论判定仍与 HealthBanner 同源,统计集合=快照范围;明细状态词按 §3 静态物料约定用着色文字,不复制状态灯实现。
- **分组独立分享入口(批 59 登记):** OverviewGroupSection 标题行右端(group-metrics 之后)放 text 型 `el-button`(Share 图标 + 「分享」文字,不用裸图标按钮——状态板读者 3 秒场景下图标语义不够直白),`@click.stop` 拦截冒泡、不触发整行折叠,hover 走 Element Plus text 按钮品牌色反馈;点击复用 StatusShareDialog(弹窗本体不变),快照范围 = 该分组条目 ∩ 当前页面筛选,scope chips 首位恒为分组 chip(label「分组」,值「厂商/能力/协议 · 组名」,维度词表 family→厂商、capability→能力、protocol→协议),其后列全部生效筛选条件。**卡片所有数字一律从快照 entries(enabled)计算,与范围 chips 恒一致(批 59 口径修订):筛选快照不得引用 OverviewGroup/Overview 的未筛选聚合字段,否则数字描述全集、chips 声明子集,自相矛盾,违反批 56 防作假约定;且 Overview 全局无 avg_latency_ms 字段,透传路径本就不完整。** 两个标量口径:① 24h 可用率 = 快照内 enabled entries 的 dots_24h 按小时求和 ok/total(探测加权,与 `internal/server/overview.go` groupAccumulator 同定义,无筛选时与后端数字构造性相等,口径见 §3 批 59 条目);② 平均延迟 = enabled entries 的 p50_ms 均值(前端无法从 dots 复现探测加权延迟,这是唯一 scope 恒一致的口径;已知代价:与分组标题行「均延」的探测加权 mean latency 数值可能略有差异——卡片内部自洽优先于与页面逐字相等);StatusCardSnapshot 只扩展 `group` 字段,不携带任何聚合标量(statusCardSnapshot.ts)。Dashboard 全局「分享状态」入口保留现状(筛选行主按钮,不动)。
- **StatusCard 一句话总结(批 59 登记):** 位置=明细区与页脚之间,hairline 分隔后;形态=「小结」前缀标签(`--hs-text-xs` placeholder)+ 一句话(`--hs-text-sm` secondary、常规字重,无底色无边框无语义色)——视觉权重明确次于 hero panel 主结论区,句式以行动建议动词(建议/无需)收尾,与 verdict 的陈述句式区分。生成规则抽纯函数(utils/statusCardSummary.ts),优先级自上而下命中即止:① 有告警 →「有 N 个端点触发告警,建议立即处理」;② 有宕机 →「N 个端点宕机,建议优先排查 {首个宕机模型}」;③ 有降级且存在连续非绿时段 →「{模型} 持续降级约 N 小时,建议排查上游」(N = 该端点 dots_24h 自最新向前连续「有探测且非绿」格数——只有 fail/partial 计入,无数据灰格与绿格一样中断计时;取全组最长者。**持续时长必须有连续证据支撑**:灰格计入会让稀疏数据端点一路数到窗口起点、恒输出「约 24 小时」,属无证据的时长宣称,违反本条「不掩盖异常」的对偶约束——不夸大异常,对维护受众是狼来了);④ 有降级无持续信号 →「N 个端点降级,建议关注,暂不紧急」;⑤ 全正常但 24h 可用率 <95% →「状态全部正常,但 24h 可用率仅 X%,建议持续观察」;⑥ 全正常且有数据 →「近 24 小时运行平稳,无需处理」;⑦ 无 24h 数据 → 在命中句后追加「暂无 24 小时探测数据」。空态(零匹配/全停用)不渲染总结行。**总结不得掩盖异常:** 只要存在非正常端点,总结首句必须指向最严重的异常(告警>宕机>降级),禁止输出「运行平稳」类措辞——这是批 56 防作假约定在文案层的镜像。
- **结论必须标注统计范围(防作假约定,批 56):** 任何呈现汇总结论的导出/分享物料,结论旁必须显式列出统计范围——无筛选标「全部端点」,有筛选逐项列出全部生效条件(一个不漏),零匹配时范围仍需保留且结论用中性「暂无数据」;禁止把局部集合呈现为全局结论,禁止零匹配显示「全部正常」(ADR 0007 防作假语义在状态板域的镜像)。批 59 补充:分组也是范围,分组卡必须带分组 chip(见上条),分组卡片不得省略分组维度词。
- **AppHeader 批次进度入口**(ticket 52 登记):仅登录态渲染(与导航同一过滤时机),位于 header-right 操作按钮左侧;文案「批次运行中 X/Y」(X/Y 口径与榜单进度一致),点击跳 /eval;发现靠 mount + 路由切换重检(refreshAuth watch 先例),仅存在未完成批次时 3s 轮询,settle 即停并隐藏,卸载必清理;禁用橙红与闪烁(闪烁为 failing 告警专属)。
- **角色 tag 是用户身份角色的唯一展示单元**(ticket 62 登记):AppHeader 右栏当前用户、AdminView 用户列表、UserManager 一律复用,禁止各处自造角色色。词表固定四词,不新增同义词:super_admin→「超级管理员」、admin→「管理员」、operator→「操作员」、viewer→「观察者」(与 spec 0005 角色定义一致)。**角色语义是「权限层级」非「健康度」**,禁用 §3 的 success/warning/danger 状态色——红黄绿是 endpoint 与批次健康信号,状态板读者眼里红=异常、绿=正常,把 super_admin 染红会读作「告警」、operator 染绿会读作「健康」,属语义错位(本映射经设计评审登记,与 §3 两套状态色并列、语义域不混用)。
- **角色色映射:**

| 角色 | el-tag type | 语义 |
|---|---|---|
| super_admin / admin | `primary`(brand 蓝) | 管理权(全局或 Hub 内全权) |
| operator / viewer | `info`(中性灰) | 非管理(操作 / 只读) |

  管理权=brand 与 §2「品牌主色=主按钮/链接/当前导航/聚焦态」同向(强调=可支配),非管理=info 中性灰。super_admin 与 admin 同色:区分靠词表(「超级管理员」vs「管理员」)+ 数据域(全局不绑 Hub vs Hub 内),不靠颜色加权——super_admin 稀有但染红代价(告警串扰)高于收益(视觉强调)。与 §3「等待中 中性灰」同为中性语义但词表与语义域不同(角色 vs 批次等待),靠上下文与词表消歧,禁止把等待中灰借用到角色域。
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
