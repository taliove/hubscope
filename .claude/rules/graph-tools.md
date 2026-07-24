# Graph Tools(code-review-graph 使用纪律)

> 本项目已构建 code-review-graph 知识图谱。**探索代码时先用 graph 工具,后用 Grep/Glob/Read**——graph 更快、更省 token,且能提供文件扫描给不出的结构上下文(调用方、依赖方、测试覆盖)。

## 何时优先用 graph

- **探索代码**:`semantic_search_nodes_tool` 或 `query_graph_tool`,代替 Grep。
- **理解影响**:`get_impact_radius_tool`,代替手工追 import。
- **代码审查**:`detect_changes_tool` + `get_review_context_tool`,代替整文件通读。
- **追溯关系**:`query_graph_tool`(callers_of / callees_of / imports_of / tests_for 等 pattern)。
- **架构问题**:`get_architecture_overview_tool` + `list_communities_tool`。

graph 覆盖不到时(具体行内容、非代码文件、graph 未索引的新文件)再回落 Grep/Glob/Read。

## 工具速查

| 工具 | 何时用 |
|---|---|
| `detect_changes_tool` | 审查代码改动——风险评分分析 |
| `get_review_context_tool` | 审查需要源码片段——省 token |
| `get_impact_radius_tool` | 评估改动爆炸半径 |
| `get_affected_flows_tool` | 找受影响执行路径 |
| `query_graph_tool` | 追 callers / callees / imports / tests |
| `semantic_search_nodes_tool` | 按名字/关键词找函数与类 |
| `get_architecture_overview_tool` | 理解代码库高层结构 |
| `refactor_tool` | 规划重命名、找死代码 |

## 纪律

1. graph 经 hooks 自动增量更新;改动很大或怀疑索引过期时,先跑 `build_or_update_graph_tool` 再查。
2. 审查流程:先 `detect_changes_tool`,再 `get_affected_flows_tool` 看波及路径,用 `query_graph_tool` pattern="tests_for" 核覆盖。
3. graph 是导航工具,不是事实终裁——结论落到代码前仍需读真实源码确认。
