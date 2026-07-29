# Spec 0014: 图像协议(images_generation / images_edit)可用性监控

关联:ADR 0012(docs/adr/0012-image-protocols.md)、W3(Endpoint = 模型 × 协议,试通才建)。

## Problem Statement

Hub 上已接入图像类模型(当前实测 `gpt-image-2`、`gpt-image-2-c`),但 HubScope 对它们的监控口径是错误的:

- 发现同步用 chat 契约(`/v1/chat/completions` + "Reply with pong")对图像模型试通,试通成功后建的是 `openai` 协议端点;
- 然而 **chat 路径通 ≠ images 路径通**——Hub 可能把 `/v1/chat/completions` 与 `/v1/images/generations` 路由到不同上游;实测(2026-07-29)两个图像模型在三条路径上全部 503 `No available providers`,HubScope 只能看到「openai 端点挂了」,看不到「图像生成/图像编辑路径挂了」,路径恢复后也无法区分是哪条路径恢复;
- 图像调用的契约形状与 chat 根本不同:无流式、无 TTFT 概念、单次调用 10–60s 起步、返回 `data[].b64_json/url` 而非 choices、**每次调用真实生成图片有真金白银的成本**(gpt-image 系 low 质量约 $0.011/张),沿用 5 分钟 × 双请求的探测模型成本不可接受(约 $95/月/端点)。

状态板读者无法回答「图像生成现在能不能用」,维护工程师无法对图像路径的故障告警。

## Solution

把图像生成与图像编辑建模为两个独立 **Protocol**:`images_generation`(`POST /v1/images/generations`,JSON)与 `images_edit`(`POST /v1/images/edits`,multipart 表单 + 固定测试图)。「Endpoint = 模型 × 协议」承重墙(W3)不动,「试通才建」机制原样复用:

- 发现同步对 capability=image 的模型追加图像协议试通,试通成功才建对应端点;chat 协议试通照旧对全模型开放;
- 图像端点探测轮 = **单次调用**(无双请求对,TTFT 恒 null),请求超时 **180s**;
- 图像端点默认探测间隔 **30 分钟**(复用既有 per-endpoint `interval_seconds` 覆盖机制,端点创建时落默认值,管理员可经既有 PATCH 调整);
- 请求体最小化(`model+prompt+n=1`,最大兼容各家上游),按模型名规则追加省钱参数(gpt-image 系追加 `quality:"low"`);
- 状态机、告警、报表、状态板全部零改动——新端点自动获得红黄绿状态、24h 可用率、延迟曲线与告警能力。

图像质量评估(vision 裁判打分)本期不做。

## User Stories

