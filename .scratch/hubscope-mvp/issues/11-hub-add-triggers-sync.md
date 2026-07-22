# 11 — 添加 Hub 即同步 + 同步状态可见

**What to build:** 添加 Hub 后立即在后台对该 hub 跑一次模型同步(不再等每小时定时):POST /api/hubs 创建成功后异步触发单 hub 同步,响应不阻塞。hub 持久化同步状态(sync_status: idle/syncing/succeeded/failed、last_synced_at、last_sync_error),每次同步(添加触发/定时/手动)都更新;前端 Hub 列表展示同步状态与最近同步时间,同步中轮询。新增 POST /api/hubs/{id}/sync 手动触发单 hub 同步(已在 syncing 时返回 409),Hub 列表每行加"立即同步"按钮;原全量同步入口保留。

**Blocked by:** 无

**Status:** done

- [x] 添加 Hub 后无需等待定时周期,模型自动出现;API 响应不被同步阻塞
- [x] Hub 列表可见每个 hub 的同步状态(同步中/成功/失败+原因)与最近同步时间
- [x] 单 Hub "立即同步"按钮触发该 hub 同步,同步中重复触发返回 409
- [x] 同步失败(如 token 错误)状态落库并在 UI 可见,不影响其他 hub
- [x] 黑盒测试:stub Hub,经 API 断言添加 hub 后异步同步完成、状态流转正确、重复触发 409
