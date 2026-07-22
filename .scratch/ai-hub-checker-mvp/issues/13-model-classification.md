# 13 — 模型双维度分类 + 可配置规则

**What to build:** 模型分类从单一 capability 扩展为双维度:capability(chat/embedding/image/tts/rerank/moderation 等能力细分)与 family(qwen/deepseek/gpt/claude/glm/llama/gemini 等厂商系列)。识别规则入库:classification_rules 表(维度/关键词/目标分类/优先级),代码内置一套覆盖常见厂商与能力的默认规则,首次迁移时种入,之后以数据库为准;管理页新增"分类规则"区块,表格化增删改;规则保存即对全部模型重算分类;手动添加模型与自动发现走同一套规则;识别不到归 other。模型 DTO 携带两个分类字段。

**Blocked by:** 无

**Status:** todo

- [ ] 模型同时带 capability 与 family 两个分类,新增/发现/手动添加都按规则识别
- [ ] 默认规则首次启动种入数据库,覆盖常见厂商与能力关键词
- [ ] 管理页表格化增删改规则,保存后立即全量重算存量模型分类
- [ ] 无法识别的模型归为 other,不出错、不阻塞同步
- [ ] 黑盒测试:stub Hub 返回多样模型 ID,经 API 断言双维度分类正确;改规则后重算生效
