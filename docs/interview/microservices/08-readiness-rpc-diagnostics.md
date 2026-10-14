# 8. 就绪检查与安全 RPC 排障

源码入口：[API 启动装配](../../../cmd/api/main.go)、[Judge 客户端](../../../internal/judgerpc/client.go)、[HTTP 探针](../../../internal/httpapi/server.go)。本次不新增服务、重试或更宽的超时。

## 8.1 先区分证据和推测

main `aac45b8` 在 [CI run 38617959365](https://github.com/kaii9/codingJudge/actions/runs/38617959365) 中，frontend/test/e2e/observability 成功，microservices 第一条提交返回 HTTP 500，耗时约 5010ms。没有底层 RPC 状态码；不能凭“约 5 秒”断言是 DNS、连接或数据库故障。

代码核对能够确认：`grpc.NewClient` 创建连接对象，不保证已经连通或鉴权成功；原 Platform `/readyz` 检查本地 store/Redis，却没有通过实际客户端检查 Judge RPC。Judge 自己的 HTTP 探针成功，也不等于 Platform→Judge 连接和 token 可用。

本次修复已证实的就绪检查缺口，并增加下一次失败的定位证据；不宣称已经查明上述 500 的根因。本地成功也不能抹掉远程失败，或替代新提交的远程 CI。

## 8.2 三种“健康”的职责

| 探针 | 回答的问题 | 不能证明 |
| --- | --- | --- |
| Platform `/healthz` | HTTP 进程活着吗？ | 下游可达、提交一定成功 |
| Platform `/readyz` | 已注册的关键依赖目前可用吗？ | 下一时刻仍可用、全部业务成功 |
| Judge gRPC `Health/Check` | RPC 传输、token 和 serving 信号可用吗？ | DB 写事务、对象存储、Worker 和沙箱都成功 |

微服务模式的 `remoteJudgeOptions` 同时注册远程产物读取和 `judge_rpc` readiness。旧单体模式客户端为 nil，不添加这一依赖。

```text
GET /readyz
  ├─ 同一个 context：总预算 2s
  ├─ Platform store Ping
  ├─ Judge Client.Ping
  │    └─ 复用业务连接 + 同一 Bearer token → Health/Check
  │         ├─ RPC 错误 / 非 SERVING → 503，component=judge_rpc
  │         └─ SERVING → 继续
  └─ Redis Ping → 全部成功才 200
```

`/healthz` 保持 200，不因为 Judge 宕机把正常 Platform 进程当作死亡。Docker Compose 的 unhealthy 本身并不会自动实现摘流量或重启；部署层需要另行配置。默认 Judge gRPC health 是静态 SERVING 信号，不是其所有依赖的动态汇总；Compose 仍另有 Judge HTTP readiness。

## 8.3 Deadline 为什么必须沿用父 Context

Judge 客户端用 `context.WithTimeout(parent, 5s)`，实际截止时间取父 deadline 和本地上限中较早者。`/readyz` 的 2s 是所有顺序检查共享的预算，不是每个依赖各给 2s。

例如 store 花了 600ms，Judge 最多只能使用约 1.4s。不能改用 Background 再给 5s，也不能因增加依赖而放宽总预算。取消继续传播到 gRPC handler。本次没有改变 Judge 业务 5s、Worker→Executor 120s 或编译/运行限制。

客户端维持 `WithDisableRetry`，诊断拦截器只调用下一个 invoker 一次，不重试、不包装错误。它禁用配置型 retry，不等于 gRPC 在确认应用未处理时绝对没有透明重试，详见第3章。

## 8.4 为什么不直接打印 error

错误描述可能含数据库信息、源码或凭证。新增诊断日志采用白名单：

| 字段 | 用途 |
| --- | --- |
| `grpc_method` | 区分幂等查找、创建、查询、下载或 Health |
| `grpc_code` | 区分 Unavailable、DeadlineExceeded、Unauthenticated、Internal 等 |
| `connection_state` | 调用结束时连接状态，不是根因判定器 |
| `duration_ms` | 对齐 RPC 预算和 HTTP 耗时 |
| `trace_id` | 有效 span context 存在时记录，关联 HTTP traceparent 的 trace ID |

没有 request/response、token、metadata、user ID、源码、密码、endpoint 或原始错误描述；标准 time/level/msg 保留。telemetry 拦截器包住诊断拦截器，后者能看到 RPC span context。

NotFound、AlreadyExists、InvalidArgument 和 Canceled 不写 WARN；DeadlineExceeded、Unavailable、Unauthenticated、Internal 等写 WARN。错误原样返回，已有 HTTP 映射、幂等语义不变。非 SERVING 的 Health 响应是成功 RPC 后的 readiness 判断，不被 RPC 错误日志捕获，但探针仍返回 503。

这是新增日志的脱敏保证，不是全系统日志与 trace 安全审计。既有 tracing 的错误事件记录仍需按威胁模型单独评估，不能外推日志白名单的测试结论。

## 8.5 下一次失败怎么查

1. 取失败 HTTP traceparent 中的 32 位 trace ID。
2. 查 Platform 日志同一 `trace_id`，确定失败的是 FindSubmission 还是 CreateSubmission。
3. 根据 code 缩小范围，再核对连接状态、下游日志和 trace。READY + DeadlineExceeded 不能直接等同数据库慢，连接状态只是结束时快照。
4. Unauthenticated 核对身份；Unavailable 核对服务、网络与传输；DeadlineExceeded 查预算消耗；Internal 查下游业务/存储边界。
5. 创建失败或响应丢失时，用原 Idempotency-Key 核对，不换新 key 盲目重提，不把所有失败自动重试。

```bash
docker compose -f deploy/compose/microservices.yml \
  -f deploy/compose/microservices-tracing.yml logs --no-color platform
curl -i --max-time 4 http://127.0.0.1:28080/readyz
curl -i --max-time 4 http://127.0.0.1:28080/healthz
```

分类提供调查方向，不代替服务器证据。不要为了绿灯延长 timeout、关闭鉴权或降低审计门槛。

## 8.6 测试为什么可信

新增测试用 loopback 随机 TCP 端口和真实 gRPC，不是 grep 源码：

- [Health 测试](../../../internal/judgerpc/client_health_test.go)：生产 token 拦截器下的成功/拒绝；无业务 backend 仍能 Health，证明没有创建提交副作用；三个非 SERVING 状态；父 deadline、客户端 cap、handler 取消；不可达实例。
- [API 装配测试](../../../cmd/api/main_test.go)：使用生产装配函数发 `/readyz`；健康 200，错误 token/停服 503；`/healthz` 独立；旧模式兼容；先消耗 store 预算，Judge 只能得到剩余时间。
- [日志测试](../../../internal/judgerpc/client_diagnostics_test.go)：六种业务 RPC 返回含敏感标记的错误，验证 JSON 白名单、分类、trace、零泄露和原错误不变；另测 Health 鉴权、deadline 和期望业务结果；超时只进入一次应用 handler。

短预算测试先等真实连接 READY，把 deadline 传播与测试机首次连接速度分开，不用固定 sleep 假装已连接。不可达用例另测冷连接失败。

红绿记录：Health 首次失败是缺少探针；API 首次失败是错误身份/停服/慢服务时仍返回 200；日志首次失败是没有诊断。修复 fixture 类型错误不算业务 RED。之后运行全量 suite 与 race，不以单文件绿灯代替验收。

```bash
go test ./... -count=1
go test -race ./cmd/api ./internal/judgerpc ./internal/rpcsecurity ./internal/httpapi -count=1
go test -race ./cmd/api ./internal/judgerpc -count=5
go vet ./...
CJ_MICRO_TRACING=1 make verify-microservices
CJ_MICRO_TRACING=1 make verify-microservices-tracing
```

microservices CI 新增显式 race 步骤，其他验收不删减。接入步骤不等于远程已通过，见 [验收记录](verification-2026-10-15.md)。长期复用数据库时新用户可能排在排行榜前50名之外，脚本不能据此断言投影丢失；使用独立空数据验证栈，不清空已有业务数据。

## 8.7 面试怎样讲

“服务自身 healthy 不代表调用方能调用它。Platform 原 readiness 没覆盖 Judge RPC，我补了带同一 token、复用同一连接的 Health，并共享原有2秒预算。真实连接测试覆盖身份错误、停服、慢响应和取消。RPC 失败日志只保留分类、连接状态和 trace ID，不记录源码或凭证。某次 CI 500 缺少底层证据，我没有凭5秒耗时武断认定原因，也没有用重试或延长 timeout 掩盖问题。”

它体现依赖建模、调用治理、安全可观测性与证据驱动排障；不等于生产服务发现、自动熔断、跨主机 HA 或所有故障零失败。
