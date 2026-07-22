# 33 — Share Link + 报告导出

**What to build:** Campaign 报告可对外分享:生成带随机 token 的只读链接,无需登录即可打开 /report/{token} 看到该 Campaign 的完整报告(Leaderboard + 趋势);链接可撤销(撤销后 404)。管理页可列出/撤销全部分享链接,创建与撤销走审计。报告可导出图片/PDF(前排依赖做完后实现)。无 token、token 错误、已撤销一律不可读。

**Blocked by:** 31 — Campaign 报告与 Leaderboard

**Status:** ready-for-agent

- [ ] 创建/撤销分享链接;免登录只读路由凭 token 渲染报告
- [ ] 管理页分享链接列表与撤销;审计记录创建/撤销
- [ ] 报告导出图片/PDF
- [ ] 黑盒测试:无登录凭 token 可读、撤销后 404、无 token/错 token 不可读