1. 作为状态板读者,我想在状态板上看到图像模型的「图像生成」端点及其红黄绿状态,以便 3 秒判断图像生成路径当前能不能用。
2. 作为状态板读者,我想把「图像生成」与「图像编辑」看作两个独立端点,以便分辨是生成挂了还是编辑挂了。
3. 作为状态板读者,我想看到图像端点的 24h 可用率分段条与延迟曲线,以便判断图像路径是「刚挂」还是「全天半死」。
4. 作为维护工程师,我想图像端点进入 down/failing 时收到与 chat 端点同语义的告警,以便用同一套值班动作响应。
5. 作为维护工程师,我想图像端点故障的告警防抖、懒状态重建与 chat 端点一致,以便重启后不收到重复告警轰炸。
6. 作为管理员,我想新接入 Hub 的图像模型在下次发现同步后自动出现图像协议端点(试通成功的前提下),以便零手工干预接入监控。
7. 作为管理员,我想非图像模型永远不被发送图像试通请求,以便不产生无意义的试错成本与上游噪音。
8. 作为管理员,我想手工登记的模型若 capability=image 也能获得图像协议试通,以便 Hub 列表之外的图像变体也能被监控。
9. 作为管理员,我想图像端点的探测间隔可以经既有端点 PATCH 接口调整,以便在成本与新鲜度之间自行权衡。
10. 作为管理员,我想图像端点创建时默认带 30 分钟探测间隔,以便不开箱即烧掉约 $95/月/端点的探测成本。
11. 作为管理员,我想图像试通失败的模型不留下永久禁用的占位端点,以便状态板不被无意义条目污染(沿用 ticket 17 语义)。
12. 作为管理员,我想图像模型曾经试通过的 chat 协议端点保留现状,以便真实可用的 chat 调用路径继续被监控。
13. 作为维护工程师,我想图像端点每轮探测只产生一条探测记录(非流式、TTFT 为空),以便探测历史诚实反映「图像调用无流式概念」的事实。
14. 作为维护工程师,我想图像探测的请求超时独立于 chat 的 60s(180s),以便生成排队/冷启动不被误判为不可用。
15. 作为维护工程师,我想图像探测成功判定为「HTTP 200 且 data 非空、每项含 b64_json 或 url」,以便上游返回 200 但空体/错体时被判为失败。
16. 作为管理员,我想 gpt-image 系模型的探测请求自动带 `quality:"low"` 等省钱参数,以便监控成本最小化。
17. 作为管理员,我想未知图像模型的探测请求只含 model+prompt+n=1,以便不因各家参数方言差异(尺寸写法、quality 支持与否)导致探测本身失败。
18. 作为管理员,我想新增省钱参数规则走既有分类规则同款机制(可配置、不写死),以便新图像模型上架时无需改代码。
19. 作为维护工程师,我想 images_edit 探测使用系统内置的固定测试图,以便编辑路径的探测不依赖任何外部素材。
20. 作为维护工程师,我想图像端点的 token 用量在上游返回 usage 时照常落库,以便用量统计口径与 chat 端点一致。
21. 作为状态板读者,我想图像协议端点的协议 tag 不占用红黄状态色、不与 anthropic/openai 配色混淆,以便一眼区分契约类型。

## Implementation Decisions

- **Protocol 枚举扩展**:`protocol` 取值由 `anthropic`/`openai` 扩为四值,新增 `images_generation`、`images_edit`(命名对齐 URL 名词,ADR 0012)。`endpoints.protocol` 为无约束 TEXT,**无需 schema 迁移**;CONTEXT.md 词表已更新。
- **hubclient**:新增图像协议调用实现——generations 走 JSON POST,edits 走 multipart POST(内含固定测试图)。成功判定:HTTP 2xx 且响应 `data` 数组非空、每项含 `b64_json` 或 `url`;上游返回 usage 时映射进既有 InputTokens/OutputTokens 字段。请求超时按协议族分派:chat 60s 不变,图像 180s。`call` 的 protocol switch 扩展,未知协议仍报错。
- **探测轮形态**:`prober.RunRound` 按端点协议族分支——chat 协议维持「非流式 + 流式」双记录;图像协议单条记录(streaming=false、TTFT=nil)。`AfterRound` 钩子签名不变(收到单元素切片),告警评估语义不动(W5)。
- **探测间隔**:复用既有 per-endpoint `interval_seconds` 覆盖(scheduler `intervalFor` 已支持),图像协议端点创建时落默认 1800s;全局默认 300s 不变。无 scheduler 改动。
- **试通门控**:discovery 的试通协议列表按模型 capability 计算——全模型照旧试 `anthropic`/`openai`;仅 capability=image 的模型追加 `images_generation`/`images_edit` 试通。试通失败不建端点、只记日志(ticket 17 语义不变)。
- **省钱参数规则**:探测请求体默认 `{"model","prompt","n":1}`;按模型名匹配的规则表追加参数(首条规则:gpt-image 系 → `quality:"low"`)。规则机制与 classifier 分类规则同款(库存规则、管理台可配),不把供应商方言写死在代码里。prompt 固定为简单生成指令。
- **images_edit 测试图**:内置固定测试图(小尺寸纯色 PNG),以 Go embed 进二进制(与 W8 单二进制交付一致),不依赖外部素材;multipart 字段按 OpenAI images edits 契约(`image` + `prompt` + `model`)。
- **存量数据处理**:`gpt-image-2`/`gpt-image-2-c` 既有 `openai` 协议端点**保留不动**(代表真实试通过的 chat 路径);本 spec 上线后的首次发现同步会为它们补试图像协议。
- **前端改动(小)**:协议 tag 配色映射扩展——`images_generation`/`images_edit` 用中性 info tag(不占用红黄状态色、不与 anthropic=success/openai=warning 混淆),协议词仍显示原值;改动前过 plan agent UI 评审(设计规范硬性要求)。状态板其余部分(状态灯、分段条、延迟曲线、详情页)零改动,暗色主题自动生效。
- **开放验证项(实现前置)**:Hub 图像上游当前全 503,成功响应真实形状(data 字段、usage 结构、edits 对测试图尺寸的接受度)未能实测。成功判定按 OpenAI 官方契约编写;上游恢复后需实测一轮校准(若 `data[0]` 结构有出入,回头修正判定,属实现内调整,不改 spec 语义)。
- **告警/状态机/报表**:零改动。图像端点自动进入既有红黄绿状态推导、24h 三档可用率、延迟 P50 基线降级检测(自校准,图像延迟量级不同不影响)与告警链路。

