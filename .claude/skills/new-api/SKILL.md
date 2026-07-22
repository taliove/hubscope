---
name: new-api
description: 新增后端接口的标准流程:契约先行 → dto → 路由 → 黑盒测试 → 门禁。加 HTTP endpoint 时使用。
---

# 新增接口流程

1. **契约先行**:在对应 ticket / spec 中写明方法、路径、请求/响应 JSON、错误码、鉴权档位(公开 GET / 需登录 / 需管理员,见 ticket 16 分档)。
2. **影响分析**:调 `architect` 代理,确认不动承重墙(尤其 W1 测试接缝、W6 凭证边界);dto 字段是否与前端 `web/src/api/types.ts` 冲突。
3. **TDD**:先在 `internal/server/` 写黑盒测试(httptest + stub Hub + 假时钟 + 真 SQLite 临时库),断言状态码与响应体,不断言内部状态。
4. **实现**:dto 进 `dto.go`(或对应领域文件),handler 进领域文件,路由注册进 `server.go`;三态可选字段用 `json.RawMessage` 区分 absent/null(既有惯例)。
5. **安全自查**:输入边界校验;错误信息不泄漏内部细节;写接口过鉴权中间件;敏感字段(token、webhook)不回明文。
6. **前端同步**:更新 `web/src/api/types.ts` 与对应 api 模块,过 `frontend-checker`。
7. **三层测试**:`test-verifier` 跑当前功能层 → 关联层 → `make test` 闭环层。
8. **审查**:`code-reviewer` 双轴审查后英文 commit。
