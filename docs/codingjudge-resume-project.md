# GoJudge 简历项目说明

> 用途：从当前真实实现中提炼简历表述、项目介绍和可自证数据。
>
> 原则：只写仓库已经实现且能够通过源码、测试、监控或基准报告证明的内容，不把固定负载延迟测试包装成最大吞吐测试。

**推荐用法：** 中级 Go 后端岗位直接使用第 5 节的五条版本；面试前先熟练第 6–8 节的 1/3/5 分钟口述，再用第 9–14 节校验数据和事实边界。

## 1. 项目定位

**项目名称：** GoJudge 在线代码评测系统

**一句话介绍：**

> 使用 Go 构建的后端主导在线代码评测系统，支持用户登录、个人提交、全站排行榜和 Go/C++/Python 异步判题；通过 PostgreSQL Transactional Outbox、Redis Streams Consumer Group、数据库租约与 fencing token 实现可靠多 Worker 判题，并使用 Docker 隔离运行不可信代码。

**核心技术栈：**

```text
Go / net/http / chi
PostgreSQL / pgx
Redis Streams
Docker Sandbox
Authenticated Executor Service / Client Pool
Prometheus / Grafana
k6 / Docker Compose / GitHub Actions
Next.js / React / Monaco Editor
```

**项目性质：**

- 个人后端项目；
- 完整 MVP，可以通过浏览器提交代码并查看结果；
- 支持注册登录、个人提交历史和全站 AC 排行榜；
- 后端可靠性和沙箱执行是重点；
- 前端用于展示业务闭环，不是主要技术卖点。

## 2. 简历精简版

适合简历空间有限、项目只保留 3 条描述的情况。

### 版本 A：偏可靠性

```text
GoJudge 在线代码评测系统
技术栈：Go、PostgreSQL、Redis Streams、Docker、Prometheus、Grafana

- 设计 API、Outbox Relay 与 Judge Worker 分离的异步判题架构，支持 Go/C++/Python 代码提交、状态轮询和判题结果持久化。
- 基于 PostgreSQL Transactional Outbox、Redis Consumer Group、延迟 XACK、重试与死信流实现至少一次任务投递，并通过租约与 fencing token 拒绝旧 Worker 的迟到结果。
- 使用 HttpOnly Cookie + 服务端 Session 实现可撤销登录态，提交记录绑定用户并按 Accepted 去重题目数聚合排行榜；使用 Docker 限制用户代码资源并接入 Prometheus/Grafana/k6 验证。
```

### 版本 B：偏 Go 后端

```text
- 使用 Go + chi 构建在线代码评测 API 和独立 Judge Worker，基于小接口拆分 HTTP、Store、Queue、Judge 与 Metrics，支持内存实现和 PostgreSQL/Redis 集成实现。
- 使用 PostgreSQL 事务同时写入 submission 与 outbox，解决数据库和 Redis 双写丢失窗口；Redis Streams 提供 Consumer Group、Pending 接管、重试和死信。
- 通过数据库行锁、30 秒租约、10 秒心跳和 fencing token 管理多 Worker 所有权，保证重复消息和故障接管场景下最终结果不被旧 Worker 覆盖。
```

## 3. 简历标准版

适合中级 Go 后端岗位，建议使用 4 至 5 条。

```text
GoJudge 在线代码评测系统
Go / chi / PostgreSQL / Redis Streams / Docker / Prometheus / Grafana / k6

- 使用 Go 构建题库、代码提交、提交历史和异步判题服务，将 HTTP API、Judge Worker 与持有 Docker Socket 的 Sandbox Executor 分离部署，避免 Web 服务直接运行不可信代码。
- 使用 bcrypt、HttpOnly Cookie 和服务端 Session 实现注册登录，提交接口强制认证，提交历史按用户隔离，排行榜按 Accepted 去重题目数聚合。
- 基于 PostgreSQL Transactional Outbox 消除数据库提交与 Redis 发布之间的双写丢失窗口；Relay 使用 SKIP LOCKED、抢占过期时间和指数退避支持并发发布与故障恢复。
- 基于 Redis Streams Consumer Group 实现多 Worker 消费，通过延迟 XACK、XAUTOCLAIM、基础设施重试和 Dead Letter Stream 提供至少一次投递和 Pending 恢复。
- 使用 PostgreSQL 行锁、租约、心跳和 fencing token 管理判题所有权；最终结果采用带 token、状态和租约期限的条件更新，拒绝旧 Worker 恢复后的迟到写入。
- Worker 通过 Bearer Token 保护的内部 HTTP 协议访问并发受限的 Executor Client Pool；Executor 使用 Docker 限制网络、CPU、内存、PID、文件系统权限和输出大小，并暴露独立健康检查与指标。
```

