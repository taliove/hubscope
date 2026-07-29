# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

- **状态板读者**(公开,含未登录):要「3 秒看懂健不健康」;可能投屏/路过/远距看屏;状态优先,操作入口让位。
- **管理台读者**(需登录):要高效完成配置与排查(Hub/模型/评估/任务/用户);信息密度优先,操作直达。
- **外部接收者**(未登录):经 /board 公开榜单、报告分享链接、StatusCard/EvalCard 图片物料接触产品;判断「服务稳不稳、哪个模型好」。(2026-07-29 确认为第三受众)

## Product Purpose

对内监控:定期对 AI Hub(模型网关)上接入的所有模型做可用性探测(Probe)与质量评估(Eval),回答两个问题——**现在健不健康**(状态板)、**哪个模型好**(评估榜单/报告)。成功 = 告警可信(不误报、不轰炸)+ 分数跨时间可比。

## Positioning

防作假是立身之本:Case 不可变 + Suite 版本化 + 绝对分制(不做 Elo),评估结果跨时间可比,供应商无法靠换题库或相对分制美化数字;汇总结论必须标注统计范围,禁止把局部呈现为全局;半成品分数不外流。邻近产品做不到:通用监控(Grafana/UptimeRobot 类)不做模型质量评估;通用跑分榜不做可用性探测与告警防抖。

## Operating Context

单二进制交付(Go embed 前端),内网 systemd/nginx 部署;桌面浏览器优先,状态板常被投屏/截图(主题确定性优先,默认亮主题、不跟随系统);亮暗双主题一等公民。公开页无需登录,管理台走会话认证。

## Capabilities and Constraints

- 监控最小单位 Endpoint = Model × Protocol;状态机:正常/降级/宕机 + failing 告警(唯一允许动画的语义)。
- 评估域词汇:Suite / Case / Verdict / Eval Run / Eval Campaign / Leaderboard / Report / Share Link(权威定义见 CONTEXT.md)。
- 单二进制、无运行时 node 依赖;SQLite 单连接;界面一律简体中文;桌面优先,窄屏不阻断即可。
- 未决:无障碍(a11y)基线未建立,已立批次后排(2026-07-29 记录为开放事实)。

## Brand Commitments

HubScope 名称 + BrandMark(瞄准镜字形:圆环 + 十字准星刻度 + 中心脉冲点,监控隐喻)+ Wordmark(系统等宽字栈,字重 700,完全静止);BrandMark 永不裸用,永远与 Wordmark 同场出现;电波青(teal)品牌色,源自 ProxyHub 品牌体系(ticket 73 并入),与 ProxyHub 的区分由图形标承担。

## Evidence on Hand

生产实例运行中(v0.2.5,2026-07 上线);设计体检基线快照 `.impeccable/critique/`(2026-07-29,三页)。无对外营销物料、无用户评价、无第三方跑分——未来工作禁止虚构这些证据。

## Product Principles

1. **状态优先于操作**——公开侧 3 秒承诺,严重度驱动信息组织。
2. **防作假高于美观**——结论标注统计范围、null 断线不编造、失败不冒充空态、半成品不外流。
3. **单二进制简单部署**——零运行时依赖,前端嵌入交付。
4. **一处事实源**——同一份知识只写一次,词表/格式/映射集中。
5. **中文界面、工具风克制**——灰阶为主、用色克制、无营销装饰。
