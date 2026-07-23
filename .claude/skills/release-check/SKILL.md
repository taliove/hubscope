---
name: release-check
description: 发布/打包前检查清单:全量门禁、版本、部署产物、环境变量与代理。打包、打 tag 或部署前使用;执行动作仍需用户明确指令。
---

# 发布检查清单

发布动作(push、tag、部署)只在用户明确指令后执行;本清单只做检查与准备。

1. **门禁全绿**:`make test` 通过;`git status` 干净;commit 历史全为英文 Conventional Commits。
2. **打包**:`make package`(单二进制 + Dockerfile + 部署文档 tar 包);本机无 Docker,Dockerfile 只做静态检查,如实告知未实际构建镜像。
3. **部署要件核对**(docs/deployment.md):
   - 首个 `super_admin` 用 `hubscope admin create` CLI 引导(不读环境变量、不写入任何文件);`DATA_PATH`、`ADDR`、`LOG_LEVEL` 经环境变量;
   - 出网代理:目标环境若有 fake-ip 类代理,必须设 `HTTPS_PROXY`,启动日志首行会打印生效代理,核对之;
   - 反向代理后设 `TRUST_PROXY=true`(否则限流/审计取不到真实 IP)。
4. **数据面**:目标机的 app.db 备份;schema 自动迁移只加不删,升级无需手工 SQL,但降级不支持——提示用户确认。
5. **配置面**:飞书 webhook、裁判模型等 settings 在管理后台核对(不入库外任何地方)。
6. **发布后验证**:服务起后核对"建 Hub → 同步模型 → 探测 → 状态/告警 → 评估"核心闭环各一项真实动作。
