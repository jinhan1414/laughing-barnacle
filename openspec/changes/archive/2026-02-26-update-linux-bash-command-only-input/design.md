## Context
`bash` 当前采用对象参数（`command`/`timeout_sec`/`working_dir`），模型在工具调用时经常生成高转义 JSON。  
本次目标是把调用入口收敛到“只传命令本身”，减少格式噪音，优先保证稳定执行。

## Goals / Non-Goals
- Goals:
  - 将 `bash` 入参简化为单命令字符串
  - 保持现有 shell 执行、超时控制、输出裁剪逻辑不变
  - 明确失败语义，拒绝旧格式时返回可诊断错误
- Non-Goals:
  - 不引入双格式长期兼容（对象+字符串并存）
  - 不改动 A2A/MCP 等其他内置工具契约
  - 不调整工具执行结果结构（`exit_code/shell/stdout/stderr`）

## Decisions
### Decision 1: `bash` 仅接收命令字符串
工具参数定义改为字符串语义，模型调用时仅提供完整命令。  
解析层仅接受该格式，空值或非字符串直接报错。

### Decision 2: 旧对象参数显式拒绝
历史 `{"command":"..."}` 参数将返回明确错误（例如“arguments must be command string”）。  
不做静默兼容，避免双协议并存导致调试成本上升。

### Decision 3: 模型侧不再暴露 `timeout_sec/working_dir`
为减少入参复杂度，模型调用面移除可选键；运行时继续使用既有默认超时和当前工作目录策略。  
如后续确需恢复扩展参数，应通过新提案明确引入。

### Decision 4: 提示词与 Skill 文案同步收敛
runtime prompt 与内置 Skill 中“参数键必须是 command”文案统一替换为“直接传完整命令”。  
避免提示词与真实契约不一致。

### Decision 5: 执行证据解析同步适配
`execution_evidence` 从新参数格式提取命令，保证“写入后回读”校验逻辑不退化。

## Risks / Trade-offs
- 风险：已有上下文或历史记录仍按旧对象格式调用。  
  缓解：返回显式错误，促使模型在下一轮按新协议重试。
- 风险：若上游 LLM 工具参数实现不接受字符串 schema，可能出现调用异常。  
  缓解：实现阶段增加覆盖测试，必要时在同一提案内调整为“语义字符串、结构最简”的可运行形式并保持显式约束。
- 取舍：放弃 `timeout_sec/working_dir` 灵活性，换取更低 token 与更稳参数生成。

## Migration Plan
1. 修改 `bash` tool 定义与参数解析器。
2. 同步更新执行证据提取逻辑。
3. 更新 runtime prompt 与内置 Skill 文案。
4. 补齐/更新单元测试并执行回归。
