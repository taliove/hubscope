---
version: 1
slug: "web-src-views-boardview-vue"
primary_target: "web/src/views/BoardView.vue"
related_targets: ["web/src/components/Leaderboard.vue","web/src/components/ScoreCell.vue"]
---

# /board 公开榜单 — 表面简报

## 范围与模式
- 模式:**Operate**(公开消费页);route `/board`,未登录直达,与状态板并列为公开侧第二页。
- 读者:外部接收者(未登录)——判断「哪个模型好、数据可不可信」。

## 页面构成
1. AppHeader(公开侧形态,同 Dashboard 简报)
2. 页头:`评估榜单` 标题行(xl/600,工具风层级,非营销 hero)+ **批次 meta 行(GH #57 起生效:**「批次 #N · 定时/手动 · 完成于 YYYY-MM-DD HH:mm」,sm/secondary 中性不着色,§7 批次词表;无 settle 批次空态不渲染)
3. running-note(仅运行中批次时):「新一批评估进行中,当前展示已完成批次 #N」(sm secondary,无底色无边框)
4. Leaderboard(shared 口径「保存图片」)
5. PublicFooter

## 边界(ticket 81 登记,spec 0010)
恒显**最新 settle 批次**矩阵榜单(复用 Leaderboard 组件);列头排序 + family 筛选 + 保存图片**全部客户端完成**(公开端点一次性返回完整 report;sortRows 镜像服务端口径——null 沉底/降序/model_id 字典序 + 判分不完整恒沉底,禁第二排序口径);**无批次切换、无行下钻(行不可点,selectable prop)、无 live 榜单、无轮询**;无 settle 批次 → 空态「暂无已完成的评估批次」(running 时附提示行)。未登录态网络面板零 session API 调用(auth status 除外)。Leaderboard 组件完整规格(矩阵/live 半成品/判分不完整/ScoreCell)在评估域简报建立前仍以 ui-guidelines §5 为准。

## 体检基线与已排改进
- critique 基线 26/36(2026-07-29):页面不标批次身份/时效(P1)、公开读者零解释层(P1)、涨跌理解依赖记忆桥(P2)、排序无可发现性(P2)、状态不进 URL(P3)。
- 已排票:#57(批次 meta 行,已按修后口径登记于构成)。

## 未决(另立批次)
- 解释层(一句话副标题 + 工具条图例 popover,与 ScoreCell tooltip 同源)、列头排序常驻弱化指示、视图状态 URL 深链、空态期望管理(「评估多久跑一次」)。
- 词表待裁决:family 筛选 UI 文案「系列」 vs 状态板分组 chip 词表「厂商」不一致,随语义手册定稿统一。
