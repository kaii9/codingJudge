# GoJudge PRD

## 1. Product Summary

GoJudge 是一个面向简历和 GitHub 展示的在线代码评测系统。它的产品形态是题库、代码提交、异步判题、结果查询和后续排行榜，但核心技术价值是安全地运行不可信代码。

一句话定位：

```text
Web 后端只是外壳，真正的难点是安全地运行不可信代码。
```

## 2. Goals

- 支持用户浏览题目、提交代码、查看判题状态和结果。
- API 服务和判题执行隔离，API 不直接运行用户代码。
- 使用 Docker sandbox 执行不可信代码，并施加资源和安全限制。
- MVP 提供 Go/C++/Python 的语言规格和 Docker runner 命令，后续继续强化多语言编译错误、运行错误和镜像管理。
- 通过清晰架构、测试、Docker Compose、OpenAPI 和 README 做成适合 GitHub 展示的后端项目。

## 3. Non-Goals

- MVP 不做复杂比赛系统、完整权限后台和多租户隔离。
- MVP 不实现大规模分布式调度，只预留 Redis Streams consumer group 边界。
- MVP 不把大测试用例塞进数据库；后续使用 MinIO 或文件系统保存实际测试文件。
- MVP 不追求前端复杂视觉效果，前端后续只承担完整流程展示。

## 4. Users

- 普通用户：浏览题目，提交代码，查看提交状态和判题结果。
- 管理员：后续维护题目、测试用例、语言配置和比赛。
- 项目评审者：通过 README、架构图、测试和 Compose 快速理解系统设计。

## 5. Core User Stories

1. 作为用户，我可以查看题目列表，了解题目标题、描述、语言和限制。
2. 作为用户，我可以打开题目详情，复制样例或阅读说明后编写代码。
3. 作为用户，我可以提交 Go、C++ 或 Python 代码，并立即得到一个 `queued` 状态的 submission。
4. 作为用户，我可以查看提交记录，确认最近提交的运行状态。
5. 作为用户，我可以打开单个提交，查看 `accepted`、`wrong_answer`、`runtime_error` 或 `time_limit_exceeded`。
6. 作为系统维护者，我可以确认 API 服务没有直接执行用户代码，判题只发生在 worker 的 Docker sandbox 中。

## 6. Functional Requirements

### MVP

- `GET /healthz` 返回健康状态。
- `GET /problems` 返回题目列表，不暴露隐藏测试用例。
- `GET /problems/{id}` 返回题目详情，不暴露隐藏测试用例。
- `POST /submissions` 创建提交，状态为 `queued`，并写入判题队列。
- `GET /submissions` 返回提交记录列表，默认按更新时间倒序。
- `GET /submissions/{id}` 返回提交状态和结果，不返回用户代码。
- API 启动 dispatcher，把队列任务发送给独立 worker。
- worker 提供 `POST /judge` 内部接口，运行 Docker sandbox 并返回标准结果。
- Redis Streams 任务只在结果成功持久化后确认；失败任务最多重试三次，然后进入死信流。

### Future

- 用户登录与提交归属。
- PostgreSQL 持久化题库、测试用例 metadata、提交和结果。
- Redis Streams 判题队列，支持 consumer group、ack、retry 和 dead-letter stream。
- MinIO 保存大测试输入输出文件。
- Next.js + Monaco Editor 前端。
- 排行榜、比赛、管理后台。

## 7. Non-Functional Requirements

- 安全：worker 必须禁用网络、限制 CPU/内存/进程数、只读文件系统、丢弃 Linux capabilities。
- 资源：编译和运行阶段分离，单个 stdout/stderr 捕获上限为 1 MiB。
- 隔离：API 服务不得直接执行用户代码。
- 可测试：核心 store、queue、dispatcher、judge、HTTP handler 都需要 Go testing 覆盖。
- 可演进：内存 store 和 queue 必须通过接口抽象，方便替换 PostgreSQL/sqlc 和 Redis Streams。
- 可部署：Docker Compose 一键启动 API 与 worker。
- 可观察：使用 `slog`，后续增加 Prometheus metrics。

## 8. Acceptance Criteria

- 本地 `make test` 通过。
- `docker compose config` 可以解析 Compose 配置。
- API 和 worker 可以分别构建。
- README 包含启动方式、接口示例、架构图、沙箱说明和简历亮点。
- OpenAPI 草案覆盖 MVP API。
- 代码提交后，API 只排队和派发，不在 API 进程运行用户代码。

## 9. Metrics

- 判题成功率。
- 判题平均耗时。
- queued/running submission 数量。
- worker 失败率和 timeout 数量。
- Redis Streams pending message 数量，未来阶段启用。

## 10. Risks

- Docker socket 挂载会让 worker 具有较高权限；MVP 中明确 worker 为高风险组件，后续可评估 gVisor、Firecracker 或 Kubernetes Job。
- 内存 queue 在进程重启后会丢任务；后续用 Redis Streams 解决。
- 内存 store 无持久化；后续用 PostgreSQL/sqlc 解决。
- 支持多语言后，编译命令、运行时限制和依赖镜像会变复杂，需要语言配置表和 per-language runner。
