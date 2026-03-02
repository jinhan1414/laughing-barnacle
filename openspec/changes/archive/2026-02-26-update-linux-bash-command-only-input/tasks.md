## 1. Specification
- [x] 1.1 定义 `bash` 新参数契约：仅允许完整命令字符串
- [x] 1.2 定义破坏性行为：旧对象参数显式报错，不做静默兼容
- [x] 1.3 通过 `openspec validate update-linux-bash-command-only-input --strict`

## 2. Implementation
- [x] 2.1 调整 `bash` tool schema 与参数解析逻辑，仅接收命令字符串
- [x] 2.2 移除 `timeout_sec`/`working_dir` 的模型侧参数入口，保持运行时默认超时策略
- [x] 2.3 更新执行证据提取逻辑，确保新参数格式下仍可识别写入与回读命令
- [x] 2.4 更新 runtime prompt 与内置 Skill 文案，改为“直接传完整命令”
- [x] 2.5 更新单元测试覆盖新入参与旧格式拒绝场景

## 3. Verification
- [x] 3.1 单测：`bash` 字符串参数可执行，空字符串/对象参数被拒绝
- [x] 3.2 单测：执行证据在新参数格式下可正确提取命令
- [x] 3.3 回归：`go test ./... -timeout 60s`
