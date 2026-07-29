---
target: 状态板 Dashboard(亮/暗实机截图 + 源码)
total_score: 22
max_score: 36
na_heuristics: 5
p0_count: 0
p1_count: 3
timestamp: 2026-07-29T07-54-36Z
slug: web-src-views-dashboardview-vue
---
# Critique:HubScope 状态板(Dashboard)

Method: dual-agent(A:设计评审 · B:机械检测)

## Design Health Score

| # | Heuristic | Score | Key Issue |
|---|-----------|-------|-----------|
| 1 | Visibility of System Status | 3 | 刷新 cadence/更新于/stale 标注都在;但最关键的 down/failing 端点首屏不可见 |
| 2 | Match System / Real World | 3 | 状态词表清晰;P50/P95 对公开读者是行话;裸数字「80」评分徽章无标签 |
| 3 | User Control and Freedom | 3 | 筛选可清可取消;banner 点击强制筛选后出口不明显 |
| 4 | Consistency and Standards | 2 | 状态排序两套口径(strip 轻→重 vs 组头重→轻);92.3% 与 97.6% 两个可用率无归属标注 |
| 5 | Error Prevention | n/a | 只读监控面,无破坏性操作;分享快照冻结已是恰当预防 |
| 6 | Recognition Rather Than Recall | 2 | 下拉用 placeholder 当 label;模型名截断砍掉区分后缀;评分含义靠 hover 回忆 |
| 7 | Flexibility and Efficiency | 2 | 筛选不进 URL(刷新即丢);无键盘加速;无严重度排序 |
| 8 | Aesthetic and Minimalist Design | 2 | banner 副文案 6 段与 strip 计数重复;三指标同权;同族组协议 tag 零区分度——噪音地板抬高 |
| 9 | Error Recovery | 3 | 「刷新失败:原因」+ stale + 保留旧数据,得体;无显式重试按钮(靠轮询自愈) |
| 10 | Help and Documentation | 2 | tooltip 兜底在;无图例(dots 灰/黄含义远距无从得知)、无降级成因可见解释 |
| **Total** | | **22/36** | **Acceptable(61%)** |

## Design Specificity Verdict

**LLM assessment:** 视觉语言有产品性格,信息架构没有。状态灯 + 中文状态词 + 24h 分段条 + sparkline + 防作假口径构成真实的监控品类语言;但页面骨架(顶部横幅 + 筛选栏 + 字母序分组卡片墙)是任何 admin dashboard 都能用的通用骨架——首屏按厂商字母序而非按严重度组织,没有把「监控产品 = 严重度优先」刻进布局。「层级不跳」的根因不在色值字号,在排序逻辑。

**Deterministic scan:** detect.mjs 对 8 个目标文件 0 findings(已做防 vacuous 验证)。机械反模式(AI slop 类:渐变文字/bounce 动画/滥用字体等)零命中——问题不在工艺底线,在信息架构与层级。注意:检测器不覆盖 HubScope 令牌纪律(硬编码色值、--el-* 书写),该纪律合规性不能由本次扫描背书。

**Visual overlays:** 无浏览器自动化工具,overlay 环节按流程降级,视觉证据由用户实机截图(亮/暗双份)承担。

## Overall Impression

底子扎实(暗色一等公民、分段条+sparkline 双行构造对齐、防作假纪律全守),但「3 秒看懂」的承诺在第二跳断裂:banner 喊完「2 个端点异常」之后,页面把那 2 个端点藏进了厂商字母序的下游。最大机会:让严重度驱动首屏组织,并把重复/均质的信息噪音降下来。

## What's Working

1. **HealthBanner 结论优先结构成立**:display 大字结论 + 全局口径不受筛选影响 + 仅异常态可点;空态中性灰不冒充「全部正常」。
2. **24h 分段条 + sparkline 双行构造性对齐**是全板最有产品质感的元素:一行答「什么时候坏的」,一行答「变得多慢」,x 轴构造对齐。
3. **暗色一等公民**:failing 橙、状态灯、分段条暗色下全部可分辨,暗色阅读节奏甚至优于亮色——三层令牌架构的回报是真实的。

## Priority Issues

**[P1] 严重度不驱动首屏:最紧急的端点在视口外**
- What: 分组按厂商字母序、组内按字典序,down/failing 端点落首屏以下;banner 只说「2 个」不说「哪 2 个」。
- Why: 状态板的唯一使命是让最严重的东西最先被看见;投屏/路过读者永远到不了异常点。
- Fix: ① banner 异常态副文案行内联异常端点名(可点 chips);② 组间按组内最高严重度降序,组内同级字典序;③ 组内卡片同严重度前置。①止血,①+②根治。
- Files: HealthBanner.vue、DashboardView.vue、OverviewGroupSection.vue
- Suggested command: /impeccable layout

