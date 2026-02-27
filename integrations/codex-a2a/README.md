# Codex A2A 本地包装

这个目录用于把本机 `codex` CLI 包装成一个最小 A2A Agent，供数字分身联调。

## 目录说明

- `codex_a2a_agent.py`：A2A 包装服务（JSON-RPC）
- `run.ps1`：本地启动脚本
- `register_local_agent.ps1`：通过 JSON API 向当前项目服务登记 A2A agent
- `state/tasks.json`：任务状态持久化文件（自动创建）
- `state/output/`：每个任务的 Codex 输出文件（自动创建）

## 启动

```powershell
cd integrations/codex-a2a
.\run.ps1
```

默认监听：`http://127.0.0.1:9091`

启动脚本会自动解析 `codex.cmd/codex.exe` 并显式传给服务（`--codex-bin`），避免服务进程 PATH 与交互终端不一致导致的 `codex cli not found in PATH`。
同时会先清理旧的 `codex_a2a_agent.py` 监听进程；服务端口使用独占绑定，避免多实例并发监听同一端口导致随机 `EOF/Empty reply`。
`codex` 子进程输出统一按 UTF-8 解码（`errors=replace`），避免 Windows 默认编码导致 `UnicodeDecodeError` 使任务线程异常。

## 当前处理方案（明确约定）

- A2A 任务返回 `status: working/submitted` 时，主服务当轮直接返回“任务仍在执行中（含 task_id）”，不再同轮继续 `a2a__get` 轮询。
- A2A 工具报错（如 `EOF`）时，主服务当轮直接返回错误摘要，不再追加 LLM 二次收尾。
- 该方案用于防止同轮工具回合持续占用请求上下文而触发 `context deadline exceeded`。

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
