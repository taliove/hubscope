# 08 — 评估后端:题库 + Evaluator

**What to build:** 模型评估的机器先转起来:内置 4 个 Suite(基础指令遵循/推理数学/代码/中文)的种子题库随迁移入库,Case 带判定方式(规则:精确/正则/包含;或 LLM 裁判 rubric)。管理员经 API 对指定模型 × Suite 手动触发一次 Eval Run:逐 Case 调 Hub 作答,按 Verdict 方式判 0~1 分(裁判默认 claude-opus-4-8 经 Hub,可配;裁判失败该 Case 记未判分而非 0 分),按 Suite 聚合得分。带 non_chat 标签的模型不在候选内。结果(含作答原文、分数、裁判理由、延迟、token)可查。

**Blocked by:** 01 — Walking skeleton:单 Endpoint 手动探测

**Status:** done

- [x] 4 个内置 Suite 及种子 Case 随迁移存在,Case 区分规则判定与裁判判定
- [x] 手动触发 Eval Run 后,结果含每 Case 的作答、分数、verdict 详情,以及 Suite 聚合分
- [x] 规则判定三种方式(精确/正则/包含)判定正确
- [x] LLM 裁判返回分数与理由;裁判调用失败记未判分,不拉低聚合
- [x] non_chat 模型无法被选为评估对象
- [x] 黑盒测试:stub Hub 模拟作答与裁判响应,触发 Run 后经 API 断言分数、聚合与裁判失败分支
