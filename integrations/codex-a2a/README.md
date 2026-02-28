# codex-a2a（官方 SDK 版）

该目录提供一个基于官方 `a2a-python` SDK（`a2a-sdk`）的本地 A2A 参考服务，用于数字分身联调。

## 目录

- `codex_a2a_agent.py`：A2A 服务（官方 SDK 实现）
- `requirements.txt`：Python 依赖
- `run.ps1`：启动脚本
- `register_local_agent.ps1`：向主服务登记本地 A2A Agent
- `state/output/`：Codex 事件流输出目录（自动创建，保存 `*.events.jsonl`）

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

执行语义：

- 默认注入最小执行前缀（仅约束“持续执行到可交付结果、失败显式暴露”，不约束业务输出结构）
- 默认使用高权限执行参数：`codex exec --dangerously-bypass-approvals-and-sandbox --json`
- 完成判定基于事件流终态证据（`turn.completed` + 最终 `agent_message`）
- 仅进程退出码为 0 不足以判定完成；缺失终态证据会显式失败

工作目录：

- 默认工作目录为启动参数 `--workdir`
- 可在 A2A 请求 `metadata` 中传 `working_dir` 覆盖本次任务目录
- 若提供的 `metadata.working_dir` 不存在、不可访问或不是目录，任务会显式失败（不静默回退）

失败路径保持显式暴露，不提供 mock 成功返回。

## 注册到数字分身

确保主服务已启动（默认 `http://127.0.0.1:8080`）后执行：

```powershell
cd integrations/codex-a2a
.\register_local_agent.ps1
```

`/api/a2a/agents/save` 会读取 Agent Card 并校验 `skills`。
