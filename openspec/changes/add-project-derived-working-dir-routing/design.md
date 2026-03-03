## Context
- 当前 MemoryFS 已存在 `/projects` 命名空间，但只作为通用记忆节点参与混合索引。
- 当前 A2A 派发链路已支持 `metadata` 透传，但没有“项目 -> 工作目录 -> A2A metadata”的稳定桥接。
- `codex-a2a` 当前支持校验传入的 `metadata.working_dir`，但仍允许缺失时退回默认启动目录。

## Goals / Non-Goals
- Goals:
  - 让数字分身在本地项目开发/分析任务中稳定知道目标工作目录
  - 保持单根目录模型，所有项目目录都由统一根目录派生
  - 将 `working_dir` 变成 `codex-local` 执行前的强约束，而不是可选提示
- Non-Goals:
  - 不引入多根目录选择逻辑
  - 不把 `/projects` 全文常驻注入 system
  - 不新增基于关键词的项目匹配分流

## Decisions
- Decision: 采用“根目录配置 + `/projects` 相对路径 + 运行时派生绝对目录”的单一真相模型
  - 根目录配置文件只保存唯一 `root_dir`
  - `/projects` 节点保存 `project_id`、`relative_path`、`aliases`、`summary`
  - `working_dir = root_dir + relative_path`
- Decision: 固定注入 `Projects Index`，而非混入通用 MemoryFS 索引竞争有限条目
  - 注入内容只包含项目索引字段与展开后的 `working_dir`
  - 详细项目内容仍通过 `context__read(resource="memory", action="read|section")` 按需读取
- Decision: `codex-a2a` 对执行型请求要求 `metadata.working_dir` 必填
  - 项目型任务不再允许回退默认启动目录
  - 缺失、空字符串、路径不存在、不是目录或不可访问时显式失败

## Risks / Trade-offs
- 风险：`working_dir` 改为必填会影响已有未显式带目录的 `codex-local` 调用
  - Mitigation：同步更新数字分身的 A2A 派发逻辑与测试，先在本地链路保证总是派生并透传
- 风险：`/projects` 节点结构不统一时索引构建会失败
  - Mitigation：为项目节点定义最小结构约束，并在读取失败时显式暴露错误
- 风险：稳定注入项目索引增加上下文长度
  - Mitigation：仅注入索引行，限制字段与条数，保持固定位置和稳定前缀

## Migration Plan
1. 增加根目录配置读取与校验。
2. 增加 Projects Index 生成逻辑并固定注入。
3. 将项目型 A2A 派发改为先解析项目再透传 `metadata.working_dir`。
4. 收紧 `codex-a2a` 执行前校验，拒绝缺失 `working_dir` 的执行请求。
5. 补齐新项目目录派生与 `/projects` 回写。

## Open Questions
- 新项目 `relative_path` 的命名冲突去重规则是追加 `-2/-3`，还是后续另行约束。
