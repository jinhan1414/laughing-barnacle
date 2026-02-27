# laughing-barnacle

基于 Go 的简化 AI Agent Web 聊天服务：
- 单全局会话（无 session）持续多轮聊天
- Agent 自动压缩上下文（loop）
- LLM 提供商采用 Cerber（按 OpenAI 兼容 Chat Completions 调用）
- Agent 工具调用支持 MCP（Model Context Protocol）与内置工具
- 内置工具包含 `linux__bash` 与 A2A（`a2a__send` / `a2a__get` / `a2a__cancel`）
- 支持 MCP `streamable_http` / `sse` / `stdio` 三种连接类型
- 支持按 MCP 服务内单工具启用/禁用
- 支持在设置页配置 Agent Skills（可启用/禁用的系统级技能指令）
- 支持 A2A（Agent2Agent）接入注册与调用（原生内置工具 + 请求式维护）
- 支持统一 MemoryFS 记忆存储（命名空间/目录/文件/分节）
- 支持在设置页配置 Agent 系统提示词与压缩提示词（保存后即时生效）
- 内置两个配置维护 Skill：`mcp-config-maintainer`、`skills-config-maintainer`
- `skills-config-maintainer` 支持通过 `skills.sh` 检索候选技能并做模糊匹配（`/api/skills/catalog/search`）
- 对 Skill/MCP 的写操作要求先给变更计划并等待用户确认
- Skill 采用文件夹接入模式（`APP_SKILLS_DIR/<skill_id>/SKILL.md`）
- 会话历史持久化，重启后可恢复聊天记录
- 独立日志页展示每次真实 LLM 输入/输出
- 独立设置页管理 MCP 服务、Skills 与 MemoryFS 可视化（节点/segment）
- MemoryFS 支持低置信记忆进入 Inbox 审核，确认后写入正式命名空间
- Memory worker 内置维护任务：失败分段重试、trash 清理、children 索引一致性修复
- 提供 API 供数字分身通过 `bash` 查询与检索：`/api/mcp/services`、`/api/skills`、`/api/skills/catalog/search`
- 提供 A2A 接入查询 API：`/api/a2a/agents`、`/api/a2a/agents/read`
- 提供记忆 API：`/api/memory/index`、`/api/memory/read`、`/api/memory/section`、`/api/memory/upsert`、`/api/memory/move`、`/api/memory/delete`、`/api/memory/inbox`、`/api/memory/inbox/review`、`/api/memory/maintenance/run`、`/api/memory/rollback`、`/api/memory/audit`、`/api/memory/metrics`
- 非流式输出

## 目录结构

- `cmd/server`: 程序入口
- `internal/config`: 环境配置与校验
- `internal/agent`: 对话主流程与自动压缩 loop
- `internal/llm`: LLM 抽象
- `internal/llm/cerber`: Cerber 客户端
- `internal/mcp`: MCP 服务配置存储与工具调用
- `internal/memory`: MemoryFS 统一记忆存储与沉淀 worker
- `internal/llmlog`: LLM 调用日志内存存储
- `internal/conversation`: 全局对话存储（无 session）
- `internal/web`: Web 路由与页面模板

## 协议归档

- [A2A 协议归档](./docs/a2a-archive.md)

## 本地开发环境运行（推荐）

### 1. 前置依赖

- Go `1.22+`
- 可用的 `CERBER_API_KEY`（必填，缺失会启动失败）

### 2. 准备环境变量

先复制一份本地配置文件：

```bash
cp .env.example .env
```

然后编辑 `.env`，至少填写：

```env
CERBER_API_KEY=your_real_api_key
```

### 3. 启动服务

#### Windows PowerShell

```powershell
Get-Content .env | ForEach-Object {
  if ($_ -match '^\s*([^#=]+)\s*=\s*(.*)\s*$') {
    [System.Environment]::SetEnvironmentVariable($matches[1].Trim(), $matches[2].Trim(), 'Process')
  }
}
go run ./cmd/server
```

