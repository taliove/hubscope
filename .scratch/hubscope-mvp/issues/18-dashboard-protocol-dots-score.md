# 18 — Dashboard 增强:协议分组 + 24h 稳定点小点 + 端点评分

**What to build:** ①overview 增加 by_protocol 聚合(每个端点恰好属于 anthropic/openai 一组),Dashboard 分组切换加"按协议";②每个端点 entry 增加 dots_24h:按小时对齐的 24 个桶(bucket_start/total/failures),由 24h 探测样本分桶而来,EndpointCard 渲染 24 个彩色小点(成功率≥95% 绿、<95% 黄、全失败红、无数据灰),hover 显示该小时详情;③status 包新增确定性扣分制评分(0-100):down 封顶 20、failing 封顶 50、degraded 封顶 80、24h 成功率低于 95% 按差距扣分、P95 超基线 2 倍 -15,无探测数据不评分;entry 带 score 与 score_reasons(中文理由列表),卡片显示彩色分数,悬停 tooltip 展示理由。

**API 契约(前后端共同遵守):**
- overview entry 新增:`score: number|null`、`score_reasons: string[]`、`dots_24h: Array<{bucket_start: string, total: number, failures: number}>`(恒 24 元素)
- overview 顶层新增:`by_protocol: OverviewGroup[]`(结构同 by_family)

**Blocked by:** 无

**Status:** done

- [x] 分组切换支持按协议,组头健康度与其他维度一致
- [x] 端点卡片展示 24 小时逐时稳定点小点及 hover 详情
- [x] 端点卡片展示评分,悬停可见中文评分理由;无数据端点显示"暂无评分"
- [x] 黑盒测试:overview 断言 by_protocol 聚合、dots 分桶、评分值与理由与种子数据吻合
