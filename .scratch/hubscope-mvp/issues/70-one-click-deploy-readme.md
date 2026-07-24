# 70 — README 修正 + 一键部署脚本 + Docker compose(spec 0006)

**What to build:** 按 spec 0006 落地使用者获取路径。① `scripts/install.sh`:一键部署成 systemd 服务(依赖检查→make build→装 /usr/local/bin→建 hubscope 用户与 /var/lib/hubscope→落 systemd unit→enable --now→健康检查→打印 admin create 引导),幂等(重跑=升级),env 可覆盖(HUBSCOPE_PREFIX/DATA_DIR/USER/PORT);② `docker-compose.yml`:build+8080+named volume hubscope-data+restart;③ docs/deployment.md 改脚本优先、unit 以脚本模板为唯一事实、清 ahc/ahc-data 旧名、Docker 节改 compose 优先;④ README.md 新增「下载与部署」节(脚本→Docker→源码构建)+ 修正 5 能力 Suite 词表 + 补 make hooks + 数据落点 + 配置表指向 deployment.md + bcrypt 去 cost 10;⑤ Makefile package 纳入 install.sh、lint 纳入 shellcheck(无 shellcheck 降级警告不卡门禁);⑥ 测试:install_test(临时目录+env 覆盖 PREFIX+stub systemctl 验证幂等/首装/缺依赖报错)、compose config 合法性、go test ./cmd/... 回退。

**Blocked by:** 无(spec 0006 已立)

**Status:** done(2026-07-24,commits 6a51023 / 85adf49 / 9bb72dd)

- [ ] `scripts/install.sh`(新):set -euo pipefail;env 覆盖四变量;依赖检查缺 Go/pnpm 明确报错;幂等每步先查后做;systemd unit 内嵌 heredoc 模板(User=hubscope、Environment=DATA_PATH/ADDR、Restart=on-failure、基本加固);健康检查轮询 /api/overview 30s;引导输出含 admin create 示例与访问地址
- [ ] `docker-compose.yml`(新,根目录):单 service build:.,ports 8080,named volume hubscope-data→/var/lib/hubscope,restart unless-stopped
- [ ] `scripts/install_test.sh` 或 Go 黑盒测试(新):临时目录+stub systemctl;断言首装落文件/重跑幂等数据不动/缺依赖明确报错
- [ ] `Makefile`:package 产物含 scripts/install.sh;lint/test 加 shellcheck(缺失时警告不 fail)
- [ ] `docs/deployment.md`:推荐路径改 install.sh;ahc→hubscope、ahc-data→hubscope-data;Docker 节 compose 优先;unit 描述指向脚本模板不复制全文
- [ ] `README.md`:新增「下载与部署」节(install.sh→Docker→源码构建三路径);「4 个内置能力 Suite(基础指令遵循/推理数学/代码/中文)」→「5 个内置能力 Suite(指令遵循/推理/代码/知识问答/语言理解与生成)」;Development 补 `make hooks`;快速上手补「默认监听 :8080,数据存 ./data/app.db」;配置表加一行指向 deployment.md 完整表;bcrypt(cost 10)→bcrypt 哈希
- [ ] grep 全仓无 ahc/ahc-data 残留(deployment.md、compose、脚本)
- [ ] `make test` 全绿(含 shellcheck 段)
