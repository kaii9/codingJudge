# GoJudge 开发计划

## 目标

构建一个 Go 后端在线代码评测系统，支持题库、代码提交、异步判题、提交记录和后续排行榜。项目核心不是 CRUD，而是安全地运行不可信代码：API 服务只是外壳，Docker sandbox 和独立 judge worker 是主线。

第一阶段先完成 MVP：题目列表、提交代码、异步判题、查看结果，并用 Docker 沙箱执行用户代码。

## 最终技术选型

```text
Backend: Go + chi
DB: PostgreSQL + pgx
Migration: goose
Queue: Redis Streams
Sandbox: Docker
Worker: Go judge-worker
Storage: MinIO
Frontend: Next.js + React + Monaco Editor
Deploy: Docker Compose
CI/CD: GitHub Actions
Docs: OpenAPI
Observability: slog + Prometheus metrics
```

MVP 的单元测试可以继续使用内存 store 和内存 queue。Compose 环境使用 PostgreSQL 和 Redis Streams，代码边界保持可替换。

## MVP 功能

1. 健康检查接口
2. 题目列表接口
3. 题目详情接口
4. 创建代码提交接口
5. 查询提交结果接口
6. 内存任务队列
7. Go judge worker
8. Docker 沙箱执行
9. README、Docker Compose、基础测试

## MVP 非目标

- 不一开始实现完整比赛系统、排行榜、管理后台。
- 不一开始支持过多语言，先支持 Go，后续扩展 C++ 和 Python。
- 不在 API 服务中直接执行用户代码。
- 不把大测试用例全部塞进数据库；后续数据库存 metadata，MinIO 或文件系统存实际输入输出文件。

## 推荐目录结构

```text
cmd/api/main.go
cmd/worker/main.go
internal/domain/
internal/httpapi/
internal/store/
internal/queue/
internal/dispatcher/
internal/judge/
internal/workerapi/
docs/
migrations/
README.md
Dockerfile
docker-compose.yml
Makefile
```

## MVP 技术栈

- Go + chi
- 内存 store + PostgreSQL store
- 内存 queue + Redis Streams queue
- Docker sandbox
- Docker Compose
- GitHub Actions
- OpenAPI 草案

## 判题流程

```text
用户提交代码
API 创建 submission
submission 状态设为 queued
任务进入 judge queue
worker 拉取任务
worker 启动 Docker 容器执行代码
比较 stdout 和 expected output
保存 Accepted / Wrong Answer / Runtime Error / Time Limit Exceeded
用户查询结果
```

## Docker 沙箱要求

执行用户代码时必须考虑：

- 禁用网络
- 限制 CPU
- 限制内存
- 限制运行时间
- 限制进程数
- 只读文件系统
- 丢弃 Linux capabilities
- 使用临时目录
- worker 和 API 服务隔离

## 数据库演进目标

PostgreSQL 负责管理：

```text
users
problems
test_cases
submissions
submission_results
languages
contests
leaderboards
```

数据访问层当前使用 `pgx`，后续如果查询复杂度继续上升，可以引入 `sqlc` 生成类型安全查询代码。`GORM` 只作为备选，不作为主路线。

## 队列演进目标

Redis Streams 负责判题任务：

```text
submission.created -> judge stream -> worker consumer group -> result saved
```

后续要支持：

- 多 worker consumer group
- pending message recovery
- ack/retry
- dead-letter stream
- worker 横向扩展

## 文件和测试用例存储

MVP 使用内置样例题。后续演进：

- PostgreSQL 存题目和测试用例 metadata
- MinIO 或本地文件系统存大输入输出文件
- worker 运行前下载测试文件到临时目录

## 前端目标

前端不是第一阶段重点，但最终应支持：

- 题目列表
- 题目详情
- Monaco Editor 代码编辑器
- 提交记录
- 判题状态
- 运行结果
- 排行榜
- 管理后台

## 第一阶段交付

- Go API 可运行
- 测试通过
- README 完整
- Docker Compose 可启动
- GitHub 项目结构专业
