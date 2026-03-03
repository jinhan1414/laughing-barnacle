## 1. Spec
- [x] 1.1 新增 `project-context-routing` spec，定义单根目录配置、`/projects` 相对路径模型与稳定注入的 Projects Index
- [x] 1.2 修改 `a2a-native-capability`，要求项目型 A2A 委托必须派生并透传 `metadata.working_dir`
- [x] 1.3 修改 `codex-a2a-reference-service`，要求执行型请求缺失或非法 `working_dir` 时显式失败

## 2. Implementation
- [x] 2.1 增加单根目录配置加载与校验
- [x] 2.2 为 `/projects` 节点补充相对路径派生与索引构建逻辑
- [x] 2.3 在系统上下文中固定注入 `Projects Index`
- [x] 2.4 在项目开发/分析任务提交 A2A 前解析项目并派生 `working_dir`
- [x] 2.5 新项目目录创建成功后写回 `/projects`
- [x] 2.6 收紧 `codex-a2a`：执行前强校验 `metadata.working_dir`

## 3. Validation
- [x] 3.1 单测：Projects Index 固定注入且只暴露索引字段
- [x] 3.2 单测：已有项目可从 `/projects` 派生 `working_dir`
- [x] 3.3 单测：新项目可由根目录 + 相对路径生成目录并回写 `/projects`
- [x] 3.4 单测：项目型 A2A 委托缺失 `working_dir` 时显式失败
- [x] 3.5 单测：`codex-a2a` 对非法 `metadata.working_dir` 显式失败
- [x] 3.6 运行 `openspec validate add-project-derived-working-dir-routing --strict`
