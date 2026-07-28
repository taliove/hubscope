# 88 — 自适应验证码后端:签发端点 + 登录契约 + 双维度触发(TDD at W1)

**What to build:** 按 spec 0012(docs/specs/0012-adaptive-captcha-login.md)决策 1-5、7、8 实现验证码后端全链路。行为:同一来源 IP **或**同一用户名字符串 10 分钟滑动窗口内登录失败 ≥2 次后,该维度后续登录要求验证码(第 3 次尝试起,双维度独立判定、任一命中即要求;不存在的用户名同样计数,防枚举旁路;登录成功双清零)。新端点 `GET /api/auth/captcha`(公开)返回 `{ captcha_id, image(PNG data URI) }`;答案内存 store(高熵随机 id、TTL 5 分钟、**一次性**——任何校验尝试即销毁、硬上限 + 清扫、上限触达拒发 fail-closed)。登录契约:`POST /api/auth/login` 增加可选 `captcha_id` + `captcha_answer`;被要求未携带 → 401 + `captcha_required: true` + 「请完成验证码」;验证码错/过期 → 401 + `captcha_required: true` + 「验证码错误或已过期」;**验码先于验密**;验码失败不计入渐进延迟计数;未要求时携带 captcha 字段忽略;验证码失败计入实例级爆破告警口径(与审计一致)。签发接口独立限流档 20/min/IP(burst 10)。图形码用 Go 自托管库 6 位数字 PNG;**若库依赖外部字体必须 go:embed 内嵌(W8)**。可测试性:captcha store 与签发档经 server option 注入(WithLoginDelay 先例),测试预置答案。**TDD at W1**:spec 测试决策 1-8 先红后绿。

**Blocked by:** 无 — 可立即开工

**Status:** implemented(bc96575,2026-07-28;TDD 先红后绿,9 条新用例全绿;spec 0011 回归 + `make test` 全绿;待 check 三维度验证——由 main 统一派)

## 执行顺序(TDD,小步)

1. **红灯**:spec 测试决策 1-8 的黑盒用例(注入预置答案 store + 毫秒/个位数参数)
2. **绿灯**:captcha store + 签发端点(+ 独立限流档)+ 双维度触发计数 + handleLogin 契约改造(验码先于验密,挂在渐进延迟计数之前)
3. **回归**:spec 0011 全部用例(渐进延迟/爆破告警/限流)零改动通过;`make test` 全绿

## 验收清单

- [x] 同一用户名失败 2 次后,第 3 次不带验证码 → 401 + `captcha_required`;带错误码 → 401 同标记;带正确码 + 正确密码 → 200(TestCaptchaTriggerRequiresAfterTwoFailures)
- [x] IP 维度与用户名维度独立触发(换用户名不清 IP 要求,换 IP 不清用户名要求)(TestCaptchaTriggerDualDimensionIndependent)
- [x] 验证码一次性(同 id 二次校验必败);TTL 过期 → 「验证码错误或已过期」(TestCaptchaOneTimeUse / TestCaptchaTTLExpiry)
- [x] 验码失败不推进渐进延迟档位(spec 0011 语义不被绕过或加倍);验码失败计入爆破告警口径(TestCaptchaFailureDoesNotAdvanceDelay;告警喂点 = handleLogin 验码两分支在同一 loginAlertTracker 实例 record,loginalert.go 零改动)
- [x] 登录成功后该用户名与该 IP 不再要求验证码(直至再次失败累计)(TestCaptchaSuccessResetsBothDimensions)
- [x] 签发接口独立限流档(20/min burst 10)生效,超限 429;store 上限触达拒发 fail-closed(TestCaptchaIssueRateLimited / TestCaptchaStoreFullFailsClosed,503 冻结文案)
- [x] 图形码纯 Go 生成,运行时零外部字体文件依赖(需要则 go:embed)(base64Captcha DriverDigit 硬编码位图字体,实证见备注节;裁决 B 接受 +约 7MB)
- [ ] spec 0011 用例回归(允许最小适配:存量构造点注入零值 WithCaptchaPolicy 禁用,行为语义不变——裁决 A)(已做:login_delay/loginAlert 两基建各一行,`make test` 全绿);check 三维度 PASS(待 main 统一派 check)

