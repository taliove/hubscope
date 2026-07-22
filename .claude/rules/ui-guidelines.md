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
- **字阶六档,不新增:** `--hs-text-xs` 12px(辅助/标签/时间戳)、`--hs-text-sm` 13px(次要正文、StatusBadge)、`--hs-text-md` 14px(正文基准、表单、表格主列)、`--hs-text-lg` 16px(卡片/分组标题)、`--hs-text-xl` 20px(页面标题、关键数字)、`--hs-text-2xl` 24px(健康横幅大字结论,横幅专用)。字重只用 400/600;行高默认 1.5,数字类 1.2。
- **圆角:** 全局统一 `--hs-radius` 6px(覆盖 `--el-border-radius-base`,卡片/按钮/输入框/弹窗一律 6px);小元素(tag、评分徽标)用 `--hs-radius-sm` 4px;分段条/时间条类填充元素(如 24h 可用率条)用 `--hs-radius-xs` 2px,仅限此类元素。24h 可用率条为分段填满式(每格填满弹性槽位,读作连续时间条),不得退回离散圆点样式。
- **阴影语义:** 只表达「可点/浮层」,不用于装饰——卡片静态 `--hs-shadow-card`,可点卡片 hover `--hs-shadow-hover`;不可点的信息卡用 `shadow="never"` + 边框。
- **间距:** 4px 基准网格,常用 4/8/12/16/24/32;卡片内边距统一走 `--el-card-padding` 变量,**密度档位按读者类型划分**:消费页(状态板、评估榜单 /eval、分享报告页)16px,管理台(/admin 全部 tab,含评估运营与题库)紧凑档 12px——档位由读者决定,不由登录态决定;禁止 `:deep(.el-card__body)` 覆写;区块间距 16px,页面上下 24px;内容区 `max-width: 1200px` 居中。

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
- **TrendChart 是趋势类折线图的唯一通用组件**(批 32 登记):裸图表(不带卡片,布局由父级负责),默认在 null 点断线(未判分批次不得连成假分),支持竖向断点标注线(占位灰虚线,如「v2 起题目变更」);ECharts 色板 JS 镜像与 TimeSeriesChart 同一份映射。
- **ModelTrendDialog 是报告页行下钻的唯一趋势弹窗**(批 32 登记):按模型按需拉取 `/api/campaigns/{id}/trends`,分数线(版本断点标注)+ 探测成功率/延迟并列;已删除模型带「已删除」tag;加载/空/错误三态齐全。
- **AppHeader 导航按登录态过滤:** 未登录只渲染公开页导航项(状态总览)+ 登录按钮,不渲染会被路由门禁弹走的项(评估、任务、管理入口);登录态随路由切换重检(沿用 refreshAuth watch 先例),不写死 mount 时一次性判断。
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

## 7. 文案规范

- 界面一律简体中文;**状态词表分两套,互不混用:** endpoint 状态=**正常 / 降级 / 宕机 / 告警**(与 StatusBadge LABELS 一致,不新增同义词);批次/运行状态=**等待中 / 运行中 / 已完成 / 失败**(沿用 campaignStatusLabel 既有口径)。禁止把一套词借用到另一语义域(如批次失败不得称「宕机」)。
- **分数展示统一 0–100 整数**(null → `-`),`formatScore` 集中于 `utils/format.ts`,组件内禁止自写 `toFixed` 分数格式;0~1 原始分只存在于 API 层。
- 按钮用动词短语(「触发同步」「新建 Hub」),不用「确定/提交」以外的泛词;错误消息必须带原因,不只说「失败」。
- 数字与时间格式统一走 `utils/format.ts`,不在组件内各写格式化。

## 8. 规范的维护

- 本文件由 design-owner 代理维护;设计评审中做出的新约定须回写本文件,否则视为未约定。
- 本文件与 [load-bearing-walls.md](./load-bearing-walls.md) 的关系:本文件管「体验一致性」,承重墙管「系统语义」;冲突时以承重墙为准(如状态机红黄绿语义由 W5 决定,本文件只做视觉映射)。
