# 56 — Dashboard 状态分享卡(Status Card)

**What to build:** Dashboard 顶部操作区加「分享状态」按钮,一键把**当前筛选范围内**的状态板生成竖版 PNG 分享卡,`el-dialog` 预览后「复制图片」或「下载 PNG」。纯前端 DOM→PNG(候选库 snapdom,开工时用 Context7 查证最新 API 后再定;html2canvas 对 Element Plus 阴影/复杂选择器还原差,不作首选),不引入任何后端依赖(W8)。卡片为专门设计的品牌物料,非页面截图:

- **品牌区**:logo + 主标题「HubScope 服务状态」,标题区要有品牌设计感(design-owner 评审重点)
- **范围副标题(防作假红线)**:无筛选显示「全部端点」;有筛选逐项列出生效条件(模型名关键词 / 协议 / 状态),一个不漏。结论统计口径 = 筛选后集合,必须显式标注范围,禁止把局部呈现为全局(ADR 0007 防作假语义的镜像)
- **大结论**:沿用 HealthBanner 四态语义与文案(全部正常 / N 个降级 / N 个异常),大字号;不新造状态词
- **生成时间**:「生成于 YYYY-MM-DD HH:mm」,取图片生成时刻(页面轮询保证数据新鲜,不重新拉取,所见即所享)
- **明细区**:异常态列出全部异常 endpoint(名称截断 + 状态词:降级/宕机/告警,严重度排序);正常态显示「全部 N 个端点正常」。分组方式(grouping)不入图
- **页脚**:`location.origin`(看图人可找到实时状态板)

**规格:** 逻辑宽 720px、2x 导出(1440px)、高度随内容;文件名 `hubscope-status-YYYYMMDD-HHmm.png`。「复制图片」走 `navigator.clipboard.write`,非安全上下文(HTTP 裸 IP)置灰并提示「当前环境不支持复制图片,请使用下载」,下载永远可用。边界态:筛选零匹配照常生成空态卡片(与页面空态文案一致);页面首次加载未完成时按钮禁用;生成/复制/下载全程 loading + 失败重试 + `ElMessage` 结果反馈(ui-guidelines §5/§6)。

**Blocked by:** None

**Status:** done

- [x] design-owner 设计评审已通过(有条件):视觉规格见评审结论;ui-guidelines 已回写(§2 导出物料间距、§3 静态物料 failing 双编码与状态词文字着色、§5 StatusCard 登记 + 结论必须标注统计范围、§6 复制降级)
- [x] POC 验证 1:snapdom 对 `:root` CSS 变量、浅底色(`--el-color-*-light-9`)、语义色还原正常(2x 导出 PNG 已目检);发现并修复祖先 overflow 裁剪导出结果的坑(离屏孪生捕获源)
- [x] POC 验证 2:`web/public/logo.svg` 未提交进 git(worktree 内不存在),采用已提交的 logo.png,2x 导出还原清晰
- [x] StatusCard 渲染模板(离屏/弹窗内 DOM,复用 StatusBadge 语义色,不复制状态灯实现)
- [x] DOM→PNG 生成 + 2x 导出 + 文件名规范
- [x] 「分享状态」入口 + el-dialog 预览 + 复制/下载 + 三态(loading/失败重试/ElMessage)
- [x] 复制能力检测与安全上下文降级(置灰 + 提示)
- [x] 范围副标题:三个过滤器逐项列出,无筛选显示「全部端点」
- [x] 边界态:筛选零匹配空态卡片;首次加载中按钮禁用
- [x] CONTEXT.md 已含 Status Card 术语;Report 术语已修正为「可导出 PDF」
- [x] 测试三层:纯前端功能无 API 行为变化,当前功能层以前端类型检查 + 构建 + 手动验收为准;若实现中触及任何 API 则补黑盒测试;`make test` 全量必过
