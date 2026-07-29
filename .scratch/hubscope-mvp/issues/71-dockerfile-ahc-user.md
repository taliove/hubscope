# 71 — Dockerfile 容器内运行用户 ahc → hubscope

**What to build:** Dockerfile 中容器内运行用户 `ahc`(L23-24 附近,USER 指令与相关 chown)改名为 `hubscope`,与 ticket 70 完成的部署路径命名统一(install.sh 系统用户、compose service、named volume、deployment.md 均已无 ahc 残留,Dockerfile 是全仓最后一处)。改完验证 `docker build` 通过(或至少 Dockerfile 静态审查 + grep 全仓零残留)。注意确认镜像内数据目录 /data 的属主与新用户名一致,避免容器启动因权限写不了 SQLite。

**Blocked by:** 70

**Status:** 已迁移至 GitHub issue #12(2026-07-28 全面切换 GitHub Issues;本地票只读存档)

- [ ] Dockerfile:`ahc` 用户创建与 `USER` 指令 → `hubscope`;相关 chown/chmod 同步
- [ ] 镜像内 /data 属主 = hubscope(容器启动可写 SQLite)
- [ ] `grep -rn "ahc" Dockerfile docker-compose.yml docs/deployment.md scripts/` 零残留(spec/ticket 历史文本除外)
- [ ] `docker build` 通过(本地无 docker 则静态审查 + 说明)
- [ ] `make test` 全绿
