# A2A 验收测试数据

## 数据文件

- payloads/01_save_valid_card.json: 正常接入（通过 agent_card_url 自动发现）
- payloads/02_save_unreachable_card.json: 卡片不可达（应显式失败）
- payloads/03_save_manual_with_skills.json: 手动直填接入（不走 card 发现）
- mock-cards/card_missing_skills.json: 缺少 skills 字段（应失败）
- mock-cards/card_empty_skills.json: skills 为空数组（应失败）
- rpc/01_message_send.json: JSON-RPC 提交任务
- rpc/02_tasks_get.template.json: JSON-RPC 查询任务模板
- rpc/03_tasks_cancel.template.json: JSON-RPC 取消任务模板

## 快速执行命令

### 1) 正常接入

```powershell
curl -sS -X POST "http://127.0.0.1:8080/api/a2a/agents/save" -H "Content-Type: application/json" --data-binary "@tests/a2a-acceptance/payloads/01_save_valid_card.json"
```

### 2) 不可达卡片应失败

```powershell
curl -sS -X POST "http://127.0.0.1:8080/api/a2a/agents/save" -H "Content-Type: application/json" --data-binary "@tests/a2a-acceptance/payloads/02_save_unreachable_card.json"
```

### 3) JSON-RPC 提交/查询/取消

```powershell
curl -sS -X POST "http://127.0.0.1:9091/a2a/rpc" -H "Content-Type: application/json" --data-binary "@tests/a2a-acceptance/rpc/01_message_send.json"
```

```powershell
# 将 __TASK_ID__ 替换为实际 task id
```
