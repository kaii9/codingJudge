# GoJudge 微服务模式

本模式把现有项目拆成三个业务域：平台负责身份和题库，判题域负责 submission、快照、租约、结果和 Outbox，Executor 负责 Docker 执行。判题 API 与 Worker 共享同一个判题数据库，因为它们属于同一个业务域。

```mermaid
flowchart LR
  Browser[浏览器] --> Platform[平台 API]
  Platform --> PlatformDB[(平台数据库)]
  Platform -->|gRPC + 身份 + 快照| JudgeAPI[判题 API]
  subgraph JudgeDomain[判题业务域]
    JudgeAPI --> JudgeDB[(判题数据库)]
    JudgeDB --> Relay[Outbox Relay]
    Relay --> Stream[Redis 判题 Stream]
    Stream --> Worker[判题 Worker]
    Worker -->|租约与 fenced 结果| JudgeDB
  end
  Worker -->|gRPC RunBatch| ExecutorA[Executor A]
  Worker -->|gRPC RunBatch| ExecutorB[Executor B]
  ExecutorA --> Docker[本地 Docker daemon]
  ExecutorB --> Docker
  Relay --> ResultStream[结果 Stream]
  ResultStream --> Projector[平台投影消费者]
  Projector --> PlatformDB
```

实际测试证据见 [2026-10-14 验收记录](microservices-verification-2026-10-14.md)。

## 启动与验收

```bash
# 内存受限的 Docker Desktop 建议逐个构建，编译缓存会复用
for service in platform judge-api worker executor migrate-platform; do
  docker compose -f deploy/compose/microservices.yml build "$service"
done
docker compose -f deploy/compose/microservices.yml up -d --no-build --wait
bash scripts/verify-microservices.sh
bash scripts/verify-microservices-faults.sh
```

公开 API：`http://localhost:28080`。可选前端：

```bash
docker compose -f deploy/compose/microservices.yml --profile frontend up -d --build frontend
# 浏览器 http://localhost:23000
```

独立端口：PostgreSQL 25432、Redis 36379、MinIO 29000。端口可以用 `CJ_MICRO_*_PORT` 覆盖。使用独立 Compose project 和数据卷，不修改原单体部署的数据卷。

```bash
docker compose -f deploy/compose/microservices.yml down
```

不要用 `down -v` 保留测试数据时；该选项会删除本模式的数据卷。

## 数据所有权

| 数据 | 所有者 | 访问方式 |
|---|---|---|
| users、sessions、题目、测试清单 | 平台库 cj_platform | 平台、题目导入工具 |
| submissions、snapshots、lease、artifact metadata | 判题库 cj_judge | 判题 API、Worker |
| completion_outbox | 判题库 | 最终结果同事务写入，Relay 发布 |
| submission_projection | 平台库 | 完成事件消费、按 event ID 去重 |
| Docker 沙箱 | Executor | RPC 请求，不访问业务库 |

两个 PostgreSQL 账号不允许 CONNECT 对方数据库。跨域 user/problem ID 不使用外键；判题域内的 submission/outbox/snapshot 外键保留。平台不持有 Docker Socket，Worker 不持有平台数据库凭据。

创建提交：平台认证用户，读取内部题目快照，经 gRPC 传给判题服务；判题库在一个事务中写 submission、snapshot 和 judge_outbox。重放保持第一次快照。Worker 始终读取该 submission 的快照。

完成提交：fencing 条件更新结果和写 completion_outbox 在同一事务。Redis Streams 投递可能重复，平台把 processed_event 和 projection 更新放进同一事务。排行榜按 distinct problem 计算 solved，多次 AC 增加 accepted submissions 但不增加 solved。

## 内部协议

`api/proto/gojudge/judge/v1/judge.proto`：创建、查找、查询、历史与产物。

`api/proto/gojudge/executor/v1/executor.proto`：批量执行。

公开 HTTP API 保留。原部署默认使用内部 HTTP，新模式通过 `EXECUTOR_PROTOCOL=grpc` 明确切换。执行客户端禁用 gRPC 自动重试，Worker 负责有界任务 attempt；不使用失败后隐式 HTTP 回退。

协议增量采用新可选字段；不复用旧字段编号，不改变既有字段含义。冻结 fixture `api/compat/executor/executor-v1.proto` 验证旧客户端调用当前服务。它证明该 fixture 的兼容性，不代表所有未来版本自动兼容。