#### macOS / Linux（bash/zsh）

```bash
set -a
source .env
set +a
go run ./cmd/server
```

启动成功后日志会出现类似：`HTTP server listening on :8080`。

### 4. 本地浏览器访问

- 聊天页：`http://localhost:8080/chat`
- 日志页：`http://localhost:8080/logs`
- 设置页：`http://localhost:8080/settings`

## 测试与构建

```bash
go test ./...
go build ./...
```

## Docker（两阶段构建）

项目根目录已提供两阶段 `Dockerfile`：
- `builder` 阶段：使用 Go 镜像编译二进制
- `runtime` 阶段：使用 Debian 运行时，内置常用 Linux 工具，便于 `linux__bash` 工具调用

本地构建与运行：

```bash
docker buildx build --platform linux/arm64 -t laughing-barnacle:local --load .
docker run --rm -p 8080:8080 \
  -v $(pwd)/data:/data \
  -e CERBER_API_KEY=your_api_key_here \
  laughing-barnacle:local
```

说明：
- 容器内默认将 MCP 配置写入 `/data/settings.json`。
- 容器内默认将 Skill 文件写入 `/data/skills`，状态写入 `/data/skills_state.json`。
- 容器内默认将会话历史写入 `/data/conversation.json`。
- 容器内默认将记忆库写入 `/data/memory.db`。
- 容器内默认将 LLM 调用日志写入 `/data/llm_logs.json`。
- 通过 `-v $(pwd)/data:/data`（或命名卷）可在容器重建后保留配置。
- 若不挂载卷，配置与日志只在该容器生命周期内有效。
- 运行镜像内置常用工具：`bash`、`curl`、`wget`、`git`、`nodejs`、`npm`、`npx`、`jq`、`vim`、`nano`、`iproute2`、`net-tools`、`dnsutils`、`procps` 等。

## CI/CD 自动构建并推送镜像

已添加 GitHub Actions 工作流：`.github/workflows/docker-image.yml`

触发规则：
- `push` 到 `main`：自动测试 + 构建 + 推送镜像
- `push` tag（如 `v1.0.0`）：自动测试 + 构建 + 推送镜像
- `pull_request`：自动测试 + 构建校验（不推送）

镜像仓库：
- `ghcr.io/<owner>/<repo>`
- 例如当前仓库是 `foo/bar`，镜像名即 `ghcr.io/foo/bar`

关键说明：
- 工作流使用 `GITHUB_TOKEN` 登录 GHCR
- 需在仓库 Settings 中允许 `packages: write`（工作流内已声明权限）
- 当前阶段仅构建/推送 `linux/arm64` 镜像
- 默认会生成分支、tag 和 commit sha 三类镜像标签

## Agent 行为

每次用户发送消息时，Agent 执行最小闭环：
1. 追加用户消息到全局历史
2. 进入自动压缩 loop（达到阈值则触发压缩）
3. 用“摘要 + 最近消息”调用 LLM 生成回复
4. 若模型返回工具调用，则通过 `linux__bash` 或已启用 MCP 服务执行并回填结果，再继续推理
5. 若模型命中 A2A 调用能力，则通过内置 `a2a__send` / `a2a__get` / `a2a__cancel` 执行
6. 将已启用 Skills 的指令注入系统提示词后生成回复
7. 追加助手回复

A2A 执行链路稳定性策略（代码现状）：
- 同轮中若 A2A 返回 `status: working/submitted`，后端直接返回“任务仍在执行中（含 task_id）”，不再继续同轮轮询。
- 同轮中若 A2A 工具报错（如 EOF），后端直接返回错误摘要，不再追加一次 LLM 收尾调用。
- 目的：避免同轮反复 `a2a__get` 把 `/chat/send` 的 2 分钟上下文耗尽并触发 `context deadline exceeded`。