## 4. 简历详细版

适合项目经历较少、希望把该项目作为主项目展开的情况。

```text
GoJudge 在线代码评测系统（个人项目）

- 使用 Go + chi 构建在线代码评测 API，支持注册登录、20 道面试题与 2 道 Starter 题、Go/C++/Python 提交、异步状态轮询、个人提交历史和全站排行榜；使用 Next.js + Monaco 构建完整演示工作台。
- 使用 bcrypt 存储密码哈希，采用 HttpOnly Cookie + 服务端 Session 管理登录态，数据库只保存 session token 的 SHA-256 哈希；提交创建、提交列表和提交详情均按当前用户隔离。
- 将 API、Outbox Relay、Judge Worker 和 Sandbox Executor 按职责与风险拆分：Worker 管理队列、租约和结果，Executor 独占 Docker Socket 并以有界并发执行不可信代码。
- 在单个 PostgreSQL 事务中创建 submission 和 judge_outbox，通过 Transactional Outbox 解决数据库与 Redis 双写一致性；使用 SKIP LOCKED 支持多 Relay 抢占，并对失败发布执行指数退避。
- 使用 Redis Streams Consumer Group 分发任务，结果写库成功后才执行 XACK；通过 XAUTOCLAIM 恢复崩溃 Worker 的 Pending 消息，基础设施错误最多重试 3 次后进入死信流。
- 使用 PostgreSQL SELECT FOR UPDATE、30 秒租约、10 秒心跳和随机 fencing token 管理 Worker 所有权；结果写入携带 token 和 lease 条件，防止重复消息或旧 Worker 覆盖新结果。
- Docker 运行阶段启用 network none、CPU/内存/PID 限制、只读根文件系统、cap-drop ALL、no-new-privileges 和输出截断；Go/C++ 采用独立编译阶段并复用产物执行多个测试用例。
- 接入 16 类业务指标，Prometheus 通过 Docker DNS 自动发现扩容 Worker，Grafana 预配置 API、Queue、Worker 和 Judge 面板；k6 固定 1 req/s 条件下，1/2/4 Worker 的 Judge P95 分别为 9.49s、3.13s、0.84s，三轮均为 121 次创建和接受，HTTP/逻辑失败及 dropped iterations 均为 0。
- 实现单/双 Executor 及独立 Docker daemon 的可复现拓扑实验；540/540 个测量任务全部 AC，同机中位吞吐从 4.267/s 提升到双 Executor 共享 daemon 的 6.390/s，同时明确该结果不能外推为生产容量。
- 使用 Go testing、race detector、PostgreSQL/Redis integration tests、Worker 故障接管脚本和 GitHub Actions覆盖核心状态机、租约接管、沙箱参数、指标和外部组件交互。
```

## 5. 推荐最终版本

如果投递中级 Go 后端，建议使用下面这版。信息密度够高，同时不会让项目描述过长。

```text
GoJudge 在线代码评测系统
Go / chi / PostgreSQL / Redis Streams / Docker / Prometheus / Grafana

- 使用 Go 构建在线代码评测 API、Judge Worker 与独立 Sandbox Executor，支持用户登录、Go/C++/Python 异步判题、个人提交历史和排行榜，避免 API/Worker 持有 Docker Socket。
- 使用 bcrypt、HttpOnly Cookie 和服务端 Session 实现可撤销登录态，提交数据绑定用户并按 Accepted 去重题目数聚合全站排行榜。
- 基于 PostgreSQL Transactional Outbox 与 Redis Streams Consumer Group 实现可靠任务链路，通过延迟 XACK、XAUTOCLAIM、失败重试和死信流提供至少一次投递及故障恢复。
- 使用数据库行锁、租约、心跳和 fencing token 管理多 Worker 判题所有权，通过条件更新拒绝重复消息和旧 Worker 的迟到结果。
- Worker 通过内部 HTTP 调用有界并发的 Executor，使用 Docker 限制用户代码网络、CPU、内存、PID、文件系统权限及输出大小；以 Prometheus/Grafana 和可复现 k6 基准验证扩容效果。
```

## 6. 一分钟项目介绍

