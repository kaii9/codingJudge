# Project Instructions

这个项目是一个使用 Go 开发的在线代码评测系统，目标是做成可以发布到 GitHub、可以写进简历的后端主导项目。

## 技术方向

- 后端语言：Go
- API：net/http + chi 风格，优先标准库
- 数据库：MVP 可先用内存存储，后续迁移 PostgreSQL
- 队列：MVP 可先用内存队列，后续迁移 Redis Streams
- 判题沙箱：Docker
- 部署：Docker Compose
- 测试：Go testing
- 文档：README、架构说明、API 示例

## 开发要求

- 优先实现 MVP 主链路，不要一开始做过多功能
- 使用测试驱动关键逻辑
- 代码结构清晰，适合 GitHub 展示
- README 要包含启动方式、接口示例、架构图、简历亮点
- 判题执行必须和 API 服务隔离
- 不要在 API 服务中直接运行用户代码