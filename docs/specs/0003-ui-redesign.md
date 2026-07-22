# HubScope — 全站 UI/UX 重设计提案(渐进式)

> 状态:proposal。本提案已与 owner 确认核心决策(动机、视觉方向、品牌色、分批、导航、Dashboard 演进、技术约束),不可推翻;未确认的细节以「待评审」标注,走 design-review 流程。设计规范唯一事实源为 [.claude/rules/ui-guidelines.md](../../.claude/rules/ui-guidelines.md),本提案落地后其 §2 按文末「规范修订」同步更新。

## 1. 背景与目标

### 1.1 现状

HubScope 前端自 MVP 起沿用 Element Plus 默认主题,无品牌定制:主色 `#409EFF`、全局背景 `#f5f7fa`、文字 `#303133` 系。功能完整、语义忠实(状态色映射 §3 从未破例),但整体观感停留在「能用的内部工具」:

- **无品牌识别:** 纯默认主题,与任意 Element Plus 后台模板无差异;
- **导航散装:** 各页面 header 手写 router-link 散链(Dashboard 顶部一排链接,EvalCenter/TaskCenter 各自为政),无统一壳,登录态不可见;
- **第一视觉层级错位:** Dashboard 第一屏是 5 张汇总卡(数字罗列),访客要「3 秒看懂健不健康」需自行心算,没有一句全局结论;
- **细节不系统:** 字号、圆角、阴影无 token 化,硬编码色值散落在 scoped style 中。

### 1.2 动机与边界

**升级驱动,渐进式重设计,不推翻重做。** 组件体系(Vue 3 + Element Plus + ECharts)、页面信息架构、交互范式全部保留;重设计聚焦三层:主题层(品牌 token 覆盖)、壳层(统一 AppHeader)、首屏层(Dashboard 健康横幅)。不引入新依赖,不碰 `go:embed` 单二进制交付(W8)。

### 1.3 成功的样子

- 访客打开 Dashboard,**第一眼看到一句全局结论**(全部正常 / N 个端点异常 + 24h 可用率),而非一堆数字;
- 所有页面(登录页除外)共享同一 AppHeader,当前位置、登录态一眼可辨,不再出现「这个页面怎么回管理台」的散链;
- 全站只存在一套设计 token,scoped style 中不再出现调色板外的硬编码色值;新增页面作者抄 token 表即可,不需设计决策;
- 状态语义辨识度零损失:红黄绿橙红的语义、告警闪烁、状态词表完全不变(W5 语义,本提案只做视觉翻译)。

## 2. 设计原则

1. **状态优先,结论先行。** 监控产品的第一信息是「现在健不健康」。任何页面布局先回答读者最急的问题:状态板先给全局结论,再给分组明细;管理台先给可操作项。颜色是信息不是装饰。
2. **第一眼 3 秒原则。** 状态板读者(多为非管理员访客)3 秒内必须能回答「红黄绿?」。大字结论 + 状态色块优先于表格与图表;密度让步于辨识度。
3. **密度分区。** 状态板低密度大留白(浏览),管理台中密度(操作),两者共用 token 但间距档位不同;不为了「好看」把管理台做稀。
4. **组件优先,零新轮子。** 一切从 Element Plus 与既有组件(StatusBadge 等)出发,定制只发生在 CSS 变量与组合层;禁止自造表单/弹窗/状态灯。
5. **token 化,不留游离值。** 颜色、字阶、圆角、阴影只从 §3 token 表取;新需求若 token 表装不下,先修订规范(ui-guidelines.md)再写代码,禁止「特例」。

## 3. 设计 Token 全表

### 3.1 颜色 token

