# 76 — 榜单分享卡片(EvalCard 物料 + 三处入口)

**What to build:** 按 spec 0007 给评估榜单加图片分享物料。① 新组件 `EvalCard.vue`(榜单分享卡唯一渲染模板,720px 逻辑宽、2x 导出、恒亮主题):自上而下——品牌区(`--hs-brand` 4px 品牌条 + `--hs-brand-soft` 浅底 + BrandMark + Wordmark + 「评估榜单」`--hs-text-2xl`/600)→ 范围行 chips(`批次 #N · 定时/手动 · 已完成/失败` 恒列;family 筛选/维度视图/非默认排序逐项;涨跌基准 `涨跌较批次 #M` 或不可比原因三分支)→ failed 警示行(仅 failed 批次,与页面 alert 同口径)→ 榜单行(名次 + 模型名截断 + ScoreStackBar 静态模式 + 分数 + 涨跌箭头仅总分视图且基准可比;**封顶 20 行**,超出收尾「另有 N 个模型未列出,详见评估榜单」)→ 页脚(hairline + 左「生成于 YYYY-MM-DD HH:mm」+ 右 origin)。卡片 = 当前所见(批次 + family 筛选 + 排序 + 维度视图高亮全部生效),chips 与全部数字同源(同一份 report 响应)。② `EvalShareDialog.vue`:结构机制对齐 StatusShareDialog——预览(限高滚动)+ 离屏双份捕获 + snapdom 复制/下载 + 复制降级提示(非安全上下文置灰)+ 暗色会话捕获前脱离 `html.dark` 级联;`utils/statusCardImage.ts` 泛化复用(改名 `cardImage.ts` 或保留,开工定),文件名 `hubscope-eval[-scope]-YYYYMMDD-HHmm.png`。③ 入口:Leaderboard 工具条右端 text 按钮(Share 图标),**仅 settle(done/failed)批次渲染**;三处生效——/eval 与控制台报告页文案「分享图片」,公开分享页 `/report/:token` 文案「保存图片」;报告页 header 既有「分享」(铸链接)改名「**复制链接**」消歧。④ `buildEvalCardSnapshot(report, query, viewSuite)` 纯函数 + vitest。空筛选结果:chips 保留 + 中性「暂无匹配模型」,禁止读作「全部正常」。运行中/等待中批次三处均无入口(spec 0004 分享边界不变)。

**Blocked by:** 75(榜单行堆叠条;EvalCard 行复用 `ScoreStackBar.vue` 静态模式,条与页面同源)

**Status:** pending

## 执行顺序(票内多 commit 拆分,单 commit ≤8 文件)

1. **utils commit**:`buildEvalCardSnapshot` 纯函数 + 单测(范围 chips 生成、不可比三分支、failed 警示、封顶 20、空筛选中性态)
2. **物料 commit**:`EvalCard.vue`(构成五区,ScoreStackBar 静态模式复用)
3. **弹窗 commit**:`EvalShareDialog.vue` + statusCardImage 泛化/复用 + 恒亮捕获
4. **入口 commit**:Leaderboard 工具条按钮(settle 才渲染,shared 文案切换)+ 报告页 header「复制链接」改名
5. **规范 commit**:ui-guidelines §5 新增 EvalCard 物料条目 + 入口登记 + 「复制链接」改名登记

## 验收清单

- [ ] EvalCard 五区构成齐全;范围 chips 逐项(无筛选/有筛选/维度视图/不可比基准各一例)与卡片数字同源
- [ ] 榜单行与页面堆叠条同源(ScoreStackBar 复用),含维度视图高亮态;涨跌箭头仅总分视图且基准可比
- [ ] 封顶 20 行 + 超出收尾计数正确;failed 批次警示行;空筛选中性态不读作「全部正常」
- [ ] 入口三处生效且仅 settle 批次可见(运行中/等待中三处均无按钮);控制台「分享图片」/ 公开页「保存图片」/ header「复制链接」文案正确
- [ ] 暗色会话导出 PNG 为亮主题;HTTP 裸 IP 复制置灰 + 降级提示,下载不受影响
- [ ] PNG 内容无裁剪(离屏双份捕获),长模型名截断不溢出,卡片无横向滚动
- [ ] `buildEvalCardSnapshot` 单测全绿;`make test` 全绿;typecheck/build 通过
- [ ] ui-guidelines §5 EvalCard 条目 + 入口 + 改名登记落地

## 风险登记

1. **两个「分享」并存期**:报告页 header 链接分享与工具条图片分享同屏,改名「复制链接」后仍需 check 环节实机确认无歧义
2. 20 行 × 堆叠条的卡片高度可观(~900px+),预览限高滚动 + 离屏捕获是既有模式,但首次在「行内含图形条」物料上验证 snapdom 保真,手测需对比页面/PNG 逐行一致
3. 公开分享页新增「保存图片」是共享面第一个主动作按钮(此前纯只读),确认不引入任何会话接口依赖(纯客户端生成,满足 ADR 0006 不变式)
