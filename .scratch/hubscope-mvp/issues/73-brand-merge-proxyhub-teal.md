# 73 — 品牌并入:HubScope 视觉体系全面并入 ProxyHub 现代极简工具风

**What to build:** HubScope 前端视觉体系从现行品牌蓝(#3B5BFD)+ Element Plus 浅色默认,全面并入 ProxyHub「现代极简工具风」体系:电波青 teal 品牌色 + 青灰灰阶 + 令牌三层架构(tokens/semantics/ep-theme)+ 暗色一等公民。设计评审草案已定稿于 `.scratch/brand-merge/ui-guidelines-draft.md`(design-owner 评审,8 项裁决见附录 B),本票按草案落地。核心工作面:① 用草案替换 `.claude/rules/ui-guidelines.md` 本体(先修正草案中 Wordmark 字体条款:Google Sans Flex 改为系统字栈,不引外部字体);② `web/src/styles/` 从单层 tokens.css 迁到三层架构,保留 `--hs-` 前缀;③ 全部组件色值/阴影/圆角迁移(状态板→管理台→导出物料 StatusCard/分享页,含 ECharts JS 镜像色板亮暗双份);④ 新增 BrandMark(teal hub 字形 SVG,与 ProxyHub 完全同构)+ Wordmark 组件,删 `web/public/logo.png`,favicon 换 SVG;⑤ 暗色模式:AppHeader 右栏图标切换(未登录可用)+ `localStorage('hs:dark')` + index.html 防闪脚本 + 默认亮不跟随系统,导出物料恒亮主题;⑥ `CONTEXT.md` 加品牌节(命名「HubScope 电波青」)。**保留不动的业务语义**:状态词表两套、failing 闪烁唯一动画、分数/可用率阈值口径、防作假条款(ADR 0007 相关)、组件唯一性登记——只改视觉映射,不改语义。failing 色定为亮 `#c2410c`/暗 `#fb923c`(orange 系,草案登记为「不引入新色相」纪律的唯一具名例外,frontend-checker 检查白名单需含此二值)。

**Blocked by:** 无

**Status:** ready-for-agent

## 执行顺序(票内多 commit 拆分,单 commit ≤8 文件)

1. **docs commit**:草案修正(Wordmark 系统字栈)→ 替换 `.claude/rules/ui-guidelines.md`;`CONTEXT.md` 加品牌节
2. **tokens commit**:`web/src/styles/` 三层架构落地(tokens/semantics/ep-theme),`main.ts` 引入序调整,`element-plus/theme-chalk/dark/css-vars.css` 依赖引入
3. **brand commit**:BrandMark.vue + Wordmark.vue 组件、AppHeader/LoginView 品牌位替换、删 logo.png、favicon SVG
4. **组件迁移 commits**(按视图分批,每批 ≤8 文件):状态板(Dashboard/EndpointCard/HealthBanner/StatusBadge/OverviewGroupSection)→ 评估(Leaderboard/EvalProgressGrid/TrendChart/ModelTrendDialog)→ 管理台(Admin/TaskCenter/Login)→ 导出物料(StatusCard 全套/分享报告页)
5. **dark commit**:暗色切换(AppHeader 入口、localStorage、防闪脚本、ECharts 镜像双份与切换重渲染)
6. **验收 commit**:按草案附录 C checklist 逐项过,frontend-checker 白名单登记 `#c2410c`/`#fb923c`

## 验收清单

- [ ] `.claude/rules/ui-guidelines.md` 替换为草案修正版(Wordmark 字体 = 系统字栈,无 Google Sans Flex);`CONTEXT.md` 品牌节落地
- [ ] `web/src/styles/` 三层架构就位,页面/scoped style 只消费 semantics 层 `--hs-*` 令牌(BrandMark 渐变 stop 为唯一豁免)
- [ ] 全站色值:brand teal(亮 #0c8078/暗 #0faea2)、功能色 #059669/#d97706/#dc2626、failing #c2410c(亮)/#fb923c(暗);`grep -rn "#3B5BFD\|#EEF2FF\|#6180FF\|#FF4500" web/src` 零残留(规范文档历史文本除外)
- [ ] BrandMark + Wordmark 就位,logo.png 删除,favicon 为 SVG;BrandMark 永不裸用(与 Wordmark 同场)
- [ ] 暗色模式全链路可用:切换入口、localStorage 持久化、index.html 防闪、ECharts 双份镜像;导出物料(StatusCard PNG/PDF)恒亮主题
- [ ] 圆角四档(2/4/8/999)落地,24h 分段条 radius-xs 2px 存续;静态卡片阴影撤销改描边分层
- [ ] 字阶七档(含 display 28px)落地,HealthBanner 大字与 StatusCard 可用率大数字升档 display;display 档无管理台滥用
- [ ] 导出物料 failing 双编码(实心点+「含 N 个告警」chip)结构不变、色值更新
- [ ] brand-soft 全部消费点(选中行、横幅、StatusCard 品牌区)截图对比确认观感
- [ ] 暗色下着色文字场景(榜单涨跌、24h 可用率数字、EndpointCard 明细)可读性抽查;若对比度不足,记录为规范第二轮修订项,不在本票硬扛
- [ ] 附录 C checklist 全项过;`make test` 全绿;frontend-checker + design-owner 双审通过

## 风险登记(来自 design-owner 评审)

1. failing 橙是「调色板外色相」唯一例外,执行端需白名单机制防误报/漏报
2. 暗色下功能色同值(未提亮)对比度 3.0–3.9:1,着色文字场景需实测,可能需第二轮规范补 dark 提亮档
3. EP 派生阶改 color-mix 后浅底观感差异大,brand-soft 消费点需逐个截图对比