## 备注(阶段 1 影响分析裁决登记,2026-07-28)

- **库选型(实证):** base64Captcha v1.3.8。DriverDigit **不加载任何字体文件**——硬编码 11×18 像素位图字体,纯 stdlib;5.6MB TTF 只服务 DriverString/Chinese/Math 且库已自行 go:embed——spec 决策 5「须自做内嵌」的前提不成立。只用 `DriverDigit.DrawCaptcha` + `Item.EncodeB64string` 两个公开 API;store 自写。**禁用库的 RandomId(math/rand 熵不足),id 与答案自产 crypto/rand。**
- **裁决 A(存量测试适配):** 「spec 0011 用例零改动回归」字面不可达——默认策略装入 New() 后,存量延迟/告警用例第 3 次失败即吃 `captcha_required`。**允许最小适配:每个存量 server 构造点加一行 `WithCaptchaPolicy(CaptchaPolicy{})`(零值禁用,WithRateLimits 同先例)**,验收语义为「行为语义不变,仅注入禁用 option」。备选(cmd 装配而非 New() 默认)破坏 loginDelayer/loginAlertTracker 装配先例,否。
- **裁决 B:** 二进制 +约 7MB(5.6MB 为 digit 路径永不执行的嵌入字体死重)接受;vendor-抄写替代维护成本更高,否。
- **裁决 C(词表冻结):** store 满 503 文案「验证码服务暂不可用,请稍后重试」;audit reason 词 `captcha required` / `captcha wrong`——进审计流水的固定词表,定稿即冻结。
- **裁决 D:** 验码失败**不计入**触发计数(计数已 ≥2 无行为差异;「10 分钟无密码失败则自适应解除」恰是 spec 决策 1 语义)。
- **挂点次序(到行):** per-IP 限流 → handleLogin 内验码判定/校验(body decode 后、GetUserByUsername 前)→ failLogin 内 alert record 与 penalize 之间插 `captchaTrigger.recordFailure` → 成功路径 loginDelayer.reset 旁加 `captchaTrigger.reset`。验码失败不调 penalize(零白等)。
- **结构:** captchaTrigger 与 loginDelayer 独立(双 map byIP/byUser + 滑动窗口 + 每维 100k 上限,满则跳过计数不 fail-closed,per-IP 限流兜底);captchaStore 默认上限 10_000(≈1.5MB,单 IP 20/min×5min ≤100 条存活,触顶需 ≥100 个不同源 IP 刷满,503 只落已被要求者头上)。
- **告警喂点:** 验码失败在 failLogin 同一 loginAlertTracker 实例新增 record,W5 单实例不变,loginalert.go 本体零改动。
- **对 89 的契约(已冻结):** login body 增可选 `captcha_id`/`captcha_answer`;签发 `GET /api/auth/captcha` → `{"data":{"captcha_id","image":"data:image/png;base64,..."}}`;错误 `{"error":{"message":"请完成验证码"|"验证码错误或已过期","captcha_required":true}}`,该键仅在验证码相关 401 出现;每次登录提交销毁已用 id,失败后须重新取图。
- **测试注入缝:** `WithCaptchaPolicy`(零值禁用)+ 导出 `NewCaptchaStore(policy)` + `(*CaptchaStore).Seed(id, answer)`(预置答案唯一注入缝,不破 W1);双 IP 靠 WithTrustProxy + XFF;新增读整 error envelope 的 helper(readErrorMessage 只取 message 不够用)。

## 风险登记

1. **契约是 89 的依赖**:`captcha_required` 标记与错误消息文案是本票对前端的契约面,改动须同步 ticket 89
2. **fail-closed 拒发的 UX**:store 上限触达时合法用户也拿不到码(503)——上限与 TTL 要匹配(5 分钟 TTL × 20/min × 合理并发,上限建议万级),登记推算
3. **W6 边界**:答案不进日志/审计;`captcha_required` 双维度独立触发不泄露用户名有效性
4. **图形库选型**:优先零字体依赖或支持内嵌字体的库;引入新 Go 依赖要过 `go mod` 审查(单二进制精神,不引重型库)
5. **与渐进延迟挂点次序**:验码判定/校验必须在延迟计数与 penalize 睡眠之前,否则被要求验证码的用户每次还要白等 2-8s