重新生成：

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.10
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
make proto
```

## 超时、取消和重试

- 平台调用判题服务使用 5 秒 deadline。
- Worker 到 Executor 使用配置的 EXECUTOR_TIMEOUT，默认 120 秒。
- Worker 单次处理总预算默认 3 分钟，包括资产与落盘；租约心跳延伸到产物、结果持久化。
- 执行预算耗尽后，使用仍受 Worker 停机取消控制的独立 5 秒收尾预算进行 fenced 重试/终止；默认最多执行 3 次。崩溃后接管时若计数已超限，不再执行用户代码。
- Executor 限制执行槽位与等待请求数，等待超时返回过载。
- 标准 gRPC Health/Watch 配合 round_robin，避开 NOT_SERVING 实例并允许恢复。
- 参数、鉴权、协议错误不重复判题；业务 WA/CE/RE/TLE 是正常响应。
- 调用发出后响应丢失属于结果未知；任务层可重新执行，最终写入仍由 fencing 决定。
- 不宣称消息或物理执行 Exactly Once。

用户程序运行预算从可信包装器开始/结束帧测量，Docker 启动和退出开销不计入。包装器以 root 控制，用户程序使用 nobody、清空环境；只有包装器拥有 SETUID/SETGID，用户程序不能读取 root 环境中的控制帧密钥。超时仍从宿主 Executor 强制清理容器。网络、CPU、内存、PID、只读挂载和 no-new-privileges 保留。

## 退出和孤儿容器

Executor 标记 NOT_SERVING，停止接新请求，等待在途任务；宽限期后取消并给容器清理留出时间。主进程等待退出流程，而不是启动 Shutdown goroutine 后立即退出。

服务端 drain 取消且调用方仍存活时返回可重试的不可用错误，不把它当成调用方主动取消。故障脚本先实际 SIGKILL，再以 100ms 宽限期实际 SIGTERM，在健康实例恢复到 AC；退出时还原默认 30 秒宽限期。可用 CJ_MICRO_EXECUTOR_SHUTDOWN_GRACE 控制该演示参数。

容器带 `gojudge.executor` 标签。重启 Executor 只清理自身实例标签且名称为 codingjudge-exec-* 的沙箱。给每个实例配置唯一、稳定的 `EXECUTOR_INSTANCE_ID`；不能让不同活跃实例共享同一个 ID。

本地默认两个 Executor 共享宿主 Docker daemon。这证明进程故障隔离和独立部署，不代表两个物理节点或线性算力扩容。独立 daemon 的实验仍见原有 multinode 拓扑和 benchmark 报告。

## TLS 与服务身份

本地 Compose 以未暴露到宿主的 gRPC 端口加 service Token 演示。远程使用以下参数启用双向 TLS：

```text
RPC_TLS_CERT_FILE=/run/secrets/service-cert.pem
RPC_TLS_KEY_FILE=/run/secrets/service-key.pem
RPC_TLS_CA_FILE=/run/secrets/ca.pem
RPC_TLS_SERVER_NAME=可选的目标服务证书名称（客户端）
```

三项证书配置必须一起提供，部分配置会启动失败，不回落明文。仍需按服务授权 Token；TLS 身份和 user_id 是不同概念。外部 HTTP 入口继续使用现有 Session。

## 追踪与观测

Go 服务使用 OpenTelemetry。HTTP/RPC 创建 span；traceparent 在 judge_outbox 中保存，Relay 写入 Stream，Worker 建立 attempt span，再传播到 Executor。日志记录 traceparent；不记录源码、隐藏测试或凭据。

设置 `OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:4318` 可导出到兼容 OTLP/HTTP 的 Collector/Jaeger。未配置时仍生成并传递 trace context，但不导出 span。不要把这一状态描述为已经部署追踪后端。

## 测试

```bash
go test ./...
go test -race ./internal/executor ./internal/judgerpc ./internal/platform ./internal/store ./internal/judgeworker ./internal/telemetry ./internal/rpcsecurity
go vet ./...

TEST_DOCKER_RUNNER=1 go test -tags integration ./internal/judge

TEST_JUDGE_DATABASE_URL='postgres://cj_judge:cj-judge-local-only@127.0.0.1:25432/cj_judge?sslmode=disable' \
TEST_PLATFORM_DATABASE_URL='postgres://cj_platform:cj-platform-local-only@127.0.0.1:25432/cj_platform?sslmode=disable' \
go test -tags integration ./internal/store -run TestMicroservice -count=1

PLAYWRIGHT_BASE_URL=http://localhost:23000 npm --prefix frontend run test:e2e
```

专用数据库集成测试在随机 schema 中验证，测试后只清理自己的 schema。不要把已有 legacy 集成测试的 TEST_DATABASE_URL 指向重要数据。

验收脚本检查真实三语言判题、排行榜收敛、单 Executor 停机、独立重新部署、历史/产物下载及两条 Stream 的 Pending 归零。故障脚本额外检查活跃沙箱期间 SIGKILL 与 SIGTERM 宽限期耗尽后的 attempt 恢复。deadline、取消、过载、响应丢失和旧协议 fixture 由真实 loopback RPC 测试补充。

## 已有数据如何迁移

默认创建新数据库，原部署可继续运行。已有数据迁移需要显式导入：先迁平台用户/题目，判题域迁提交并补与该提交对应的快照，然后重建排行榜 projection。不要直接删除跨域外键后继续让两个服务共享旧库，也不要自动从新模式回写旧库。本次提供新模式和回滚到原模式的启动路径，不自动搬迁用户既有数据。
