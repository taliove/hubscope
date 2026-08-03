---
version: 1
slug: "web-src-views-boardview-vue"
primary_target: "web/src/views/BoardView.vue"
related_targets: ["web/src/components/Leaderboard.vue","web/src/components/ScoreCell.vue","web/src/utils/boardSort.ts"]
---

# /benchmark 公开榜单(v2 重建)— 表面简报

> **v2 重写(2026-08-01,GH #122):** /board 演进为 /benchmark(GH #112 IA;/board 客户端重定向保活),Apple 产品对比页气质重构(GH #118,spec 0018 §13)。

## 范围与模式
- 模式:**Operate**(公开消费页);route `/benchmark`,未登录直达,与状态概览并列为公开侧两页之一;壳内渲染。
- 读者:外部接收者(未登录)——判断「哪个模型适合什么场景、数据可不可信」。

## 页面构成(自上而下)
1. **页头:** h1「**Benchmark**」(3xl 页面标题档——「页面 h1 = 侧边栏标签」惯例,GH #118 裁决;「评估榜单」保留为描述性短语)+ lede 一行(md secondary:「不同模型适合什么场景:同一套考题,各能力维度逐项对比。」)
2. **批次 meta 行**(GH #57 沿置):「批次 #N · 定时/手动 · 完成于|失败于 YYYY-MM-DD HH:mm」(sm secondary 中性;done→完成于、failed→失败于,防作假口径;finished_at null 时时间整段省略;仅 report 已加载分支渲染)
3. **running-note**(仅运行中批次):「新一批评估进行中,当前展示已完成批次」(sm secondary,无底色无边框,静态不轮询)
4. **Leaderboard**(shared 口径「保存图片」,`selectable=false`)
5. 三态:加载 skeleton(el-skeleton rows=8,轻容器 state-block)/ 错误 el-alert + 重试 / 无 settle 批次空态「暂无已完成的评估批次」(running 时附提示行,轻容器 state-block)

## 边界(ticket 81 沿置,spec 0010)
恒显**最新 settle 批次**矩阵榜单;列头排序 + family 筛选 + 保存图片**全部客户端完成**(公开端点一次性返回完整 report,不接参数;`utils/boardSort.ts` 的 `sortRows` 镜像服务端口径——null 沉底/降序/model_id 字典序 + 判分不完整恒沉底,禁第二排序口径;`familyOptionsOf` 取自未过滤全集,选项不随选择塌缩);**无批次切换、无行下钻、无 live 榜单、无轮询**。未登录态网络面板零 session API 调用。Leaderboard 组件完整规格(矩阵/live 半成品/判分不完整/ScoreCell/窄屏卡片式)以 ui-guidelines §5 与 share-materials brief 为准。

## v2 视觉(GH #118)
- 轻容器语法:榜单外壳 = 白面 + 1px 描边 + radius-lg + 无阴影,桌面 padding 24/32、窄屏 16(Leaderboard 组件内 media query)。
- Apple 对比页框架:标题承担层级,lede 答「这页是什么」;无营销 hero。
- **矩阵行 hover 上浮豁免(GH #118 main 裁决):** hairline 分隔的行上浮会破坏共享 grid 基线与 3px 仪式竖条几何;行可点性由 hover 填充 `--hs-brand-soft` + 指针承担(本页行不可点,豁免为组件级口径)。
- 文字场景 *-text 阶(档色分数、涨跌箭头);前 3 名仪式重译为蓝品牌(3px brand 竖条 + 名次大字)。

## 未决(另立批次)
- 解释层(一句话副标题扩展/图例)、视图状态 URL 深链、空态期望管理(「评估多久跑一次」)。
