# laughing-barnacle

基于 Go 的简化 AI Agent Web 聊天服务：
- 单全局会话（无 session）持续多轮聊天
- Agent 自动压缩上下文（loop）
- LLM 提供商采用 Cerber（按 OpenAI 兼容 Chat Completions 调用）
- Agent 工具调用仅通过 MCP（Model Context Protocol）服务
- 支持按 MCP 服务内单工具启用/禁用
- 支持在设置页配置 Agent Skills（可启用/禁用的系统级技能指令）
- 支持在设置页配置 Agent 系统提示词与压缩提示词（保存后即时生效）
- 会话历史持久化，重启后可恢复聊天记录
- 独立日志页展示每次真实 LLM 输入/输出
- 独立设置页管理 MCP 服务与 Skills
- 非流式输出

## 目录结构

- `cmd/server`: 程序入口
- `internal/config`: 环境配置与校验
- `internal/agent`: 对话主流程与自动压缩 loop
- `internal/llm`: LLM 抽象
- `internal/llm/cerber`: Cerber 客户端
- `internal/mcp`: MCP 服务配置存储与工具调用
- `internal/llmlog`: LLM 调用日志内存存储
- `internal/conversation`: 全局对话存储（无 session）
- `internal/web`: Web 路由与页面模板

## 快速启动

1. 配置环境变量（可复制 `.env.example`）：

```bash
cp .env.example .env
# 编辑 .env，填入 CERBER_API_KEY
```

2. 导入环境变量并启动：

```bash
set -a; source .env; set +a
go run ./cmd/server
```

3. 访问页面：
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
- `runtime` 阶段：使用精简运行时镜像，仅包含可执行文件

本地构建与运行：

```bash
docker buildx build --platform linux/arm64 -t laughing-barnacle:local --load .
docker run --rm -p 8080:8080 \
  -v $(pwd)/data:/data \
  -e CERBER_API_KEY=your_api_key_here \
  laughing-barnacle:local
```

说明：
- 容器内默认将 MCP/Skills 配置写入 `/data/settings.json`。
- 容器内默认将会话历史写入 `/data/conversation.json`。
- 容器内默认将 LLM 调用日志写入 `/data/llm_logs.json`。
- 通过 `-v $(pwd)/data:/data`（或命名卷）可在容器重建后保留配置。
- 若不挂载卷，配置与日志只在该容器生命周期内有效。

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
2. 若处于固定休息时段 `00:30-08:30` 且请求非紧急，执行夜间复盘（生活/工作/学习），并尝试自我进化更新系统提示词，然后返回休息提示（强制策略）
3. 若已起床且当天尚未晨间规划，先生成“任务进度回顾 + 今日 Top3 + 能力提升建议”，再继续处理用户请求
4. 进入自动压缩 loop（达到阈值则触发压缩）
5. 用“摘要 + 最近消息”调用 LLM 生成回复
6. 若模型返回工具调用，则仅通过已启用 MCP 服务执行并回填结果，再继续推理
7. 将已启用 Skills 的指令注入系统提示词后生成回复
8. 追加助手回复

此外，服务进程会每分钟触发一次后台“人类习惯”调度：
- 夜间窗口（00:30-08:30）自动执行一次夜间复盘，并尝试更新系统提示词（自我进化）
- 醒来后自动执行一次晨间规划（任务回顾 + 今日 Top 3 + 能力提升）
- 以上均按“每日一次”去重持久化

压缩与回复的真实调用都会写入日志页。

## 关键配置

- `APP_ADDR`: HTTP 监听地址
- `APP_SETTINGS_FILE`: 设置持久化文件路径（含 MCP、Skills、Agent 提示词配置）
- `APP_CONVERSATION_FILE`: 对话历史持久化文件路径
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
- Agent 提示词统一通过设置页管理（单一来源，可编辑、可重置为内置默认）
- `APP_LLM_LOG_LIMIT`: 内存日志上限
