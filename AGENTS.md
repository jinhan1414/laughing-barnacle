# Project Agents Notes

- 应用界面仅适配手机端。新增或修改前端页面时，按移动端优先设计与实现，不要求桌面端适配。
- Docker 镜像目标架构固定为 `linux/arm64`。新增或修改 Dockerfile、构建脚本、CI 配置时，默认保持 arm64，不切换为 amd64。
- Skill 技术方案固定为“渐进式披露”：首轮仅注入 skill 索引，不直接注入完整技能说明；需要详情时，只能通过 `linux__bash` 执行 `curl -s "http://127.0.0.1:8080/api/skills/read?id=<skill_id>"` 按需读取，不新增其他内置工具。
- 当用户明确提出“记住”某项长期规则时，必须同步更新到 `AGENTS.md` 后再回复确认；后续实现默认遵循已记录规则。
