# 90 — 公开页登录入口统一:撤 header 按钮,全部走页脚「管理登录」

**What to build:** 用户裁决(2026-07-28,取代 spec 0010「状态板维持现状」口径):**未登录态下所有公开页 header 一律不渲染登录按钮**——登录按钮对公开读者太醒目,传递「内容要账号」的错误信号。登录入口统一为页脚 hairline + text 小字「管理登录」→ /login(`--hs-text-xs` placeholder,居中,与 /board 既有页脚同构)。范围:状态总览 DashboardView、/board(已有,核对同源)、EndpointDetailView(同为公开页,一并统一)。页脚抽共享组件(如 PublicFooter.vue)三页复用,不三处复制。登录态 header 用户区不变;/login 页自身不加页脚。ui-guidelines §5 /board 条目与 AppHeader 条目修订:从「/board 与状态板登录入口有差异」改为「公开页统一无 header 登录按钮,页脚管理登录」;spec 0010 相应口径标注被本 ticket 修订。

**Blocked by:** 无(81 已交付,页脚模式已有先例)

**Status:** done(2 commit:750ecf0 → f0bfd16;check PASS,LOW-1 规范措辞已收窄——/report/:token 豁免登记)

## 验收清单

- [ ] 未登录:状态总览、/board、EndpointDetail 三页 header 均无登录按钮,页脚均有「管理登录」可点
- [ ] 登录态:header 用户区/退出不变,页脚「管理登录」仍渲染(与 /board 现行为一致)
- [ ] 页脚为共享组件,三页同源;/login 页无页脚
- [ ] ui-guidelines 修订登记(取消「状态板保留 header 登录」差异条款);spec 0010 口径标注修订
- [ ] `make test` 全绿

## 风险登记

1. AppHeader 的 `isPublicBoard` 逻辑改为通用「公开页」判定(按 route meta.public),注意登录态下导航过滤逻辑不受影响
2. EndpointDetail 有自身页脚/底部元素时,页脚插入位置实机核对
