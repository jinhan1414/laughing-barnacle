# Codex A2A 本地包装

这个目录用于把本机 `codex` CLI 包装成一个最小 A2A Agent，供数字分身联调。

## 目录说明

- `codex_a2a_agent.py`：A2A 包装服务（JSON-RPC）
- `run.ps1`：本地启动脚本
- `register_local_agent.ps1`：向当前项目服务登记 A2A agent
- `state/tasks.json`：任务状态持久化文件（自动创建）
- `state/output/`：每个任务的 Codex 输出文件（自动创建）

## 启动

```powershell
cd integrations/codex-a2a
.\run.ps1
```

默认监听：`http://127.0.0.1:9091`

## 手动登记到数字分身

确保主服务已启动（默认 `http://127.0.0.1:8080`）后执行：

```powershell
cd integrations/codex-a2a
.\register_local_agent.ps1
```

## 快速验证

```powershell
curl -s http://127.0.0.1:9091/.well-known/agent-card.json
curl -s http://127.0.0.1:8080/api/a2a/agents
```
