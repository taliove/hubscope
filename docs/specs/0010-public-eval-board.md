# 0010 — 公开榜单页 /board 与榜单美化

**Status:** accepted(2026-07-27,grill 共识 + 用户评审通过)
**ADRs:** 无新增(公开只读边界沿用 ADR 0006 控制面语义;本 spec 登记「榜单公开」产品决策)
**Tickets:** 80(后端公开只读端点)/ 81(/board 公开页)/ 82(榜单美化三件套)

## 背景

两点用户决策(2026-07-27 grill):

1. **榜单应对外公开。** 现状:eval API 自 ticket 16 起全部要 session,/eval 与 /campaigns/:id/report 会话门禁,免登录看榜单只有 /report/:token 分享链接(高熵、可撤销,不是产品公开页)。用户定位:榜单与状态板并列为**公开侧第二页**,未登录直接可看,「直接出版本」。
2. **矩阵榜单(78/79 交付)还可以更优美。** 方向经 grill 圈定三件套:表格化精致、轨道中性化、前三名仪式感。

## 决策总览(2026-07-27 grill 共识,逐项确认)

| 域 | 决策 |
|---|---|
| 公开形态 | 新增精简公开页 `/board`「评估榜单」,恒显**最新已 settle 批次**矩阵榜单;/eval 保持登录完整版不变 |
| 页面交互 | 列头排序(该列降序/再点回总分)+ family 筛选 + 保存图片,**全部客户端完成**(API 一次性返回完整 report);无批次切换、无行下钻、无 live 榜单 |
| 运行中批次 | 恰有批次在跑时,榜单上方一行静态中性提示「新一批评估进行中,当前展示已完成批次 #N」;不轮询、不带进度细节(运行状态是元数据,ticket 54 同口径) |
| 入口 | AppHeader 未登录导航 = 状态总览 + 评估榜单;**/board header 不放登录按钮,页脚 text 小字「管理登录」**(「状态板维持现状」已被 ticket 84 修订:所有公开页统一无 header 登录按钮,页脚管理登录) |
| 后端 | 新增公开只读端点,返回最新 settle 批次 report + 运行中标志;其余 eval API 维持 session;写操作面零暴露 |
| 美化 ① | 表格化精致:列头下 hairline + 行间 1px `--hs-border-light` 分隔 + 行高 **46px 定稿**(44–48 区间内,15 行一屏算术最优) |
| 美化 ② | 轨道中性化:细条轨道 `--hs-brand-soft` → **`--hs-bg-hover` 定稿**(亮 #eff4f5 / 暗 #1f2227;「卡面之上一步的中性填充」语义,档色成全榜唯一跳色) |
| 美化 ③ | 前三名仪式感:行左侧 3px `--hs-brand` 竖条(描边语言,不用背景色块)+ 名次数字放大一档;teal 600 保留 |

## 后端公开只读端点(ticket 80)

- **GET `/api/public/eval/board`** → `{ "report": <与 /api/campaigns/{id}/report 同形状> | null, "running": bool }`。
  - `report` = 最近一次 settle(done/failed)批次的完整报告(总分降序,与现有报告同序列化);无 settle 批次 → `null`(前端空态)。
  - `running` = 是否存在 pending/running 批次(前端提示行)。
- 匿名 200,不读 session;不暴露任何写接口、task 细节、token。既有 eval API(campaigns 列表/报告/趋势/触发)维持 session 不变。
- 行数据含 family/suite_scores/cells/weights/baseline——与 /report/:token 分享面已公开的信息同级(评估结论本就可经 token 外传,本决策是把它从「链接可达」升级为「页面直达」,信息级别不变)。
- 排序默认总分降序;前端 /board 的列头排序与 family 筛选在此一份数据上客户端完成,端点不接 sort/family 参数。

## /board 公开页(ticket 81)

- **路由** `/board`,`meta.public`;组件 BoardView(新),复用 Leaderboard 组件:父级持完整 report,`query` 事件**本地处理**(排序/筛选纯函数,不再请求)——Leaderboard 组件本身零改动或近零改动。
- **客户端排序纯函数** `sortRows(rows, key)`:分数降序、null(未判分)沉底(与服务端 unscored-last 同口径)、同分按 model_id 字典序稳定;family 筛选 `row.family` 精确匹配;`familyOptions` 来自未筛选全集。
- **保存图片**:复用 EvalShareDialog/EvalCard 纯客户端生成(ticket 76/79,无会话依赖),文案「保存图片」(读者是接收方,shared 口径);快照 = 当前排序/筛选生效后的所见。
- **运行中提示行**:running=true 时榜单上方一行 `--hs-text-sm` secondary「新一批评估进行中,当前展示已完成批次 #N」;无底色无边框不轮询。
- **空态**:无 settle 批次 → 空态 + 「暂无已完成的评估批次」;running=true 时附提示行。
- **AppHeader**:未登录导航新增「评估榜单」→ /board(状态总览 / 评估榜单);**/board 页 header 不渲染登录按钮**,页脚 hairline + text 小字「管理登录」→ /login(`--hs-text-xs` placeholder,右对齐或居中,落地定稿);~~状态板(header 登录按钮)维持现状~~(**已被 ticket 84 修订**,2026-07-28 用户裁决:醒目登录按钮对公开读者传递「内容要账号」的错误信号,所有公开页——状态总览/EndpointDetail//board——统一无 header 登录按钮,登录入口一律走页脚「管理登录」共享组件,居中落地定稿)。
- 行不可点(无下钻);涨跌列、前 3 名、水印等矩阵既定口径全部沿用。

## 榜单美化三件套(ticket 82)

页面与物料同源(Leaderboard/ScoreCell 层改,/eval、/board、EvalCard 自动一致):

1. **表格化精致:** 列头下 1px `--hs-border` 分隔(列头 xs secondary,padding-bottom 收 8px);行间 1px `--hs-border-light` 分隔(替代现行纯 gap);行高提到 44–48px(落地定稿)。
2. **轨道中性化:** ScoreCell/总分条轨道 `--hs-brand-soft` → 中性灰(候选 `--hs-border-light` / `--hs-bg-hover`,亮暗双主题观感校准后定稿回写 §5);档色(绿/黄/红)更跳,整页色彩克制。
3. **前三名仪式感:** 行左侧 3px `--hs-brand` 竖条(描边语言,**不用背景色块**——工具风「轻阴影靠描边分层」纪律)+ 名次数字升一档(`--hs-text-lg` 600);teal 名次保留。EvalCard 物料行同步(页面/物料同构)。

## 对既有规范的修订(ui-guidelines.md,实现票内登记)

- §1 产品形态:公开侧 = 状态板 + **公开榜单页 /board**(双形态更新为「公开两页 + 管理台」)。
- §5 新增「/board 公开榜单页」条目:读者/交互边界(客户端排序筛选/保存图片/无下钻)、运行中提示行、页脚「管理登录」与 header 不放登录按钮的差异登记。
- §5 Leaderboard/ScoreCell 条目:表格化(hairline/行高)、轨道中性化令牌定稿、前三名竖条登记;EvalCard 条目同步前三名竖条。
- AppHeader 导航条目:未登录导航加「评估榜单」。
- spec 0004 分享边界不变(本 spec 不触碰运行中分数边界);ADR 0006 token 分享机制保留(可撤销的定向分享与公开页并存)。

## Testing Decisions

- **80(W1 黑盒,既有接缝):** httptest + stub Hub + 假时钟 + 真 SQLite:匿名 200 无 session;无 settle 批次 → report null;有 settle → 返回最近 settle 批次(多批次取最新);running 标志(pending/running 存在与否);响应不含明文凭证类字段(沿用既有报告序列化断言)。
- **81(vitest):** `sortRows` 纯函数(降序/null 沉底/同分字典序/维度键)、family 过滤;组件级手测:未登录直达 /board、排序/筛选/保存图片、运行中提示行、空态、页脚「管理登录」跳转。
- **82:** 纯样式票,无新测试;手测清单:亮暗双主题 hairline/轨道观感、前 3 名竖条、行高呼吸感、EvalCard 物料同步、无横向滚动回归。
