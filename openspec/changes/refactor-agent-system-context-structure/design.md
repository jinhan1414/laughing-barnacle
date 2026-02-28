## Context
当前系统上下文由多个来源拼接：
- `DefaultSystemPrompt` 承载角色与回答策略；
- `responseStylePrompt` 在请求构建阶段再次注入；
- `buildToolRuntimePrompt` 承载工具环境、接口约束与执行硬规则；
- Skill/A2A/Memory/Async 索引采用渐进式披露按需读取详情。

该结构可用，但存在三类问题：
1. 规则分散且重复，模型注意力被冗余文案稀释；
2. `linux__bash` 的命名与 Windows PowerShell 执行现实存在语义冲突；
3. 维护写入在命令行层面需处理多层 JSON 转义，失败率高。

## Goals / Non-Goals
- Goals:
  - 让系统上下文结构模块化、可扫描、无重复。
  - 让模型明确 shell 真实执行环境，降低 Linux 语法误用。
  - 让维护写入提示统一到 JSON 协议与低转义路径。
  - 保持执行证据约束与渐进式披露原则，减少无效工具回合。
- Non-Goals:
  - 不新增 `get_skill_detail` 等原生内置工具。
  - 不将 `linux__bash` 改名为 `windows__powershell`（避免破坏现有契约与兼容链路）。
  - 不引入关键词/正则分流的决策逻辑。

## Decisions
- Decision: 上下文固定分块并保持顺序稳定
  - 统一块级结构，避免“同一策略在 system 多处重复”。
  - 将“回答策略”保留单一权威段落，移除同义重复注入。
  - 原因：提升可读性与 token 利用率，同时有利于请求前缀稳定。

- Decision: 保留 `linux__bash` 名称，但新增强制环境警戒
  - 在 Windows 场景显式声明“工具名为 `linux__bash`，但命令在 PowerShell 执行”。
  - 明确禁止输出 Bash 专属语法并固定 `curl.exe` 规则。
  - 原因：兼容现有工具契约，同时降低模型望文生义风险。

- Decision: 维护写入提示统一为“JSON 协议 + 宿主友好构造”
  - 路由约束集中声明在统一段落；
  - PowerShell 写入默认 `Invoke-RestMethod + ConvertTo-Json`，避免手写深层转义。
  - 原因：降低命令拼接复杂度与失败概率。

- Decision: 渐进式披露策略保持不变但收敛读取预算
  - 继续使用 `linux__bash` 调本地 API 读取详情；
  - 单轮默认只拉取 1 个最相关详情，不足再补读。
  - 原因：遵守当前架构约束并控制工具轮次成本。

## Risks / Trade-offs
- 风险：未改工具名，仍可能有少量语义误触发。
  - Mitigation：在运行时提示中把“环境警戒 + 语法边界”放到高优先级硬约束。

- 风险：上下文重排可能影响既有缓存命中。
  - Mitigation：固定段落顺序与标签，更新对应缓存稳定性测试。

- 风险：约束过强可能压制模型自由表达。
  - Mitigation：只约束执行/接口/证据，不约束业务答案格式。

## Migration Plan
1. 先改 OpenSpec 规范，明确新上下文结构与契约。
2. 再按规范重构 `DefaultSystemPrompt`、`responseStylePrompt`、`buildToolRuntimePrompt` 的分工。
3. 同步更新 Skill 文案中与 JSON 写入、PowerShell 约束相关表述。
4. 通过单测/集成测试验证工具调用稳定性与请求前缀顺序。

## Open Questions
- 无。
