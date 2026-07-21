# 06 — 飞书告警

**What to build:** Endpoint 连续 3 次 Probe 失败时,向配置好的飞书群机器人 webhook 发送告警(含模型、协议、当前错误摘要);由红转绿时发送恢复通知。webhook 地址与告警开关在设置页维护,无需改配置重启。每次告警事件落库(类型、是否发送成功),同一故障期间不重复轰炸。

**Blocked by:** 03 — 状态机 + 总览 Dashboard

**Status:** done

- [x] 连续 3 次失败触发一次 down 告警,内容含模型/协议/错误摘要
- [x] 由红转绿触发一次 recovered 通知
- [x] 同一故障持续期间不重复发送 down 告警
- [x] webhook 地址与开关可在设置页修改并即时生效;webhook 未配置时跳过发送不报错
- [x] 测试用 stub webhook server 断言:消息内容、发送时机、不重复发送、事件落库
