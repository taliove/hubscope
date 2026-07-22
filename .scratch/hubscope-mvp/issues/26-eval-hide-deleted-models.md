# 26 — 评估视图隐藏已删除模型

**What to build:** 评估中心的"最新得分对比"不再出现已删除的模型(如 kimi-k1):`GET /api/evals/latest` 过滤掉 models 表中已不存在的 model_db_id。历史数据不物理删除:历史 Eval Run 详情、单模型趋势中该模型仍可见,前端在这些位置给已删除模型打「已删除」徽标(模型已不在 models 表即视为已删除)。

**Blocked by:** None — can start immediately

**Status:** done

- [x] GET /api/evals/latest 不返回已删除模型的行,现役模型不受影响
- [x] 历史 eval run 详情(GET /api/evals/{id})仍可读,结果中 model_id 文本正常渲染
- [x] 前端对比视图无已删除模型;历史/趋势处已删除模型带「已删除」标记
- [x] 黑盒测试:跑评估 → 删除模型 → latest 不含该模型而 run 详情仍可读
