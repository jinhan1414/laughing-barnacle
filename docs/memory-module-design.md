# MemoryFS 记忆模块设计（规范 / 使用 / 维护）

## 1. 目标与边界

本设计仅定义数字分身的统一记忆模块（MemoryFS），覆盖：

1. 规范：命名空间、数据模型、接口、状态机。
2. 使用：LLM 如何读取与写入记忆，何时触发。
3. 维护：不活跃收口、定时保洁、观测与故障恢复。

默认约束：

- 单用户、全局单会话。
- 渐进式披露（先索引、后摘要、再分节）。
- 不依赖关键词/正则做核心分流。
- 记忆沉淀的软回合结束：**用户不活跃 5 分钟**。

---

## 2. 规范

## 2.1 统一命名空间

根目录固定为：

- `/meta`
- `/profile`
- `/preferences`
- `/constraints`
- `/goals`
- `/projects`
- `/routines`
- `/conversation/archive`
- `/inbox`

路径规则：

1. 必须以 `/` 开头。
2. 段名仅允许 `[a-z0-9_-]`，统一 `kebab-case`。
3. 大小写不敏感，存储层落盘统一小写。
4. 关键模式：
   - 项目：`/projects/<project_id>/...`
   - 归档：`/conversation/archive/<archive_id>/...`

## 2.2 节点类型与字段

MemoryFS 节点仅两类：

- `dir`：目录节点（索引用途）
- `file`：文件节点（内容用途）

统一元数据：

- `id`
- `path`
- `title`
- `type` (`dir|file`)
- `schema_kind`
- `schema_version`
- `tags`
- `source` (`chat|tool|manual|system`)
- `confidence` (`0.0~1.0`)
- `revision`
- `created_at`
- `updated_at`

`file` 内容体：

- `summary`（必填）
- `facts`（可选，数组）
- `sections`（可选，数组：`id/title/digest/content`）
- `refs`（可选，追溯来源）

## 2.3 状态机规范

### 2.3.1 会话分段状态

`open -> closed -> processing -> persisted`

失败分支：

`processing -> failed -> retrying -> persisted | dead_letter`

### 2.3.2 记忆项状态

`pending(inbox) -> confirmed(namespace) -> stale -> archived/trash`

## 2.4 API 规范

读接口：

- `GET /api/memory/index?path=/...`
- `GET /api/memory/read?path=/...`
- `GET /api/memory/section?path=/...&section_id=...`

写接口：

- `POST /api/memory/upsert`
- `POST /api/memory/move`
- `POST /api/memory/delete`

写入协议：

1. `upsert.mode`：`patch|replace`（默认 `patch`）。
2. 支持 `expected_revision`（乐观锁）。
3. `delete` 默认 `soft`（移动到 `/inbox/trash/...`）。

## 2.5 存储规范（BoltDB）

建议独立 DB：`APP_MEMORY_FILE`。

bucket 规划：

- `memory_nodes`（path -> node）
- `memory_children`（dir_path -> child_paths）
- `memory_segments`（segment_id -> segment）
- `memory_jobs`（job_id -> job）
- `memory_meta`（schema/version）
- `memory_audit`（审计日志）

---

## 3. 使用

## 3.1 LLM 读取记忆（渐进式披露）

每轮请求，Agent 固定注入 `MEMORY_INDEX`（目录级索引），不注入全文。

读取顺序固定：

1. `index`（看可用路径）
2. `read`（拿摘要 + section 列表）
3. `section`（按需精读 1~2 段）

约束：

1. 单轮默认只读 1 个最相关文件。
2. 不足时再补读 section。
3. 禁止一次性拉完整正文。

### 3.1.1 注入示例（L0）

```text
[MEMORY_INDEX_V1]
active_paths:
- /projects/pay-refactor/overview | rev=7 | summary=支付重构灰度中
- /projects/pay-refactor/risks | rev=4 | summary=存在密钥版本不一致风险
read_api:
- curl -s "http://127.0.0.1:8080/api/memory/read?path=<path>"
- curl -s "http://127.0.0.1:8080/api/memory/section?path=<path>&section_id=<id>"
rules:
- 先索引再读摘要再读分节
- 禁止一次性拉取全文
[/MEMORY_INDEX_V1]
```

### 3.1.2 工具读取示例（L1/L2）

```bash
curl -s "http://127.0.0.1:8080/api/memory/read?path=/projects/pay-refactor/risks"
curl -s "http://127.0.0.1:8080/api/memory/section?path=/projects/pay-refactor/risks&section_id=s1"
```