| Token | 值 | 用途 | 实现方式 |
|---|---|---|---|
| `--hs-brand` | `#3B5BFD` | 品牌主色:主按钮、链接、当前导航、聚焦态 | 覆盖 `--el-color-primary` |
| `--hs-brand-hover` | `#6180FF` | 主色 hover/active 浅阶 | 覆盖 `--el-color-primary-light-3` |
| `--hs-brand-soft` | `#EEF2FF` | 品牌浅色底:选中行、横幅底、高亮块 | 覆盖 `--el-color-primary-light-9`,自定义变量兜底 |
| `--hs-text-primary` | `#1F2329` | 标题、正文主文字 | 覆盖 `--el-text-color-primary` |
| `--hs-text-regular` | `#3E4450` | 常规正文(由主文字派生) | 覆盖 `--el-text-color-regular` |
| `--hs-text-secondary` | `#646A73` | 次要说明、标签、辅助信息 | 覆盖 `--el-text-color-secondary` |
| `--hs-text-placeholder` | `#9CA3AF` | 占位/禁用文字 | 覆盖 `--el-text-color-placeholder` |
| `--hs-border` | `#E5E6EB` | 卡片边、分割线、输入框边 | 覆盖 `--el-border-color` / `--el-border-color-light` |
| `--hs-bg-page` | `#F7F8FA` | 页面全局背景 | 覆盖 `--el-bg-color-page`,App.vue 引用 |
| `--hs-bg-card` | `#FFFFFF` | 卡片/面板底 | `--el-bg-color`(默认即白,不覆盖) |

**状态色(语义承重,不改值、不改映射,见 ui-guidelines §3):**

| 语义 | 值 | 实现方式 |
|---|---|---|
| healthy 正常 | `#67C23A` | `--el-color-success`(默认即此值,不覆盖) |
| degraded 降级 | `#E6A23C` | `--el-color-warning`(同上) |
| down 宕机 | `#F56C6C` | `--el-color-danger`(同上) |
| failing 告警 | `#FF4500` + 闪烁 | 自定义 `--hs-status-failing`(Element Plus 无此语义,走自定义变量,唯一来源为 StatusBadge) |

**实现策略(Element Plus 变量 vs 自定义变量):**

- **走 `--el-*` 覆盖:** 一切 Element Plus 组件已消费的语义槽位(primary、text、border、bg、radius)。在主色上还需同步覆盖派生阶:`--el-color-primary-light-3/5/7/8/9` 与 `--el-color-primary-dark-2`,从 `#3B5BFD` 按 Element Plus 同款 mix 算法预生成,保证 hover/禁用/浅底阶调一致。
- **走自定义 `--hs-*`:** Element Plus 没有对应槽位的值——`#FF4500` 告警色、品牌软底的高亮用法(横幅、导航选中底)、状态横幅的语义渐层。自定义变量只定义在 `:root`,组件一律引用变量不写字面值。
- 全部 token 集中在 `web/src/styles/tokens.css`(新文件,`main.ts` 在 `element-plus/dist/index.css` 之后引入,保证覆盖序)。

### 3.2 字阶

固定六档,不新增:

| 档位 | 值 | 用途 |
|---|---|---|
| `--hs-text-xs` | 12px | 辅助说明、标签、时间戳 |
| `--hs-text-sm` | 13px | 次要正文、StatusBadge、表格次要列 |
| `--hs-text-md` | 14px | 正文基准、表单、表格主列 |
| `--hs-text-lg` | 16px | 卡片标题、分组标题 |
| `--hs-text-xl` | 20px | 页面标题、关键数字 |
| `--hs-text-2xl` | 24px | 健康横幅大字结论(横幅专用,不作通用标题) |

字重只用 400/600 两档;行高默认 1.5,数字类(metric value)1.2。

### 3.3 间距

4px 基准网格,常用档位:4 / 8 / 12 / 16 / 24 / 32。卡片内边距 16px(管理台可 12px 紧凑档),区块间距 16px,页面上下 24px。内容区 `max-width: 1200px` 居中不变。

### 3.4 圆角与阴影

| Token | 值 | 用途 |
|---|---|---|
| `--hs-radius` | 6px | 全局统一圆角,覆盖 `--el-border-radius-base`;卡片、按钮、输入框、弹窗一律 6px |
| `--hs-radius-sm` | 4px | 小元素(tag、评分徽标、24h 小点) |
| `--hs-shadow-card` | `0 1px 2px rgba(31,35,41,.04), 0 1px 6px rgba(31,35,41,.06)` | 卡片静态(比 Element Plus 默认更轻) |
| `--hs-shadow-hover` | `0 4px 12px rgba(31,35,41,.10)` | 可点卡片 hover 浮起 |

阴影只表达「可点/浮层」语义,不用于装饰性堆叠;不可点的信息卡用 `shadow="never"` + 边框。

## 4. 全局壳规格

### 4.1 AppHeader(新组件 `components/AppHeader.vue`)

全页面共用(登录页除外),替换当前各页手写散链。

**结构(左→右):**

