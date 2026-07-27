---
name: product
description: 产品决策流程:产品形态(状态板/管理台双形态)、读者模型、防作假语义边界、README 对外门面形态。新页面开工前判断读者与形态、写或检查 README 时使用。
---

# 产品决策流程

本 skill 承载 HubScope 的产品层决策,与 frontend skill(实现)互补:本 skill 定「做什么、给谁看」,frontend skill 定「怎么实现」。

## 1. 产品形态与读者(新页面开工前必判)

- **双形态:** 公开状态板(Dashboard、EndpointDetail,无需登录)+ 管理台(EvalCenter、TaskCenter、Admin,需登录)。
- **两类读者:** 状态板读者要「3 秒看懂健不健康」——状态优先,操作入口让位;管理台读者要「高效完成配置与排查」——信息密度优先,操作直达。
- 评审新页面时先判断服务哪类读者,密度与信息层级匹配读者任务。

## 2. 防作假语义边界(产品核心约束)

- 任何呈现汇总结论的导出/分享物料,结论旁必须显式列出统计范围——无筛选标「全部端点」,有筛选逐项列出全部生效条件(一个不漏);零匹配时范围仍需保留且结论用中性「暂无数据」;禁止把局部集合呈现为全局结论,禁止零匹配显示「全部正常」。
- 分数涨跌:跨 Suite 版本断点(题目变更/判分口径变更)一律不显示涨跌箭头,用占位并标注「题目已变更」——禁止把题库变化呈现为模型降级。
- 运行中批次半成品分数不外流(分享页/StatusCard 等对外物料不公开半成品分数,只公开运行状态与判分覆盖)。

## 3. README 对外门面形态

README 是**公开门面**(GitHub 首页),不是内部协作文档。HubScope 是单二进制交付的部署型产品——「下载」=一键部署脚本/二进制,不是 git clone。

- **双语:** 主页 `README.md` = English,`README.zh-CN.md` = 简体中文,两文件头部互链,内容结构镜像同步——改一处必须改另一处。
- **头部构成:** 居中 logo + 项目名 + 一句话定位 + 徽章行(CI、Go version、Go Report Card、Release、License: MIT)。
- **正文只回答三问:** What it does(功能 bullet)→ Get started(Docker 优先、install.sh 其次)→ Build from source + Configuration(仅对外必需 env)→ License 节收尾(MIT)。
- **不进 README 的内容:** 协作规则(AGENTS.md/CLAUDE.md)、领域术语(CONTEXT.md)、spec/ADR 索引、agent 分工、`make hooks`/`make test` 等贡献者命令——这些是协作文档的职责,README 不链接不提及。贡献者信息走 CONTRIBUTING.md(尚未创建,创建前不引用)。
- **怎么用的最小路径:** 启动 → `hubscope admin create` 建首个 super_admin(ADR 0011,硬前提)→ 打开 :8080 登录 → 添加 Hub → 自动发现模型。
- **交付形态事实基线:** 交付物是 Go 单二进制(`make build` → `bin/hubscope`,前端 `web/dist` 经 go:embed 内嵌),无运行时 node 依赖(W8)。「怎么下载」按使用者路径排序:① 预编译二进制(GitHub Releases)② Docker ③ 一键部署脚本(`scripts/install.sh`)④ 从源码构建。写「下载」节前先 `gh release view` 确认当前 release 实际产物。
- **写/检查 README 时:** 以陌生读者视角走读三问;不凭记忆写命令,每条命令读代码或只读执行验证;先给建议 diff/草稿,**等用户确认后才用 Edit 落盘**;只改 `README.md`/`README.zh-CN.md`,其他文件一律不碰。

## 4. 产品形态决策触发

- 新增 `views/` 视图 → 先过本 skill 判读者与形态,再进 frontend skill;
- 写或检查 README → 用本 skill 第 3 节形态约定;
- 涉及对外物料(StatusCard/分享报告)的结论呈现 → 用本 skill 第 2 节防作假语义边界。

产品形态与 UI 视觉细节(布局/组件/语义色)的完整规范在 .claude/rules/ui-guidelines.md(由 plan agent 维护);本 skill 不复制其内容,只引用。
