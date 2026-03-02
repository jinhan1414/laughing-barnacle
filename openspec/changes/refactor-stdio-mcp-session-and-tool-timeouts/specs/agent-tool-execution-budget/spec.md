## ADDED Requirements
### Requirement: Tool Call Timeout Budget Must Be Per Call
系统 MUST 将 `2m` 工具执行预算施加在单次工具调用上，而不是整轮聊天请求上。

#### Scenario: Single tool call gets independent timeout budget
- **WHEN** Agent 在一次聊天回复生成过程中执行某个工具调用
- **THEN** 该次工具调用 MUST 拥有独立的 `2m` 超时上下文
- **AND** 其他工具调用不得复用这一预算

#### Scenario: Chat request is not aborted solely by previous tool durations
- **WHEN** 同一轮聊天中前序工具调用已消耗较长时间但尚未触发本次工具调用的独立超时
- **THEN** 系统 MUST 继续执行后续工具调用
- **AND** 不得仅因为整轮请求已运行接近 `2m` 就提前取消后续工具调用