```
[Logo 方块 + HubScope]   [状态总览] [评估中心] [任务中心]        [管理视图] [登录 / 退出]
      品牌区                  主导航(公开读者路径)                管理入口    登录态
```

- **尺寸:** 高 56px,白底,底部 1px `--hs-border` 分割线;内容 `max-width: 1200px` 居中,与页面内容区对齐;sticky 置顶(`position: sticky; top: 0; z-index` 高于卡片)。
- **品牌区:** 24×24 圆角方块(品牌色底 + 白色「HS」或简化图形,纯 CSS 实现,不引入图片资源) + 「HubScope」16px 600 字重;点击回 `/`。
- **主导航:** 三项均为 router-link,14px;当前页高亮 = 品牌色文字 + 底部 2px 品牌色指示条;非当前项 `--hs-text-regular`,hover 变 `--hs-brand-hover`。「评估中心」「任务中心」对未登录访客可见但点击后被路由守卫弹去登录(现状行为保留,导航不隐藏)。
- **右侧:** 「管理视图」为次强调按钮(`el-button` 默认型,文字「管理视图」);登录态取自 `fetchAuthStatus`(与路由守卫同源):未登录显示「登录」主按钮(跳 `/login`),已登录显示「退出」文字按钮(调 logout 后跳 `/`)。会话状态在 AppHeader 内自查一次并监听路由变化刷新,不新增全局状态库。
- **挂载方式:** App.vue 中按路由 meta 条件渲染(`route.name !== 'login'`),各视图删除自己的 header 散链与「HubScope」标题块。

### 4.2 登录页(`views/LoginView.vue`)

- 不渲染 AppHeader(无导航干扰,聚焦登录);
- 页面底 `--hs-bg-page`,垂直水平居中;
- 卡片上移至品牌区:Logo 方块 40px + 「HubScope」20px + 副标题「LLM Hub 监控与评估平台」13px 次要色,卡片本体保留 360px 宽、口令输入 + 主色登录按钮;
- 登录失败仍走 `ElMessage` 带原因(现状不变);成功回跳 `redirect` 不变。

## 5. Dashboard 规格(第一视觉层级重构)

### 5.1 全局健康横幅(新组件 `components/HealthBanner.vue`)

页面第一屏,替代汇总卡成为视觉焦点。

**文案与配色规则(与 W5 状态语义一一对应,不自造档位):**

| 条件 | 大字结论(24px) | 副文案(14px) | 横幅配色 |
|---|---|---|---|
| 无异常(healthy 覆盖全部启用端点) | 「全部正常」 | 「24h 可用率 XX.X% · 共 N 个端点 · 更新于 HH:mm」 | 浅绿底(`--el-color-success-light-9`)+ 绿结论字 |
| 存在降级,无宕机/告警 | 「N 个端点降级」 | 「24h 可用率 XX.X% · 另有 M 个正常 · 更新于 HH:mm」 | 浅黄底 + 黄结论字 |
| 存在宕机或告警 | 「N 个端点异常」 | 「告警 X · 宕机 Y · 降级 Z · 24h 可用率 XX.X%」 | 浅红底 + 红结论字;若含 failing,结论旁复用 StatusBadge 的告警闪烁点 |
| 数据加载中 | skeleton 占位(横幅骨架,高度固定防跳动) | — | 中性灰 |
| 刷新失败 | 保留现有 `el-alert` 错误条,横幅展示最后一次成功数据并标注「数据非最新」 | — | 沿用上轮状态色 |

- 「N 个端点异常」点击 = 应用状态过滤(等价于点现有汇总卡),滚动定位到矩阵区;无异常时不可点。
- 统计口径只计启用端点;禁用端点在副文案中以「· K 个已停用」尾注呈现,不进可用率。

**数据来源(后端改动点,需单独 ticket):** 现有 overview API 按组返回 `availability_24h`,但**无全局 24h 可用率聚合字段**。需扩展 `GET /api/overview` 响应增加全局聚合:`total_endpoints`、`status_counts`(已有 `statusCounts` 前端自算可替代,可不扩)、`availability_24h`(全局加权:按探测次数加权,口径与组内一致)。前端改动以「后端先扩字段」为前置;若批次 2 先行而后端未就绪,横幅副文案先降级为「共 N 个端点」,可用率位留空,不留假数据。

### 5.2 统计条(汇总卡降级)