## Testing Decisions

- **唯一接缝 W1,不新增接缝**(已与用户确认):全部行为测试走 `internal/server` HTTP API 层 + stub Hub + 假时钟 + 真 SQLite 临时库,不 mock 内部模块、不断言内部状态。
- **stub Hub 扩展**:既有 stub(见 `alerts_test.go` 的 `newStubHubServer`)新增 `/v1/images/generations` 与 `/v1/images/edits` 路由,支持成功(返回合法 data 载荷)与失败(503/空 data/错体)模式切换;edits 路由需真实解析 multipart 以验证测试图确实被上传。
- **被测行为**(均经 API 可观察):
  - 发现同步:capability=image 模型试通成功后出现两个图像协议端点;非图像模型不出现;试通失败不留占位端点;
  - 图像端点创建时 `interval_seconds` 默认为 1800(chat 端点仍为 null);
  - 探测轮:图像端点一轮产生单条 probe(streaming=false、ttft 为 null),chat 端点仍双条;
  - 成功判定边界:200 空 data / 200 错体 / 503 均判失败;200 合法 data 判成功;
  - 状态与告警:stub 切失败模式后图像端点经既有链路出 down/failing 与告警事件(复用 alerts_test 模式);
  - 省钱参数:stub 断言收到的请求体含/不含 `quality:"low"` 按模型名规则分派。
- **好测试的标准**:只断外部可观察行为(API 响应、stub 收到的请求、库中经 API 可读的状态),不断言内部函数调用与私有结构;假时钟推进调度,不等真实时间。
- **先例**:`alerts_test.go`(stub Hub + 模式切换 + 告警链路)、`discovery_test.go`(同步后端点集合断言)、`integration_test.go`(探测轮落库断言)、scheduler 既有假时钟测试。

## Out of Scope

- 图像质量评估(vision 裁判、图像 Suite/Case)——留作后续方向,评估域(W7)本期零改动;
-  retiring 或改造图像模型存量 `openai` 协议端点;
- 各家上游图像 API 方言的完整适配层(仅做最小请求体 + 规则化省钱参数;Hub 的 OpenAI 兼容门面是协议边界);
- 图像探测间隔的全局设置项(本期用 per-endpoint 默认值,全局图像间隔设置留待有第二个 Hub/更多图像模型时再议);
- 状态板 UI 除协议 tag 配色外的任何改动;
- `response_format`、尺寸、背景等生成参数的可配置化(探测用固定最小参数)。

## Further Notes

- 成本算术:low 质量约 $0.011/张,30 分钟间隔 ≈ $16/月/端点;两协议 × 当前 2 个图像模型 ≈ $64/月。试通每次成功约 $0.011,失败(503 秒回)零成本。
- 「每一家图像调用都不一样」的应对边界:HubScope 只对自家 Hub 的 OpenAI 兼容门面发言;上游方言由 Hub 归一化,本系统只处理参数级差异(省钱参数规则表)。
- 实测记录(2026-07-29):`/v1/images/generations`(复数,OpenAI 官方路径)与 `/v1/images/generation`(单数)在 Hub 上均转发上游(均 503 而非 404),实现按官方复数路径。
- ADR 0012 已登记协议化决策与理由;CONTEXT.md「Protocol」「Probe」词条已同步。