> GoJudge 是我使用 Go 开发的在线代码评测系统，支持用户登录、题库、代码提交、异步判题、个人提交历史和全站排行榜。项目的核心难点不是题目 CRUD，而是安全、可靠地执行不可信代码。
>
> 登录态使用 HttpOnly Cookie 和服务端 Session，密码用 bcrypt 哈希，Session token 在数据库中只保存 SHA-256 哈希。提交接口必须登录，submission 会绑定 user_id，提交历史只能看自己的记录，排行榜则从 Accepted 提交中按去重题目数聚合。
>
> 用户提交代码后，API 会在同一个 PostgreSQL 事务中写入 submission 和 outbox，再由后台 Relay 发布到 Redis Streams。多个 Judge Worker 通过 Consumer Group 消费任务，但 Redis 只负责消息分发，真正的结果写权限由 PostgreSQL 租约和 fencing token 控制。Worker 必须先获得租约，最终更新也必须携带当前 token，所以旧 Worker 即使在租约过期后恢复，也无法覆盖新 Worker 的结果。
>
> Worker 不持有 Docker Socket，而是通过 Bearer Token 保护的内部 HTTP 调用有界并发的 Sandbox Executor。Executor 关闭容器网络并限制 CPU、内存、PID、文件系统权限和输出大小。系统还接入了 Prometheus、Grafana 和 k6，用可复现的固定负载与拓扑实验验证扩容效果。

## 7. 三分钟项目介绍

### 7.1 业务背景

> 系统提供注册登录、题库、代码编辑、提交、异步判题、个人提交历史和全站排行榜，支持 Go、C++ 和 Python。前端使用 Monaco 展示完整闭环，但项目重点是后端可靠性、代码执行安全和用户数据边界。

### 7.2 主链路

> API 收到提交后不会直接发 Redis，而是在 PostgreSQL 事务中同时写 submission 和 outbox。这样即使 Redis 当时不可用，提交也不会丢。Relay 使用 SKIP LOCKED 抢占未发布事件，发布失败会指数退避。

### 7.3 消费可靠性

> Worker 使用 Redis Streams Consumer Group。消息读取后进入 Pending，只有结果写入 PostgreSQL 成功才 XACK。如果 Worker 崩溃，其他 Worker 使用 XAUTOCLAIM 接管 Pending。

### 7.4 并发正确性

> Redis 的消息所有权不足以保护数据库结果，所以我在 PostgreSQL 中增加了租约、心跳和 fencing token。每次接管生成新 token，最终结果使用 token、running 状态和 lease 未过期作为条件更新。旧 Worker 的 UPDATE 影响 0 行，因此不能覆盖新结果。

### 7.5 安全执行

> Worker 通过 Executor Client Pool 轮询发送批量判题请求，Executor 在并发槽位内通过 Docker CLI 启动一次性容器。运行时关闭网络，限制 CPU、内存和进程数，使用只读文件系统、cap-drop ALL 和 no-new-privileges，并分别限制 stdout/stderr 为 1 MiB。Go 和 C++ 先编译一次，再复用产物执行测试用例。

### 7.6 验证

> 单元测试覆盖状态机和异常路径，integration tests 验证 PostgreSQL/Redis 语义，故障脚本验证 Worker 暂停后的租约接管。Prometheus 和 Grafana监控 API、Outbox、Queue、Worker 和 Judge，k6 基准使用相同固定到达率比较 1/2/4 Worker。

## 8. 五分钟介绍结构

不要逐个介绍框架，按以下顺序讲：

1. 系统业务闭环：登录、题库、提交、判题、历史、排行榜；
2. 为什么在线判题的难点是不可信代码；
3. API 与 Worker 为什么必须隔离；
4. Worker 与 Executor 为什么还要再隔离；
5. 为什么数据库与 Redis 存在双写窗口；
6. Transactional Outbox 如何保证任务不丢；
7. Redis Streams 如何实现 Pending 和接管；
8. 为什么还需要数据库租约与 fencing token；
9. Docker 做了哪些限制，还有哪些风险；
10. 为什么登录选择 Cookie Session 而不是 JWT；
11. 如何用测试、监控和故障注入证明设计有效；
12. 当前系统边界与下一步演进。

## 9. 可自证数据

### 9.1 固定负载基准

原始报告：[Worker Scaling Benchmark](benchmarks/2026-07-03-worker-scaling.md)

测试条件：

```text
场景：Python submission
到达率：1 req/s
持续时间：2m
执行器：constant-arrival-rate
Worker：1 / 2 / 4
每个 Worker concurrency：1
环境：macOS arm64 + Docker Desktop
```

结果：

| Worker | Created | Accepted | Judge P95 | HTTP P95 | HTTP failure | Logical failure | Dropped |
| 1 | 121 | 121 | 9489ms | 19.18ms | 0 | 0 | 0 |
| 2 | 121 | 121 | 3125ms | 17.94ms | 0 | 0 | 0 |
| 4 | 121 | 121 | 837ms | 5.37ms | 0 | 0 | 0 |