原 5 张可点汇总卡改为**一行细条统计**:「总数 N · 正常 a · 降级 b · 宕机 c · 告警 d · 已停用 k」,13px,各状态项前置 StatusBadge 色点;保留点击过滤语义(点状态项过滤、再点取消),激活态用品牌色下划线而非卡片区框。过滤交互与现状一致(ui-guidelines §6 轮询/反馈约定不变)。

### 5.3 EndpointCard 精修点

- 状态指示从「顶部 3px 边条」改为**左侧 3px 竖条 + 左上角状态区**(视线从左向右扫,状态在最前);色值映射不变;
- 排版层级:模型名 16px/600(截断 + title 保留)→ 状态行(StatusBadge + 评分徽标)→ 指标行三项等宽(24h 成功率 / P50 / P95,标签 12px 次要色、数值 15px→14px 收敛进字阶)→ 24h 点带 → 页脚「最近探测」12px;
- hover 用 `--hs-shadow-hover` 浮起,替换现有 inset 框线反馈;
- 评分徽标浅底色从硬编码 `#f0f9eb/#fdf6ec/#fef0f0` 收拢为 `--el-color-*-light-9` 变量;
- 24h 点带弹性槽位逻辑不动(历史 bug 防线),仅圆角改 `--hs-radius-sm`。

### 5.4 过滤行

**保留,不删功能**:模型名过滤、协议、状态下拉、分组维度四项全留;排版收敛为一行(允许 wrap),「每 10 秒自动刷新 · 更新于」信息移入横幅副文案,过滤行不再重复。分组区(OverviewGroupSection)结构不变,仅随 token 更新视觉。

## 6. 分批实施计划

每批独立可交付、独立过验收;批间不阻塞业务 ticket,但同一批内不留半成品视觉(新旧 token 混排不超过一个发布周期)。

### 批 1:全局壳(主题 token + AppHeader + 登录页)

- **范围:** 新建 `styles/tokens.css` 全量 token;main.ts 引入;App.vue 壳改造;新建 AppHeader;LoginView 品牌区;各视图删除手写 header 散链。
- **涉及文件(预估):** `web/src/styles/tokens.css`(新)、`web/src/components/AppHeader.vue`(新)、`web/src/App.vue`、`web/src/main.ts`、`web/src/views/LoginView.vue`、`web/src/views/DashboardView.vue`(删 header)、`EvalCenterView.vue` / `TaskCenterView.vue` / `AdminView.vue` / `EndpointDetailView.vue`(各删散链与标题块)。约 9 个文件。
- **验收标准:** ① 全站主色/文字/边框/圆角呈现新 token,无 `#409EFF` 残留;② 五个页面共享 AppHeader,当前页高亮正确,登录/退出全链路可用;③ 登录页无 header、居中品牌布局;④ ui-guidelines §2 修订稿同步落地。
- **风险:** ① Element Plus 派生色阶(light-3..9)覆盖不全导致 hover/禁用态色偏——验收时逐组件过一遍按钮/tag/分页的 hover 与 disabled;② AppHeader 会话态与路由守卫时序竞态(守卫已查过 auth,header 重复查)——复用同一 `fetchAuthStatus`,接受一次冗余请求,不引入状态库。

### 批 2:Dashboard 健康横幅

- **范围:** HealthBanner 组件;统计条替代汇总卡;EndpointCard 精修;过滤行收敛;刷新信息迁移。
- **前置(后端):** overview API 扩展全局 `availability_24h`(见 §5.1,单独 ticket,黑盒测试走 W1 接缝)。
- **涉及文件(预估):** `components/HealthBanner.vue`(新)、`views/DashboardView.vue`、`components/EndpointCard.vue`、`components/OverviewGroupSection.vue`、`api/overview.ts`、`api/types.ts`、`composables/useOverview.ts`。约 7 个文件(不含后端)。
- **验收标准:** ① 四种横幅状态(全好/降级/异常含告警/加载中)逐一造数据自验截图;② 3 秒原则:首屏不滚动即可读出全局结论;③ 卡片无横向滚动、24h 点带不溢出;④ 横幅可点过滤与原汇总卡行为等价。
- **风险:** ① 可用率口径前后端不一致(加权方式)——后端 ticket 中先定义口径再写前端;② 横幅状态与过滤状态双源(横幅是全局,过滤是局部)——横幅永不受过滤器影响,文案只反映全局。