**[P1] banner 与 stats strip 信息重复 + 元信息混排**
- What: banner 副文案严重度计数与 80px 下 stats strip 是同一组数字渲染两遍;「每 10 秒自动刷新 · 更新于」元信息与严重度计数同字号同权重混排。
- Why: 重复信息是纯 extraneous load;元信息稀释「降级 15」(60% 舰队)这个真正的第二跳信号。
- Fix: banner 只留结论 + 可用率 + 异常 chips;状态计数归 strip;刷新元信息降 xs/placeholder 移 banner 行右端。
- Files: HealthBanner.vue、DashboardView.vue
- Suggested command: /impeccable distill

**[P1] 卡片墙均质化:卡内无层级,卡间无差异**
- What: 三指标同字号同权重(fable-5 P95 56.16s 严重信号与 P50 平起平坐);模型名 4 列下截断「claude-haiku-4-5-…」砍掉区分后缀;评分徽章裸「80」无标签;同族组 anthropic tag 全同纯噪音。
- Why: 9+ 张卡片视觉重量全同 = 逐卡精读才能找异常,是「层级不跳」在卡片层的肉身;主键截断伤可识别性。
- Fix: ① 卡内主从:状态行升格(md/600),P50 主指标,P95 降 secondary;② 卡片 minmax 260→~300px(3 列)或模型名等宽+中间截断保后缀;③ 评分徽章加「评分」前缀或仅异常时显示;④ 组内协议 tag 全同时收敛重复(不动 GH #34 色映射)。
- Files: EndpointCard.vue、DashboardView.vue、OverviewGroupSection.vue
- Suggested command: /impeccable layout

**[P2] 两套状态排序口径 + 两个可用率无归属**
- What: strip 轻→重(STRIP_ORDER)vs 组头重→轻(STATUS_PRIORITY);banner 92.3%(全局)与组头 97.6%(本组)并列无口径标注。
- Why: 读者要心算 reconciliation;排序不一致破坏扫描肌肉记忆,小不一致腐蚀监控产品信任。
- Fix: 统一重→轻;组头可用率加「本组」前缀。
- Files: DashboardView.vue、OverviewGroupSection.vue

**[P2] 筛选控件 placeholder 充当 label + 双控件控制同一状态**
- What: 协议/状态下拉以 placeholder 为唯一标签,选中后标签消失;strip 与下拉同时控制 statusFilter。
- Why: Recognition over recall 失分;选中后不知道这是什么维度。
- Fix: 下拉加内联前缀 label(「协议:」「状态:」);strip=快捷路径的关系在规范登记为有意为之。
- Files: DashboardView.vue

## Persona Red Flags

**Alex(效率专家):** 筛选不进 URL——刷新丢整个过滤视图,无法收藏/转发「只看降级」;分享产物是 PNG 非可链接过滤视图;无键盘路径(无 `/` 聚焦);无法按严重度/可用率/延迟排序;banner 点击后无一键复位。

**Sam(无障碍依赖):** stats strip 可点项是 `<span @click>` 无 role/tabindex/键盘事件;组头折叠、整卡下钻、banner 点击四处主交互全部 click-only;24h dots 信息只在 hover tooltip,对屏幕阅读器是 24 个空 span;dots 三态纯颜色编码无图例。

**投屏/路过读者(项目特有,3 秒远距):** 第一跳(红色大结论)远距可读,第二跳不存在——「是哪 2 个」需走近逐卡找;状态词 13px、指标 12-14px 两米外不可读;模型名截断远距彻底失效;24h 可用率 92.3% 以 14px 混在副文案第六段——页面上没有一个一眼可读的健康大数字。

## Minor Observations

- 「总数 25」teal 下划线读作 tab 选中态,与导航 tab 下划线语言撞车(它表达的是「无筛选」)。
- banner 可点仅 cursor + hover shadow,无「点击查看异常」提示,可发现性靠缘分。
- 暗色下 group section 卡面 #17191d 对页面底 #0f1115 阶跃小,section 边界比亮色模糊。
- 「已停用 N」>0 才出现,strip 布局随其跳动。
- stale「数据非最新」chip 白底描边在红色 soft 底上突兀。
- `claude-opus-4.6` 与 `claude-opus-4-6` 并存,截断加剧混淆,佐证保后缀重要。
- 登录态 header「管理视图」实心按钮抢走状态板主行动位。

## Questions to Consider

1. 如果 banner 直接列出异常端点名字(chips 即入口),「点击横幅过滤+滚动」还有存在必要吗?首屏应按厂商分组,还是按「需要关注」分组?
2. 页面是否缺一个 display 档「24h 可用率」大数字(与 StatusCard 物料同源)?3 秒读者只能记住一个数字,现在没有元素在争取成为那个数字。
3. 卡片的 P95、评分徽章、dots、sparkline 四样里哪些是 3 秒读者要的、哪些该退到详情页?均质满卡墙 vs「异常卡大、健康卡小」的非均质矩阵,哪个更像监控产品?
