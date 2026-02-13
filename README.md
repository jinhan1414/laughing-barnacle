# laughing-barnacle

基于 Go 的简化 AI Agent Web 聊天服务：
- 单全局会话（无 session）持续多轮聊天
- Agent 自动压缩上下文（loop）
- LLM 提供商采用 Cerber（按 OpenAI 兼容 Chat Completions 调用）
- 独立日志页展示每次真实 LLM 输入/输出
- 非流式输出

## 目录结构

- `cmd/server`: 程序入口
- `internal/config`: 环境配置与校验
- `internal/agent`: 对话主流程与自动压缩 loop
- `internal/llm`: LLM 抽象
- `internal/llm/cerber`: Cerber 客户端
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

## 测试与构建

```bash
go test ./...
go build ./...
```

## Agent 行为

每次用户发送消息时，Agent 执行最小闭环：
1. 追加用户消息到全局历史
2. 进入自动压缩 loop（达到阈值则触发压缩）
3. 用“摘要 + 最近消息”调用 LLM 生成回复
4. 追加助手回复

压缩与回复的真实调用都会写入日志页。

## 关键配置

- `APP_ADDR`: HTTP 监听地址
- `CERBER_BASE_URL`: Cerber 服务地址
- `CERBER_API_KEY`: Cerber API Key（必填）
- `CERBER_MODEL`: 默认模型
- `CERBER_TEMPERATURE`: 采样温度
- `CERBER_TIMEOUT`: LLM 请求超时
- `AGENT_MAX_RECENT_MESSAGES`: 回复时最多携带的最近消息数
- `AGENT_COMPRESSION_TRIGGER_MESSAGES`: 消息数触发压缩阈值
- `AGENT_COMPRESSION_TRIGGER_CHARS`: 字符数触发压缩阈值
- `AGENT_KEEP_RECENT_AFTER_COMPRESSION`: 压缩后保留最近消息条数
- `AGENT_MAX_COMPRESSION_LOOPS`: 每轮用户请求最大压缩循环次数
- `APP_LLM_LOG_LIMIT`: 内存日志上限
