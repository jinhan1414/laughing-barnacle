# Project Context

## Purpose
`laughing-barnacle` 是一个基于 Go 的 AI Agent Web 聊天服务，目标是为单一所属用户提供长期可演进的数字分身能力。系统采用全局单会话，围绕“对话、工具调用、上下文压缩、长期记忆沉淀、可观测与可维护配置”构建，强调真实执行链路、可追踪、可恢复。

## Tech Stack
- Go 1.22+
- 标准库 `net/http` + 服务端模板页面（聊天/日志/设置）
- Cerber（OpenAI 兼容 Chat Completions）作为 LLM 提供方
- MCP（Model Context Protocol）工具生态，支持 `streamable_http` / `sse` / `stdio`
- bbolt（MemoryFS 记忆持久化）
- 文件持久化（settings、skills 状态、conversation、llm logs）
- Docker 两阶段构建，目标平台固定 `linux/arm64`
- GitHub Actions（测试、构建、推送镜像）

## Project Conventions

### Code Style
- 使用 Go 约定式风格与小函数拆分，优先可读性与可测试性。
- 默认暴露失败，不做静默降级、不引入 mock 成功路径。
- 禁止无意义兼容分支与废弃代码堆积，变更后及时删除不可达逻辑。
- 命名以语义清晰为先，避免魔法数字，关键阈值抽为具名常量或配置。

### Architecture Patterns
- 单体服务分层：`cmd/server` 启动层，`internal/*` 按职责拆分（agent、llm、mcp、memory、conversation、web、config）。
- 对话主链路固定：触发条件 -> 动作计划 -> 工具调用 -> 接口返回 -> 回读校验 -> 最终回复。
- Agent 仅保留 `linux__bash` 作为内置本地工具；扩展能力通过 MCP 服务注入。
- Skill 通过存储层以标准 `SKILL.md` 管理，按“渐进式披露”注入（先索引，按需读取详情）。
- 长上下文采用“结构化归档 + 轻摘要 + 按索引回读”策略，避免一次性注入全文。

### Testing Strategy
- 优先自动化验证：`go test ./...` 作为基础回归检查。
- 关键链路（Agent 执行、工具调用、记忆写入、配置变更）要求可观察、可断言。
- 后端单测执行需控制超时（硬上限 60 秒），避免卡死任务。
- 涉及 Skill 存储与注入规范的变更必须补充对应测试。

### Git Workflow
- 日常在特性分支开发并提交可审阅 commit，提交信息建议使用动词开头的明确描述。
- 完成任务后默认执行 `git commit` 与 `git push`（除非用户明确要求不推送）。
- OpenSpec 驱动需求变更：先提案与校验，再实现，再归档。

## Domain Context
- 产品定位是“单用户数字分身”，不是多租户会话系统。
- 会话是全局单会话，强调长期记忆沉淀与持续进化。
- Skill 调用以 LLM 主动触发为主（显式点名或 description 隐式匹配），不依赖关键词/正则分流核心业务。
- 设置页底部工具栏是统一扩展入口，后续能力优先接入该入口。
- 调度任务由用户配置 Cron 触发，链路需支持执行证据追踪与结果回写。

## Important Constraints
- 前端页面按移动端优先设计，不要求桌面端适配。
- Docker 与 CI 镜像目标架构固定为 `linux/arm64`。
- 非必要不改 system 文本/提示词；优先通过业务逻辑与结构化上下文解决问题。
- 核心业务机制禁止采用关键词/正则匹配分流。
- 单文件不超过 300 行；函数长度、嵌套和复杂度受严格约束。
- 仅在任务确认完成时发送一次 done 通知。

## External Dependencies
- Cerber LLM 服务（`CERBER_BASE_URL`、`CERBER_API_KEY`、`CERBER_MODEL`）
- MCP 外部工具服务（HTTP/SSE/STDIO）
- 本地与容器内 Linux 工具链（供 `linux__bash` 执行）
- GitHub Actions + GHCR（CI/CD 与镜像发布）