此外，服务进程启动后会注册后台 Cron 调度：
- 所有任务由设置中心的 Cron 表达式驱动（5 段：分 时 日 月 周），默认不内置任务，由用户按需创建
- 执行结果会记录任务的最近运行时间与状态，并写回设置存储
- 可通过设置页“定时任务”统一查看、编辑、立即执行，Agent 也可通过 `/api/schedules` 与 `/settings/schedules/*` 接口按确认流程自动维护
- 并发策略：同一任务执行中再次触发会标记 `skipped/already_running`；不同任务即使同一时刻触发也会串行执行，避免冲突

压缩与回复的真实调用都会写入日志页。

## 关键配置

- `APP_ADDR`: HTTP 监听地址
- `APP_SETTINGS_FILE`: 设置持久化文件路径（含 MCP 与 Agent 提示词配置）
- `APP_SKILLS_DIR`: Skill 文件夹路径（目录内每个 Skill 以 `SKILL.md` 存储）
- `APP_SKILLS_STATE_FILE`: Skill 状态文件路径（启用状态、来源、更新时间）
- `APP_CONVERSATION_FILE`: 对话历史持久化文件路径
- `APP_MEMORY_FILE`: 记忆持久化文件路径（bbolt）
- `APP_LLM_LOG_FILE`: LLM 调用日志持久化文件路径
- `CERBER_BASE_URL`: Cerber 服务地址
- `CERBER_API_KEY`: Cerber API Key（必填）
- `CERBER_MODEL`: 默认模型
- `CERBER_TEMPERATURE`: 采样温度
- `CERBER_TIMEOUT`: LLM 请求超时
- `MCP_HTTP_TIMEOUT`: MCP HTTP 调用超时
- `MCP_PROTOCOL_VERSION`: MCP 协议版本（默认 `2025-06-18`）
- `MCP_TOOL_CACHE_TTL`: MCP 工具列表缓存时长
- `AGENT_MAX_RECENT_MESSAGES`: 回复时最多携带的最近消息数
- `AGENT_COMPRESSION_TRIGGER_MESSAGES`: 消息数触发压缩阈值
- `AGENT_COMPRESSION_TRIGGER_CHARS`: 字符数触发压缩阈值
- `AGENT_KEEP_RECENT_AFTER_COMPRESSION`: 压缩后保留最近消息条数
- `AGENT_MAX_COMPRESSION_LOOPS`: 每轮用户请求最大压缩循环次数
- `AGENT_MAX_TOOL_CALL_ROUNDS`: 单轮对话最大工具调用回合数
- `AGENT_MEMORY_IDLE_WINDOW`: 记忆分段不活跃关闭窗口（默认 `5m`）
- `AGENT_MEMORY_MAX_SEGMENT_WINDOW`: 单分段最大持续时间（默认 `10m`）
- `AGENT_MEMORY_MAX_SEGMENT_MESSAGES`: 单分段最大轮次数（默认 `8`）
- `AGENT_MEMORY_WORKER_INTERVAL`: 记忆沉淀 worker 扫描周期（默认 `30s`）
- `AGENT_MEMORY_TRASH_TTL`: `/inbox/trash` 过期清理周期（默认 `720h`）
- `AGENT_MEMORY_FAILED_RETRY_AFTER`: failed segment 进入重试的最短等待时间（默认 `2m`）
- `AGENT_MEMORY_EXTRACTION_USE_LLM`: 是否启用 LLM 结构化提取（默认 `true`）
- `AGENT_MEMORY_EXTRACTION_FALLBACK`: LLM 提取失败后是否回退规则提取（默认 `true`）
- `AGENT_MEMORY_EXTRACTION_MODEL`: 记忆提取模型（默认复用 `CERBER_MODEL`）
- `AGENT_MEMORY_EXTRACTION_TEMPERATURE`: 记忆提取温度（默认 `0`）
- Agent 提示词统一通过设置页管理（单一来源，可编辑、可重置为内置默认）
- `APP_LLM_LOG_LIMIT`: 内存日志上限