## 3.2 LLM 写入记忆（软回合触发）

写入不是“每条消息即刻落库”，而是基于 `segment` 聚合后沉淀：

1. 硬回合完成后，把 turn 工件追加到 `open segment`。
2. 若用户继续发言，继续合并并重置 idle 计时。
3. 用户不活跃达到 **5 分钟**，`open -> closed`。
4. 处理 `closed segment`：
   - LLM 输出结构化提取 JSON
   - 服务端做确定性校验
   - 高置信写正式命名空间
   - 低置信写 `/inbox`
5. 写入后记录 `audit`。

## 3.3 软回合结束规则（5 分钟）

默认配置：

- `AGENT_MEMORY_IDLE_WINDOW=5m`

关闭条件：

1. 距离最后一次用户输入 `>=5m`。
2. 或达到 `max_window=10m`（兜底强制关闭）。
3. 或达到 `max_messages=8`（防超长段）。

## 3.4 典型调用流（真实）

用户问题：

`今天支付重构要不要继续扩大灰度？先给我风险判断。`

系统行为：

1. 注入 `MEMORY_INDEX`。
2. LLM 调用 `read(/projects/pay-refactor/risks)`。
3. LLM 再调用 `section(..., s1)`。
4. 结合 `overview` 读取结果给出结论并附来源。
5. 本轮结束后先入 `open segment`，等待 5 分钟不活跃再沉淀。

---

## 4. 维护

## 4.1 维护职责边界

1. 对话主链路负责“即时应答 + segment 累积”。
2. Memory worker 负责“软回合关闭 + 结构化沉淀”。
3. 定时任务负责“保洁与一致性”，不替代主链路判断。

## 4.2 定时任务（保洁类）

建议新增任务：

1. `memory-archive-compact`：归档重分节与索引压缩。
2. `memory-inbox-digest`：汇总待确认记忆项。
3. `memory-stale-cleanup`：清理 trash/过期项。
4. `memory-consistency-check`：巡检孤儿节点、冲突路径、revision 异常。

## 4.3 审计与观测

建议指标：

- `memory_segments_open_total`
- `memory_segments_closed_total`
- `memory_persist_success_total`
- `memory_persist_failed_total`
- `memory_inbox_items_total`
- `memory_revision_conflict_total`

审计日志字段：

- `segment_id`
- `source_turn_range`
- `idle_duration`
- `target_paths`
- `result`
- `error`

## 4.4 故障恢复

1. 重启恢复：扫描超时 `open segment`，自动关闭并补处理。
2. 处理失败：进入 `failed/retrying`，超过重试阈值进 `dead_letter`。
3. 误写回滚：基于 `memory_audit` 与 `revision` 回滚到上一个版本。

## 4.5 数据保留策略

1. 正式命名空间保留历史 revision（可配置保留数）。
2. `/inbox` 保留 30 天（可配置）。
3. `/inbox/trash` 保留 7~30 天后硬删除。

---

## 5. 实施基线

## 5.1 新增配置

- `APP_MEMORY_FILE=./data/memory.db`
- `AGENT_MEMORY_IDLE_WINDOW=5m`
- `AGENT_MEMORY_MAX_SEGMENT_WINDOW=10m`
- `AGENT_MEMORY_MAX_SEGMENT_MESSAGES=8`
- `AGENT_MEMORY_WORKER_INTERVAL=30s`

## 5.2 代码落点（建议）

1. `internal/memory/*`：存储与状态机。
2. `internal/web/server.go`：`/api/memory/*`。
3. `internal/agent/agent.go`：
   - 注入 `MEMORY_INDEX`
   - 回合后 append segment
4. `internal/scheduler/*`：保洁任务。
5. `internal/web/templates/chat.html`：工具栏入口扩展（记忆相关手动触发）。

## 5.3 验收标准

1. 每轮首轮上下文只出现 Memory 索引，不出现全文。
2. 用户连续追问时，记忆不会碎片化；5 分钟不活跃后才沉淀。
3. 无关键词/正则分流依赖。
4. 低置信信息进入 `/inbox`，可追溯。
5. 定时任务仅做保洁与一致性，不抢主链路判断。

---

## 6. 结论

本方案将记忆模块统一为 MemoryFS，并把“用户不活跃 5 分钟”作为记忆沉淀的软回合关闭标准。通过“规范、使用、维护”三层设计，保证：

1. 记忆结构统一。
2. LLM 使用稳定且省 token。
3. 维护可观测、可恢复、可演进。
