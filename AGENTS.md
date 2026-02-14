# Project Agents Notes

- 应用界面仅适配手机端。新增或修改前端页面时，按移动端优先设计与实现，不要求桌面端适配。
- Docker 镜像目标架构固定为 `linux/arm64`。新增或修改 Dockerfile、构建脚本、CI 配置时，默认保持 arm64，不切换为 amd64。