这组数据只能支持：

- 相同固定负载下，增加 Worker 降低 Judge P95；
- API 没有被判题同步阻塞；
- 测试期间没有失败和 dropped iteration；
- 每轮结束 Pending 可以归零。

它不能支持：

- 系统最大 QPS；
- 生产环境容量；
- 线性吞吐扩展；
- Go/C++ 编译型任务在 Linux 上的容量。

### 9.2 题库数据

可以表述：

```text
20 道 Hot 题 + 2 道 Starter 题，每道 Hot 题至少 6 个隐藏测试用例。
```

源码与测试：

- [题库迁移](../migrations/004_hot20_problem_set.sql)
- [题库完整性集成测试](../internal/store/hot20_reference_integration_test.go)

### 9.3 多语言

可以表述“支持 Go、C++ 和 Python 判题”，依据：

- [语言规格](../internal/judge/docker_runner.go)
- Docker Runner tests；
- 前端 Playwright 判题流程。

不能说“三语言都完成了同等压力测试”，因为当前 Worker scaling report 是 Python-only。

### 9.4 登录与排行榜

可以表述：

```text
支持注册登录、个人提交历史和全站 AC 排行榜。
```

源码依据：

- [Auth Helpers](../internal/auth/auth.go)
- [HTTP API](../internal/httpapi/server.go)
- [PostgreSQL Auth Schema](../migrations/005_auth_leaderboard.sql)
- [Leaderboard Page](../frontend/app/leaderboard/page.tsx)

实现口径：

- 密码使用 bcrypt 哈希；
- 浏览器保存 `HttpOnly` Cookie；
- 数据库保存 session token 的 SHA-256 哈希；
- 提交创建、列表、详情都按当前用户隔离；
- 排行榜按 Accepted 提交中的去重题目数排序。

## 10. 简历数据如何自证

| 简历表述 | 源码依据 | 运行证据 |
| --- | --- | --- |
| Transactional Outbox | `PostgresStore.CreateSubmission` | PostgreSQL integration tests |
| SKIP LOCKED Relay | `ClaimOutbox` | 多 Relay/Store tests |
| Redis Consumer Group | `RedisStreamsQueue` | Redis integration tests |
| Pending 接管 | `XAUTOCLAIM` | fault-test、Redis tests |
| fencing token | `CompleteSubmission WHERE judge_token` | lease/fencing tests |
| Docker 资源隔离 | `dockerSandboxArgs` | Docker Runner tests |
| Worker / Executor 隔离 | `executor.ClientPool` + `executor.Server` | executor unit tests、multi-node verification |
| 多 Worker 扩容 | Compose scale + DNS discovery | k6 report、Prometheus |
| 零基准失败 | k6 raw summaries | benchmark report |
| 故障恢复 | Processor + lease | `scripts/fault-test.sh` |
| 登录态可撤销 | `user_sessions` + `DeleteSession` | HTTP auth tests |
| 密码和 Session 安全存储 | `internal/auth` | auth unit tests |
| 用户提交隔离 | `ListSubmissionsByUser` / `GetSubmissionForUser` | HTTP/store tests |
| 排行榜聚合 | `ListLeaderboard` | leaderboard store/API tests |

面试时不要只说“有测试”，要能指出“哪个测试证明哪个性质”。

## 11. 项目难点写法

### 11.1 双写一致性

错误写法：

> 使用 Redis 实现异步判题。

推荐写法：

> 识别 PostgreSQL 提交成功但 Redis 发布失败的双写窗口，在同一数据库事务中写入 submission 与 outbox，并通过可重试 Relay 实现最终发布。

### 11.2 多 Worker 一致性

错误写法：

> 使用分布式锁防止重复消费。

推荐写法：

> Redis 提供至少一次投递，PostgreSQL 通过行锁、租约和 fencing token 控制最终写权限，使重复消息和旧 Worker 不会产生竞争终态。

### 11.3 沙箱安全

错误写法：

> Docker 完全隔离恶意代码。

推荐写法：

> 在 MVP 威胁模型下使用 Docker 限制网络和资源，并将持有 Docker Socket 的 Executor 与 API/Worker 隔离；同时明确 Docker 共享内核，仍不是强对抗多租户沙箱。

### 11.4 性能验证

错误写法：

> 系统支持高并发，吞吐线性提升。

推荐写法：

> 使用 constant-arrival-rate 固定负载测试比较 1/2/4 Worker，在本地 Python 场景中 Judge P95 从 9.49s 降至 0.84s，测试期间 HTTP/逻辑失败和 dropped iterations 为 0。

