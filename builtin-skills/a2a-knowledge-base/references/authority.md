# A2A 权威来源

## 使用原则

- 先区分“协议标准”与“本仓库实现”。
- 协议定义优先看官方规范站和官方仓库。
- SDK 用法优先看对应官方 SDK 仓库。
- 历史背景文章只用于说明背景，不作为规范定义。

## 外部权威源优先级

1. `https://a2a-protocol.org/latest/`
2. `https://github.com/a2aproject/A2A`
3. `https://github.com/a2aproject/a2a-go`
4. `https://github.com/a2aproject/a2a-python`

## 本仓库权威源优先级

1. `internal/a2a/*`、`internal/agent/*`、`internal/web/*` 当前代码
2. 相关测试，如 `internal/a2a/provider_sdk_test.go`
3. `integrations/codex-a2a/README.md`
4. `openspec/changes/archive/2026-02-26-add-a2a-native-capability/`

## 回答规则

- 文档与代码冲突时，以当前主干代码和测试为准。
- 若无法从权威源确认字段全集，不要编造完整 schema。
- 若问题包含“最新”“当前官方”之类表述，应重新核对外部权威源。
