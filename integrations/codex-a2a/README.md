# codex-a2a（官方 SDK 版）

该目录提供一个基于官方 `a2a-python` SDK（`a2a-sdk`）的本地 A2A 参考服务，用于数字分身联调。

## 目录

- `codex_a2a_agent.py`：A2A 服务（官方 SDK 实现）
- `requirements.txt`：Python 依赖
- `run.ps1`：启动脚本
- `register_local_agent.ps1`：向主服务登记本地 A2A Agent
- `state/output/`：Codex 执行输出目录（自动创建）

## 依赖

```powershell
python -m pip install -r requirements.txt
```

依赖重点：

- `a2a-sdk[http-server]==0.3.24`
- `uvicorn`

## 启动

```powershell
cd integrations/codex-a2a
.\run.ps1
```

默认监听 `http://127.0.0.1:9091`，暴露：

- Agent Card：`/.well-known/agent-card.json`
- JSON-RPC：`/a2a/rpc`

Agent Card 包含：

- `protocolVersion: 0.3.0`
- 至少 1 个可执行 skill（`codex_exec`）

## 任务生命周期（send/get/cancel）

服务由官方 `DefaultRequestHandler` + `InMemoryTaskStore` 托管任务状态机，执行器为 `CodexAgentExecutor`：

- `message/send`：提交任务并异步执行本地 `codex exec`
- `tasks/get`：查询任务状态与产物
- `tasks/cancel`：终止运行中任务并标记 `canceled`

失败路径保持显式暴露，不提供 mock 成功返回。

## 注册到数字分身

确保主服务已启动（默认 `http://127.0.0.1:8080`）后执行：

```powershell
cd integrations/codex-a2a
.\register_local_agent.ps1
```

`/api/a2a/agents/save` 会读取 Agent Card 并校验 `skills`。