### 11.5 登录与排行榜

错误写法：

> 使用 JWT 实现完整权限系统。

推荐写法：

> 使用 bcrypt、HttpOnly Cookie 和服务端 Session 实现 Web 登录，支持退出登录和服务端撤销；提交记录绑定 user_id，并基于 Accepted 提交聚合全站排行榜。

## 12. PostgreSQL 与 MySQL 的简历处理

当前仓库实际使用 PostgreSQL，不能在简历中直接替换成 MySQL。

推荐写法：

```text
PostgreSQL（熟悉 MySQL 8 等价实现）
```

面试回答：

> 项目实现使用 PostgreSQL，核心依赖事务、行锁、条件更新和 SKIP LOCKED。我日常更熟悉 MySQL，MySQL 8 InnoDB 可以通过 SELECT FOR UPDATE SKIP LOCKED、联合索引和事务实现同样的 Outbox 与租约模型，但需要改写 UPDATE FROM RETURNING、ON CONFLICT 和时间类型，并重新验证默认隔离级别。

不要写：

```text
使用 MySQL 实现 Transactional Outbox
```

除非仓库已经真正增加 MySQL 实现和测试。

## 13. 不应出现在简历里的表述

避免：

- exactly-once；
- 分布式事务；
- 生产级绝对安全沙箱；
- 线性扩展；
- 最大吞吐达到某个未经测量的 QPS；
- Kubernetes 高可用部署；
- MinIO 只完成编排、未实际用于测试用例和 artifact；
- 完整用户权限系统；
- JWT 双 token 刷新体系；
- 比赛排行榜、封榜或罚时赛制；
- gVisor 或 Firecracker 已落地。

可以说：

- at-least-once + 幂等效果；
- Transactional Outbox；
- Docker MVP 沙箱；
- 多 Worker 故障接管；
- 固定负载延迟改善；
- 小样例仍可存 PostgreSQL，大测试用例和判题 artifact 已支持走 MinIO；
- 支持基础注册登录、个人提交历史和全站 AC 排行榜；
- gVisor/Firecracker 是后续演进方向。

## 14. 面试前自检清单

必须能解释：

- [ ] 为什么返回 202；
- [ ] 为什么不能数据库提交后直接 XADD；
- [ ] Outbox 为什么可能重复发布；
- [ ] XREADGROUP、Pending、XACK、XAUTOCLAIM 的关系；
- [ ] 为什么结果成功后才 ACK；
- [ ] lease 和 fencing token 分别解决什么问题；
- [ ] Worker 崩溃后如何恢复；
- [ ] Worker/Executor 边界、Docker 限制项及 Executor 持有 Docker Socket 的风险；
- [ ] 用户错误为什么不重试，基础设施错误为什么重试；
- [ ] 为什么登录用 Cookie Session，不用 JWT；
- [ ] 排行榜的统计口径是什么，为什么不是比赛榜；
- [ ] 固定负载和最大吞吐测试的区别；
- [ ] PostgreSQL 方案如何迁移到 MySQL 8；
- [ ] 哪些功能目前没有实现。

## 15. 英文简历参考

```text
GoJudge — Online Code Judge
Go, PostgreSQL, Redis Streams, Docker, Prometheus, Grafana

- Built a Go-based online judge with user authentication, personal submission history, leaderboard ranking, and isolated API, worker, and sandbox-executor processes for Go, C++, and Python submissions.
- Implemented revocable web authentication with bcrypt, HttpOnly cookies, server-side sessions, and hashed session tokens; scoped submission reads by user and aggregated leaderboard rows from accepted submissions.
- Implemented a transactional outbox and Redis Streams consumer group with delayed acknowledgements, pending recovery, retries, and dead-letter handling.
- Used PostgreSQL leases, heartbeats, and fencing tokens to reject stale worker results under duplicate delivery and worker failover.
- Kept Docker access inside an authenticated, concurrency-bounded sandbox executor; applied network, CPU, memory, PID, filesystem, capability, and output limits and validated scaling through Prometheus and reproducible k6 benchmarks.
```

## 16. 相关学习材料

- 完整源码教学：[codingjudge-complete-tutorial.md](codingjudge-complete-tutorial.md)
- 面试题手册：[codingjudge-interview-handbook.md](codingjudge-interview-handbook.md)
- 基准报告：[benchmarks/2026-07-03-worker-scaling.md](benchmarks/2026-07-03-worker-scaling.md)
- Executor 拓扑报告：[benchmarks/2026-09-09-executor-topology.md](benchmarks/2026-09-09-executor-topology.md)
- 项目 README：[README](../README.md)