### 批 3:EndpointDetail

- **范围:** 详情页接入新 token 与字阶;状态区与 Dashboard 卡片同构(StatusBadge 居首);图表(ECharts)系列色复核调色板;探测记录表密度调整。
- **涉及文件(预估):** `views/EndpointDetailView.vue`、`components/ProbeRecordTable.vue`、`components/TimeSeriesChart.vue`。约 3 个文件。
- **验收标准:** ① 与 Dashboard 卡片的状态表达一致(同色同词);② 图表无调色板外色相;③ 返回路径走 AppHeader,页面内不再手写返回链(保留上下文返回链接可议,评审定)。
- **风险:** ECharts 主题与 CSS 变量脱节(图表色在 JS 中)——从 token 表 JS 镜像取色,不手抄。

### 批 4:EvalCenter / TaskCenter

- **范围:** 两页 token 化;表格行高/密度档(管理台紧凑 12px 内边距);评分档位色复核(阈值后端口径不变);趋势图断点标注视觉统一。
- **涉及文件(预估):** `views/EvalCenterView.vue`、`views/TaskCenterView.vue`、`components/EvalRunList.vue`、`EvalTrendChart.vue`、`ScoreMatrix.vue`、`EvalRunDetailDialog.vue`、`CaseLibrary.vue`。约 7 个文件。
- **验收标准:** ① 管理台密度不劣化(一屏信息量不减少);② 评分绿/黄/红阈值渲染与后端口径一致;③ el-tabs 不加 `lazy` 的既有约定不破。
- **风险:** 评估中心组件多、硬编码色值存量大——允许批内分两个 commit,但同一组件不留混排。

### 批 5:Admin

- **范围:** Admin 页 token 化;HubManager / ModelAdder / SettingsPanel / AuditLogs / ClassificationRules / EndpointTable 视觉收拢;破坏性操作确认样式统一。
- **涉及文件(预估):** `views/AdminView.vue` + 上述 6 个组件。约 7 个文件。
- **验收标准:** ① 表单/弹窗全部走 Element Plus 原组件 + token,无自造控件;② 凭证脱敏展示(后 4 位)样式不弱化(W6);③ 禁用/删除的二次确认路径不变。
- **风险:** Admin 为低频页,验收易走过场——frontend-checker 逐项对 §4/§5/§6 过,不以「不常用」降级标准。

## 7. 验收与回归

每一批完成后按序执行,缺一不可:

1. **frontend-checker 全量自查**,依据为 ui-guidelines.md(批 1 起依据为修订后 §2 + 不变 §3);自查清单含:溢出/截断、三态、轮询配对清理、语义色映射、文案词表。
2. **截图自验:** 批内每个改动页面至少「默认态 + 交互态(hover/激活)+ 异常态(空/错误)」三视角人工过目;Dashboard 批需四种横幅状态截图齐全。
3. **`make test` 全绿**(pre-commit 门禁强制):后端全部测试 + 前端类型检查 + 前端构建;批 2 另需 overview API 扩展的黑盒测试。
4. **规范回写:** 批内产生的新约定(如统计条交互、横幅点击语义)当批回写 ui-guidelines.md,不留口头约定。

## 8. 明确不做的事

- **不做深色模式**(浅色唯一主题,token 结构不为深色预留双轨);
- **不换框架/组件库**(Vue 3 + Element Plus + ECharts 不动,不引入 Tailwind/UnoCSS 等新体系);
- **不做侧边栏布局**(页面数量与双形态定位不需要,顶部导航足够;未来页面数超 6 再议);
- **不做响应式/移动端专项**(桌面优先 + 窄屏不阻断的现状策略不变);
- **不出静态 mockup / 高保真稿**(token 表 + 组件规格即事实源,实现即设计验证,截图自验替代稿图对稿);
- **不改状态语义与词表**(红黄绿橙红映射、「正常/降级/宕机/告警」词表、告警闪烁全部冻结,W5 承重);
- **不改信息架构与路由**(无新增/删除页面,无路由变更)。

## 9. 规范修订(随批 1 落地)

ui-guidelines.md §2「视觉基线」按本提案 §3 token 表重写(修订稿随本 spec 一并评审);§3 状态色映射保持不变;§6/§7 无变化。批 2 落地后补记「健康横幅」为登记组件(§5)。
