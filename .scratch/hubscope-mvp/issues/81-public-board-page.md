# 81 — /board 公开榜单页

**What to build:** 按 spec 0010 新增精简公开页 `/board`(meta.public,标题「评估榜单」),恒显最新 settle 批次矩阵榜单。BoardView(新)复用 Leaderboard 组件:父级持完整 report,`query` 事件**本地处理**——`sortRows(rows, key)` 纯函数(分数降序、null 沉底与服务端 unscored-last 同口径、同分 model_id 字典序)+ family 精确匹配过滤,familyOptions 来自未筛选全集;Leaderboard 组件零改动或近零改动。保存图片复用 EvalShareDialog/EvalCard(纯客户端,文案「保存图片」shared 口径),快照 = 当前排序/筛选生效后的所见。运行中提示行:running=true 时榜单上方一行 `--hs-text-sm` secondary「新一批评估进行中,当前展示已完成批次 #N」,无底色无边框不轮询。空态:无 settle 批次 → 「暂无已完成的评估批次」(+ running 提示行)。AppHeader:未登录导航加「评估榜单」;/board header **不渲染登录按钮**,页脚 hairline + text 小字「管理登录」→ /login(`--hs-text-xs` placeholder);状态板维持现状。行不可点(无下钻);涨跌列/前 3 名/水印等矩阵口径沿用。

**Blocked by:** 80(公开端点)

**Status:** done(4 commit:5b3a2dc → 11afb99 → 69d39cf → 5bffb38;check 三维度 PASS;2 条 LOW 折入 ticket 82)

## 执行顺序(票内 commit 拆分,单 commit ≤8 文件)

1. **utils commit:** `sortRows` + family 过滤纯函数 + vitest(降序/null 沉底/同分字典序/维度键/空数组)
2. **页面 commit:** BoardView + 路由(meta.public)+ API client(getPublicEvalBoard)+ 空态/运行中提示行/loading 三态
3. **导航与页脚 commit:** AppHeader 未登录导航项 + /board header 去登录按钮 + 页脚「管理登录」
4. **规范 commit:** ui-guidelines §1 公开双页 + §5 /board 条目 + AppHeader 导航条目登记;spec 0010 Status 已 accepted

## 验收清单

- [ ] 未登录直达 /board:榜单完整渲染(矩阵/涨跌/前 3 名/水印),无任何 session API 请求(网络面板核对)
- [ ] 列头排序:该列降序/再点回总分,客户端完成(无新请求);null 沉底;family 筛选即时生效;familyOptions 不随筛选坍缩
- [ ] 保存图片:预览 = 当前排序/筛选所见;下载 PNG 文件名口径不变;暗色会话导出亮主题;HTTP 裸 IP 复制降级
- [ ] running=true:提示行文案正确(批次号 = 展示的 settle 批次);无 settle 批次空态 + 提示行
- [ ] AppHeader:未登录导航 = 状态总览 + 评估榜单;/board header 无登录按钮,页脚「管理登录」可点;状态板 header 登录按钮不变;登录态导航不变
- [ ] 行不可点无下钻;三态(加载/空/错误+重试)齐全
- [ ] ui-guidelines 登记落地;`make test` 全绿

## 风险登记

1. **Leaderboard 复用边界**:BoardView 以「本地处理 query」方式复用,若发现 Leaderboard 内部有假设「query 必然后端往返」的逻辑(如排序指示与 viewSuite 状态),优先在 BoardView 适配,不动组件;不得不动时保持 /eval 行为零变化
2. **保存图片快照**:buildEvalCardSnapshot 的 query 参数语义(排序非默认出 chip)在客户端排序下同样成立;注意「排序 · 能力点名」chip 与页面状态一致
3. **/board 与 /eval 样式漂移**:两页共用 Leaderboard,美化(82)自动双端生效;本票不做任何 /board 专属视觉
