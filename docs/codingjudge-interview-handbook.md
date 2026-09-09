# GoJudge 面试问答手册

> 使用方式：先背“简短回答”，再理解“完整回答”，最后能指向“源码依据”。面试官继续深入时，从“常见追问”展开。
>
> 事实边界：项目提供 at-least-once 投递和幂等效果，不宣称 exactly-once；Docker 是 MVP 隔离，不是绝对安全沙箱；k6 报告是固定负载延迟测试，不是最大吞吐测试。

## 1. 基础与整体架构

### Q1：这个项目解决什么问题？

**简短回答**

GoJudge 是一个支持登录、提交、个人历史、排行榜和 Go/C++/Python 判题的在线代码评测系统，重点解决不可信代码隔离、异步任务可靠投递和多 Worker 并发判题正确性。

**完整回答**

系统提供题库、用户登录、代码提交、状态轮询、判题结果、个人提交历史和排行榜。API 不直接执行代码；Worker 通过 PostgreSQL 租约与 fencing token 控制结果写权限，再调用独立 Sandbox Executor 编译和运行代码。只有 Executor 持有 Docker Socket。

**源码依据**

- [领域模型](../internal/domain/domain.go)
- [API 入口](../cmd/api/main.go)
- [Worker 入口](../cmd/worker/main.go)
- [Executor 入口](../cmd/executor/main.go)

**常见追问**

- 为什么在线判题比普通 CRUD 难？
- 为什么一定要拆 Worker？
- 系统目前缺少哪些产品能力？

### Q2：完整请求链路是什么？

**简短回答**

API 先校验 Cookie Session，再在 PostgreSQL 事务中写带 user_id 的 submission 和 outbox，Relay 发布到 Redis Streams，Worker 获取数据库租约后调用 Executor 判题，结果写库成功再 XACK，前端轮询终态。

**完整回答**

主链路是：Cookie Session 认证 → POST submission → 数据库事务 → Outbox Relay → XADD → XREADGROUP/XAUTOCLAIM → ClaimSubmission → Executor compile/run → CompleteSubmission 条件更新 → XACK → GET submission。数据库是业务状态权威源，Redis 只负责消息分发。

**源码依据**

- `PostgresStore.CreateSubmission`
- `outbox.Relay.PublishBatch`
- `judgeworker.Processor.ProcessJob`

**常见追问**

- 哪些步骤允许重复？
- 哪一步失败会造成任务丢失？
- 为什么 XACK 放在最后？

### Q3：为什么使用异步判题？

**简短回答**

代码编译和执行耗时且风险高，同步处理会占用 HTTP 连接并把用户程序故障传递给 API。

**完整回答**

API 返回 202 和 submission ID，判题在 Worker 中完成。这样 API 延迟不受编译运行时间直接影响，Worker 可以独立扩容和限制权限，失败也能通过队列重试和租约接管恢复。

**源码依据**

- [创建提交 Handler](../internal/httpapi/server.go)
- [Worker Processor](../internal/judgeworker/processor.go)

**常见追问**

- 为什么不在 API 中启动 goroutine？
- 为什么不用 WebSocket 同步等待？
- 202 与 200 的语义差异是什么？

### Q4：系统有哪些服务？

**简短回答**

Frontend、API、Worker、Sandbox Executor、PostgreSQL、Redis、MinIO、Prometheus 和 Grafana；MinIO 用于 object-backed 测试用例文件和提交 artifact。

**完整回答**

API 负责 HTTP 和持久化，Relay 位于 API 进程但独立运行；Worker 消费任务、拉取 MinIO 测试用例、维护租约并调用 Executor；Executor 验证 Bearer Token、限制并发并持有 Docker Socket；PostgreSQL 保存权威数据、租约、fencing token 和 object metadata；Redis 管理消息；Prometheus/Grafana 负责观测。服务通过 Docker Compose 编排。

**源码依据**

- [docker-compose.yml](../docker-compose.yml)

**常见追问**

- Relay 为什么放在 API 进程？
- 如何拆成独立服务？
- MinIO 当前实际做了什么？

### Q5：项目的核心设计原则是什么？

**简短回答**

风险隔离、数据库作为权威源、至少一次投递、资源端 fencing、失败可恢复、监控不进入关键路径。

**完整回答**

API 不运行代码；Redis 消息可重复但最终写入必须幂等；租约过期后旧 Worker 必须被数据库拒绝；Prometheus 和 Grafana 故障不能中断判题；所有异步边界都明确处理崩溃窗口。

**源码依据**

- [完整教程](codingjudge-complete-tutorial.md)
- [可靠 Worker 设计](superpowers/specs/2026-07-02-reliable-multi-worker-design.md)

**常见追问**

- 哪个原则最重要？
- 哪些保证来自数据库？
- 哪些能力只是 MVP？

## 2. Go 代码与工程结构

### Q6：为什么使用 net/http + chi？

**简短回答**

保留标准库 Handler 模型，同时使用 chi 的路由参数和路由模板，依赖较轻且便于测试。

**完整回答**

Server 实现 `http.Handler`，测试可以直接使用 httptest。chi 提供 `/submissions/{id}` 路由和 RoutePattern，后者还能避免 Prometheus 使用真实 ID 形成高基数 label。

**源码依据**

- [HTTP Server](../internal/httpapi/server.go)
- [HTTP Middleware](../internal/httpapi/middleware.go)

**常见追问**

- 与 Gin 相比有什么差异？
- 为什么不直接使用 Go 1.22 ServeMux？
- chi RoutePattern 为什么影响监控？

### Q7：项目如何使用小接口？

**简短回答**

接口定义在使用方，例如 HTTP 依赖 ProblemStore，Worker 依赖 LeaseStore，Relay 依赖 OutboxStore。

**完整回答**

每层只看到需要的方法，MemoryStore 和 PostgresStore 可以复用同一业务代码，测试可注入 fake。接口不是为了抽象所有数据库，而是为了隔离业务边界和外部依赖。

**源码依据**

- `httpapi.ProblemStore`
- `store.LeaseStore`
- `store.OutboxStore`
- `judge.Runner`

**常见追问**

- 为什么接口不统一放到 domain 包？
- 接口是否过度拆分？
- 如何增加 MySQLStore？

### Q8：Context 在项目里有什么作用？

**简短回答**

用于请求取消、进程关闭、Worker 到 Executor 的 HTTP 取消、Docker 超时、Redis 阻塞读取和心跳生命周期控制。

**完整回答**

HTTP Context 传递到 Store；根 Context 监听系统信号；Docker 使用 WithTimeout；Processor 用 judgeCtx 同时控制判题和心跳；Pool 使用 acquireCtx 与 workCtx 区分停止接单和强制终止。

**源码依据**

- [API main](../cmd/api/main.go)
- [Worker Pool](../internal/judgeworker/pool.go)
- [Docker Runner](../internal/judge/docker_runner.go)

**常见追问**

- 为什么不能把 Context 保存到结构体长期复用？
- Context 取消后 Docker 如何停止？
- 为什么 Pool 需要两个 Context？

### Q9：Worker 如何实现并发？

**简短回答**

每个 Worker 进程根据 concurrency 创建多个独立 slot，每个 slot 一个 Consumer ID 和 Processor goroutine；最终同时运行的沙箱数还受 Executor 并发槽位限制。

**完整回答**

slot 独立调用 XREADGROUP，Redis Consumer Group 负责分配消息。进程扩容和 slot 扩容都能提高并发，但生产更倾向多进程，便于资源限制、故障隔离和调度。

**源码依据**

- [Worker main](../cmd/worker/main.go)
- [Worker Pool](../internal/judgeworker/pool.go)

**常见追问**

- 一个进程 100 个 slot 有什么问题？
- 如何限制 Docker 并发？
- goroutine 泄漏如何测试？

### Q10：如何优雅关闭 Worker？

**简短回答**

先取消 acquireCtx 停止领取新任务，等待当前任务；超过 grace 后再取消 workCtx。

**完整回答**

一个 Context 直接取消会立即中断当前 Executor 请求，并继续传递到 Docker 执行。双 Context 让系统先排空已有任务，超时才强制结束，兼顾正确性与部署关闭时间。

**源码依据**

- [Pool.Run](../internal/judgeworker/pool.go)

**常见追问**

- 强制取消后消息怎么办？
- shutdown grace 应如何设置？
- API 的优雅关闭有何不同？

## 3. HTTP、业务状态与数据库

### Q11：为什么创建提交返回 202？

**简短回答**

请求已可靠接受，但判题尚未完成，202 表示异步处理已开始。

**完整回答**

API 返回 queued submission 和 ID，前端随后轮询。若返回 200 容易让调用方误以为业务已经完成；若同步等待则把 Docker 延迟传递给 HTTP。

**源码依据**

- `httpapi.Server.createSubmission`

**常见追问**

- 创建失败返回什么？
- 如何提供任务进度？
- 是否需要 Location Header？

### Q12：提交接口做了哪些校验？

**简短回答**

限制请求体和代码大小，校验 JSON、必填字段、语言和题目存在性，并隐藏响应中的源码。

**完整回答**

请求体最大约 65 KiB，代码最大 64 KiB；只接受 Go/C++/Python；题目必须存在；成功后才增加 submission metric。统一错误结构方便前端处理。

**源码依据**

- [server.go](../internal/httpapi/server.go)

**常见追问**

- 为什么请求体上限比代码上限多 1024 字节？
- 如何防止 JSON 后追加多余内容？
- 为什么不返回源码？

### Q13：Submission 状态机是什么？

**简短回答**

queued → running → accepted/wrong_answer/runtime_error/time_limit_exceeded/internal_error；基础设施重试会 running → queued。

**完整回答**

终态不会重新执行。租约过期可由新 Worker 在 running 状态接管。用户程序结果直接进入终态，基础设施错误才释放为 queued 并重试。

**源码依据**

- [domain.go](../internal/domain/domain.go)
- [Processor](../internal/judgeworker/processor.go)

**常见追问**

- 为什么没有 compile_error？
- running 能否直接回 queued？
- 如何防止终态被覆盖？

### Q14：为什么隐藏测试用例不能返回前端？

**简短回答**

隐藏用例是判题依据，返回前端会允许用户针对答案硬编码。

**完整回答**

Store 返回完整 Problem，API Handler 在响应前清空 TestCases，Worker 则直接使用完整数据。公开 DTO 与判题 DTO 将来可以进一步拆分。

**源码依据**

- `listProblems`
- `getProblem`
- `judge.Service.Evaluate`

**常见追问**

- Handler 清空是否足够安全？
- 如何迁移到 MinIO？
- 管理员如何查看测试用例？

### Q15：为什么创建 submission 和 outbox 必须同一事务？

**简短回答**

保证提交记录和待发布意图同时成功或同时失败，消除数据库成功但消息未发送的永久丢失。

**完整回答**

事务只覆盖 PostgreSQL，不能覆盖 Redis。因此把消息意图先写到数据库，由 Relay 异步发布。如果事务回滚，两条记录都不存在；如果提交成功，Outbox 一定可被恢复。

**源码依据**

- `PostgresStore.CreateSubmission`

**常见追问**

- Redis 发布成功但数据库标记失败怎么办？
- 为什么不用两阶段提交？
- Outbox 表会不会无限增长？

### Q16：数据库索引如何设计？

**简短回答**

围绕 Outbox 待发布扫描、租约过期恢复、题库排序和标签查询设计联合或部分索引。

**完整回答**

Outbox 关注 published_at、next_attempt_at 和 id；submission recovery 关注 lease_expires_at 与状态；题库关注 collection、sort_order；索引服务具体查询，而不是给每列单独建索引。

**源码依据**

- [003_reliable_workers.sql](../migrations/003_reliable_workers.sql)
- [004_hot20_problem_set.sql](../migrations/004_hot20_problem_set.sql)

**常见追问**

- PostgreSQL 部分索引如何迁移 MySQL？
- SKIP LOCKED 查询如何看执行计划？
- 索引过多有什么成本？

## 4. Outbox 与 Redis Streams

### Q17：直接写数据库再发 Redis 有什么问题？

**简短回答**

两次写入没有共同事务，进程可能在数据库提交后、XADD 前崩溃，造成 submission 永久 queued。

**完整回答**

反过来先发 Redis 也不行，数据库失败会产生孤儿消息。Transactional Outbox 把 submission 和发送意图放进一个数据库事务，再由 Relay 负责最终发布。

**源码依据**

- [Postgres Store](../internal/store/postgres.go)
- [Outbox Relay](../internal/outbox/relay.go)

**常见追问**

- Outbox 是最终一致性还是强一致性？
- 用户何时收到 202？
- Relay 延迟如何监控？

### Q18：Outbox 能保证 exactly-once 吗？

**简短回答**

不能。它保证事件不会因双写窗口永久丢失，但发布可能重复。

**完整回答**

XADD 成功后、MarkOutboxPublished 前崩溃会再次发布。系统接受 at-least-once，通过 submission 终态检查、租约和 fencing token 处理重复。

**源码依据**

- `Relay.PublishBatch`
- `Processor.ProcessJob`

**常见追问**

- 如何实现业务幂等？
- 能否给 Redis 消息设置唯一 ID？
- exactly-once 为什么通常是端到端问题？

### Q19：SKIP LOCKED 有什么作用？

**简短回答**

多个 Relay 并发抢 Outbox 时跳过已被其他事务锁住的行，减少等待和重复抢占。

**完整回答**

ClaimOutbox 使用 CTE 选候选行、FOR UPDATE SKIP LOCKED、UPDATE FROM 和 RETURNING，把抢占和返回合并。claim expiry 负责 Relay 崩溃后的恢复。

**源码依据**

- `PostgresStore.ClaimOutbox`

**常见追问**

- SKIP LOCKED 是否返回一致快照？
- MySQL 8 如何实现？
- 为什么还需要 claimed_by？

### Q20：Redis Streams 相比 List 有什么优势？

**简短回答**

Streams 原生提供 Consumer Group、Pending、ACK、消息 ID 和消费者接管。

**完整回答**

List 的 BRPOP 获取后消息即离开队列，需要自行实现确认和恢复。Streams 把已投递未确认消息保留在 PEL，适合可靠 Worker 消费。

**源码依据**

- [redis_streams.go](../internal/queue/redis_streams.go)

**常见追问**

- 与 Kafka 有什么区别？
- Stream 是否需要裁剪？
- Consumer Group 如何扩容？

### Q21：XREADGROUP、Pending 和 XACK 的关系是什么？

**简短回答**

XREADGROUP 分配消息，消息进入 Pending；业务完成后 XACK 才从 Pending 移除。

**完整回答**

Pending 记录消息属于哪个 consumer、idle 多久和投递次数。它不是普通“未消费队列长度”，而是已交付但尚未确认的任务集合。

**源码依据**

- `RedisStreamsQueue.Dequeue`
- `RedisStreamsQueue.Ack`

**常见追问**

- XPENDING 为 0 说明什么？
- ACK 是否删除 Stream 消息？
- 如何清理历史消息？

### Q22：XAUTOCLAIM 如何恢复任务？

**简短回答**

它把 idle 超过阈值的 Pending 消息转移给当前 consumer，用于接管崩溃 Worker 的任务。

**完整回答**

Dequeue 先尝试 claim Pending，再读取新消息。Worker 正常运行时通过 Touch 刷新 idle 时间，避免长任务被过早接管。

**源码依据**

- `claimPending`
- `Touch`

**常见追问**

- claimMinIdle 应如何设置？
- 与数据库 lease 时间如何协调？
- 接管会不会导致重复执行？

### Q23：为什么结果写库后才 XACK？

**简短回答**

如果先 ACK，Worker 在结果写库前崩溃，消息会永久丢失。

**完整回答**

先写库后 ACK 可能产生重复消息，但重复可通过终态检查处理；先 ACK 后写库会产生无法恢复的数据丢失，因此选择前者。

**源码依据**

- `Processor.processClaim`

**常见追问**

- 写库成功但 ACK 失败怎么办？
- 是否需要数据库和 Redis 分布式事务？
- ACK 重复执行是否安全？

### Q24：重试和死信如何实现？

**简短回答**

基础设施错误释放数据库租约并重新 XADD，达到最大尝试次数后写 internal_error 和 dead-letter stream。

**完整回答**

RetryJob 使用 Redis TxPipeline 同时 XADD 新消息和 XACK 旧消息。用户代码错误不重试；Docker、数据库、心跳等基础设施错误才重试。默认最多 3 次。

**源码依据**

- `handleInfrastructureError`
- `RetryJob`
- `DeadLetter`

**常见追问**

- 为什么不直接重新读取同一 Pending？
- 重试是否需要退避？
- DLQ 如何人工处理？

## 5. 租约、fencing 与多 Worker

### Q25：为什么 Redis 分配消息后还要数据库 Claim？

**简短回答**

Redis 只管理消息投递，数据库才保存权威业务状态和最终结果。

**完整回答**

消息可能重复或被接管，Consumer 所有权也可能过期。ClaimSubmission 在数据库行锁下判断终态和租约，只有获得新 token 的 Worker 才能执行和写入结果。

**源码依据**

- `PostgresStore.ClaimSubmission`
- `Processor.ProcessJob`

**常见追问**

- 能否只使用 Redis 分布式锁？
- 为什么 Claim 要使用 FOR UPDATE？
- 两个 Worker 同时 Claim 会怎样？

### Q26：租约解决什么问题？

**简短回答**

租约给 Worker 一个有限时间的所有权，Worker 崩溃后无需主动释放，过期即可被接管。

**完整回答**

永久锁会在持有者崩溃后卡死。租约通过 lease_expires_at 自动失效，心跳续期保持活跃任务，其他 Worker 只能在过期后接管。

**源码依据**

- `ClaimSubmission`
- `RenewSubmissionLease`

**常见追问**

- 租约多长合适？
- 心跳失败怎么办？
- 时钟偏差有什么影响？

### Q27：fencing token 解决什么问题？

**简短回答**

阻止租约已过期的旧 Worker 在恢复后覆盖新 Worker 的结果。

**完整回答**

每次认领生成新 token。CompleteSubmission 的 WHERE 同时校验 id、token、running 和 lease 未过期。旧 token 更新影响 0 行，Processor 将其视为 lease lost。

**源码依据**

- `randomToken`
- `CompleteSubmission`

**常见追问**

- 随机 token 与单调版本号哪个好？
- 为什么只用 worker ID 不够？
- token 应保存在哪里？

### Q28：心跳为什么同时更新数据库和 Redis？

**简短回答**

数据库 lease 控制写权限，Redis idle 控制 Pending 何时可被接管，两者职责不同。

**完整回答**

只续数据库不 Touch Redis，消息可能被 XAUTOCLAIM；只 Touch Redis 不续数据库，Worker 会失去结果写权限。任一步失败都取消当前判题。

**源码依据**

- `Processor.heartbeat`

**常见追问**

- 两个心跳更新能否原子？
- 更新顺序有什么影响？
- 如何减少心跳数据库压力？

### Q29：ClaimState 为什么有五种？

**简短回答**

用于明确区分获得执行权、终态重复消息、同 receipt 活跃、其他 receipt 活跃和缺失提交。

**完整回答**

Processor 根据状态决定执行、ACK、保留 Pending 或死信，避免把并发判断散落在 Worker 业务代码中。

**源码依据**

- [ClaimState](../internal/domain/domain.go)
- `ProcessJob`

**常见追问**

- active_same_receipt 为什么不 ACK？
- active_other_receipt 为什么可以 ACK？
- terminal 为什么不重新执行？

### Q30：系统如何处理重复消息？

**简短回答**

允许重复投递，但通过 ClaimState、终态检查和 fencing 条件更新保证重复消息不产生错误结果。

**完整回答**

重复消息可能在 Outbox 重发、ACK 失败或 Pending 接管时出现。终态消息只 ACK；有效租约存在时不重复抢占；真正并发时只有当前 token 可以写结果。

**源码依据**

- `ClaimSubmission`
- `ProcessJob`
- `CompleteSubmission`

**常见追问**

- 是否完全避免重复执行？
- 幂等键是什么？
- 重复 Docker 执行有什么成本？

## 6. 判题与 Docker 沙箱

### Q31：Judge Service、Executor Client 与 Docker Runner 如何分工？

**简短回答**

Judge Service 解释判题结果，Executor Client 负责跨进程调用，Docker Runner 只负责编译和执行代码。

**完整回答**

Worker 中的 Service 依赖 `judge.Runner`/`BatchRunner` 接口；默认 Compose 注入 Executor Client Pool，它用 Bearer Token 调用 `/v1/run-batch`。Executor 中的 Docker Runner 返回 stage、stdout、stderr、exit code、duration 和 timeout，Service 再映射为业务状态。边界通过接口解耦，便于 fake 测试和替换更强沙箱。

**源码依据**

- [judge/service.go](../internal/judge/service.go)
- [judge/docker_runner.go](../internal/judge/docker_runner.go)
- [executor/client.go](../internal/executor/client.go)
- [executor/server.go](../internal/executor/server.go)

**常见追问**

- 为什么比较逻辑不放 Runner？
- 如何增加远程沙箱？
- 如何支持 Special Judge？

### Q32：Go、C++、Python 的执行方式有什么不同？

**简短回答**

Go/C++ 先在独立编译容器编译一次，再复用产物运行测试；Python 直接解释执行。

**完整回答**

编译阶段上限 30 秒、512 MiB，编译失败返回 Compile Error。运行阶段使用题目限制，每个测试用例单独启动只读运行容器。BatchRunner 避免每个用例重复编译。

**源码依据**

- `languageSpecFor`
- `RunBatch`

**常见追问**

- 为什么编译限制与运行限制分开？
- 编译缓存如何做？
- 当前如何表示编译错误？

### Q33：如何判定 Accepted、WA、RE、TLE？

**简短回答**

编译阶段失败为 Compile Error；运行阶段先判断 timeout，再判断非零 exit code，再比较标准输出，全部用例通过才 Accepted。

**完整回答**

输出比较只移除末尾空白，不改变内部空格。遇到首个失败用例提前结束。没有测试用例时当前实现直接 Accepted，这是一个 MVP 限制。

**源码依据**

- `judgeRun`
- `normalizeOutput`

**常见追问**

- 浮点答案如何比较？
- 多答案题如何支持？
- 是否需要保存每个用例结果？

### Q34：Docker 做了哪些限制？

**简短回答**

关闭网络，限制内存、CPU 和 PID，根文件系统只读，移除 capabilities，启用 no-new-privileges，并限制输出。

**完整回答**

运行使用 network none、memory、cpus 1、pids-limit 64、read-only、cap-drop ALL、no-new-privileges 和 noexec tmpfs。工作目录运行时只读挂载。

**源码依据**

- `dockerSandboxArgs`
- `dockerRunArgs`

**常见追问**

- 哪项防 fork bomb？
- 只读根文件系统是否禁止所有写入？
- seccomp 是否配置？

### Q35：如何处理死循环和海量输出？

**简短回答**

Context 超时终止 Docker CLI；stdout/stderr 各自最多捕获 1 MiB。

**完整回答**

运行 deadline 是题目限制加 500ms 管理余量。limitedBuffer 达到上限后丢弃后续内容但向写入方返回成功，防止子进程因管道阻塞并避免 Worker 内存无限增长。

**源码依据**

- `runPrepared`
- `limitedBuffer`
- `executeDocker`

**常见追问**

- Docker CLI 被杀后容器一定退出吗？
- 输出超限是否应该作为单独状态？
- 为什么增加 500ms？

### Q36：Docker 是否足够安全？

**简短回答**

适合 MVP 风险控制，但不是强多租户安全边界；默认架构已把 Docker Socket 从 API/Worker 移到独立 Executor，但 Executor 仍是高权限组件。

**完整回答**

容器共享宿主机内核，Docker Socket 具有高权限。生产应把 Executor 放在专用节点，加上 TLS/mTLS、服务发现和健康摘除；更强隔离可考虑 gVisor 或 Firecracker。

**源码依据**

- [docker-compose.yml](../docker-compose.yml)
- [Docker Runner](../internal/judge/docker_runner.go)

**常见追问**

- gVisor 与 Firecracker 有什么区别？
- 为什么 cap-drop 仍不够？
- 如何隔离不同用户？

## 7. 可观测性与性能

### Q37：监控了哪些指标？

**简短回答**

HTTP、submission、Outbox、Redis Queue、Pending、Worker、retry/DLQ、lease takeover 和 Judge case。

**完整回答**

API 与每个 Worker 使用独立 Registry，并注册 Go/process collector。Grafana 按 API、Queue/Outbox、Worker、Judge 四个区域展示。

**源码依据**

- [metrics.go](../internal/metrics/metrics.go)
- [Grafana dashboard](../deploy/grafana/dashboards/gojudge-overview.json)

**常见追问**

- 哪个指标最能发现积压？
- 如何计算 Judge P95？
- Prometheus 挂了会影响判题吗？

### Q38：为什么不能用 submission ID 作为 Prometheus label？

**简短回答**

ID 值无界，会形成高基数时间序列，快速消耗 Prometheus 内存和存储。

**完整回答**

HTTP route 使用 chi 模板，例如 `/submissions/{id}`；结果、语言和状态都是有限枚举。具体 submission 应通过日志或数据库查询，不放 label。

**源码依据**

- `ObserveHTTP`
- [metrics tests](../internal/metrics/metrics_test.go)

**常见追问**

- worker ID 能否作为 label？
- error message 为什么不能作为 label？
- 高基数如何排查？

### Q39：固定负载测试与最大吞吐测试有什么区别？

**简短回答**

固定负载保持相同到达率比较延迟和积压；最大吞吐测试逐步提高负载寻找容量上限。

**完整回答**

当前 k6 使用 1 req/s、2 分钟、1/2/4 Worker，证明多 Worker 在相同压力下改善 Judge P95。它不能证明系统最大 QPS或线性扩展。

**源码依据**

- [benchmark report](benchmarks/2026-07-03-worker-scaling.md)
- [k6 submissions](../loadtest/submissions.js)

**常见追问**

- constant-arrival-rate 为什么优于固定 VU？
- dropped iterations 表示什么？
- 如何设计 saturation test？

### Q40：如何解释当前基准结果？

**简短回答**

Python 固定负载下 Judge P95 从 1 Worker 的 9.49s 降到 4 Worker 的 0.84s；另一组受控饱和实验中，双 Executor 共享 daemon 的中位吞吐是单 Executor 的 1.50 倍。

**完整回答**

第一组数据说明 API 与判题解耦，并发 Worker 降低排队延迟。第二组使用 540/540 个 AC 任务比较 Executor/daemon 拓扑，显示增加 Executor 槽位有收益，但同一 Docker Desktop VM 内独立 daemon 没有超过 shared daemon。两组都只覆盖本机 Python workload，不能外推生产容量或编译型语言性能。

**源码依据**

- [benchmark report](benchmarks/2026-07-03-worker-scaling.md)
- [executor topology report](benchmarks/2026-09-09-executor-topology.md)

**常见追问**

- 为什么 1 Worker 的 HTTP rate 反而更高？
- 为什么 Peak Pending 只是采样值？
- 如何提高结果可信度？

## 8. 故障场景

### Q41：Redis 运行中宕机会怎样？

**简短回答**

submission 和 outbox 仍可写 PostgreSQL，Relay 发布失败后记录错误并退避，Redis 恢复后继续发布。

**完整回答**

API 启动要求同时配置数据库和 Redis 地址，但创建请求不在同步路径调用 Redis。长期故障会积累 Outbox，需要监控和容量限制。

**源码依据**

- `CreateSubmission`
- `Relay.PublishBatch`

**常见追问**

- API 重启能否恢复？
- Outbox 堆积如何告警？
- Redis 恢复后是否产生重复？

### Q42：Worker 判题中崩溃会怎样？

**简短回答**

消息保留在 Pending，租约过期后其他 Worker 使用 XAUTOCLAIM 接管并生成新 token。

**完整回答**

旧 Worker 不需要主动释放。新 Worker 接管后数据库 token 变化，旧 Worker 即使恢复也无法提交结果。

**源码依据**

- [fault-test.sh](../scripts/fault-test.sh)
- `claimPending`
- `ClaimSubmission`

**常见追问**

- 恢复时间由什么决定？
- 为什么同时需要 Redis idle 和 DB lease？
- 容器临时目录如何清理？

### Q43：结果写库成功但 XACK 失败怎么办？

**简短回答**

消息仍在 Pending，重新投递后发现 submission 已是终态，只执行 ACK，不重复写结果。

**完整回答**

这就是选择“先写库后 ACK”的可恢复窗口。系统接受重复投递，用终态和条件更新实现幂等。

**源码依据**

- `processClaim`
- `ProcessJob` 的 ClaimTerminal 分支

**常见追问**

- ACK 重试是否需要单独任务？
- 消息多久会被接管？
- 会不会重复运行代码？

### Q44：XADD 成功但 MarkOutboxPublished 失败怎么办？

**简短回答**

Outbox 会再次发布同一 submission，形成重复消息，但 Worker 幂等链路可以处理。

**完整回答**

不能在 PostgreSQL 与 Redis 间建立普通本地事务，因此选择“不丢但可能重复”。重复成本低于永久丢失，最终状态由数据库控制。

**源码依据**

- `Relay.PublishBatch`
- `ClaimSubmission`

**常见追问**

- 如何减少重复发布？
- 是否可以使用 Redis 消息去重键？
- 为什么不能依赖 outbox ID 唯一？

### Q45：Worker 暂停超过租约后又恢复会怎样？

**简短回答**

新 Worker 已获得新 token，旧 Worker 的心跳或结果条件更新影响 0 行，判定 lease lost。

**完整回答**

租约负责允许接管，fencing 负责拒绝旧写。只有租约而没有 fencing 无法处理暂停后恢复的旧进程。

**源码依据**

- `RenewSubmissionLease`
- `CompleteSubmission`
- `ErrLeaseLost`

**常见追问**

- GC pause 会触发吗？
- token 是否需要单调递增？
- lease lost 后 Docker 如何停止？

### Q46：Prometheus 或 Grafana 宕机会怎样？

**简短回答**

不影响 API、队列和判题，因为监控不在关键业务路径。

**完整回答**

指标记录是进程内操作，Prometheus主动 scrape；Grafana 只查询 Prometheus。Pending sampler 失败只记录日志，不取消 API。

**源码依据**

- [redis_sampler.go](../internal/metrics/redis_sampler.go)
- [Compose](../docker-compose.yml)

**常见追问**

- Worker metrics 端口绑定失败呢？
- 指标代码会不会 panic？
- 如何做告警？

## 9. 架构演进

### Q47：如何把 PostgreSQL 替换成 MySQL？

**简短回答**

核心模式可迁移，但需要更换驱动并改写参数、类型、Upsert、部分索引和 UPDATE FROM RETURNING。

**完整回答**

MySQL 8 InnoDB 支持事务、行锁和 SKIP LOCKED。ClaimOutbox 可改成事务内 SELECT FOR UPDATE SKIP LOCKED、UPDATE、SELECT。还需重新验证默认 REPEATABLE READ、gap lock 和 UTC 时间语义。

**源码依据**

- [PostgreSQL Store](../internal/store/postgres_reliability.go)
- [完整教程 MySQL 章节](codingjudge-complete-tutorial.md#25-postgresql-与-mysql-8-对照)

**常见追问**

- 为什么不能只换连接字符串？
- 部分索引如何替代？
- MySQL 默认隔离级别有什么影响？

### Q48：如何接入 MinIO 存储测试用例？

**简短回答**

数据库保留 testcase metadata 和 object key，Worker 判题前从 MinIO 下载输入/标准输出并校验 size 和 SHA256，判题后把源码快照、stdout、stderr 作为 artifact 上传 MinIO。

**完整回答**

实现上保留 DB 文本小样例，同时在 `problem_test_cases` 增加 `input_object_key`、`expected_output_object_key`、`input_sha256`、`expected_output_sha256`、`size_bytes` 和 `hidden` 字段。Worker 在心跳保护下解析 object-backed 用例；如果 MinIO 下载失败或 checksum 不匹配，任务按基础设施错误重试。判题 artifact 使用 `submission_id + attempt + fencing_token` 生成不可覆盖 key，再通过当前 token 条件写入 `submission_artifacts`，避免旧 Worker 上传的文件被系统引用。

**源码依据**

- Compose：[docker-compose.yml](../docker-compose.yml)
- Schema：[MinIO Assets Migration](../migrations/006_minio_assets.sql)
- 用例解析：[case_assets.go](../internal/judge/case_assets.go)
- Artifact 写入：[processor.go](../internal/judgeworker/processor.go)

**常见追问**

- 如何保证题目版本一致？
- Worker 是否缓存测试包？
- MinIO 宕机如何处理？

### Q49：如何把轮询改成 SSE？

**简短回答**

保留 submission 查询作为权威接口，再增加 SSE 状态通知，断线后客户端按 ID重新查询。

**完整回答**

SSE 可以降低频繁轮询，但需要连接管理、代理超时、断线重连和多 API 实例广播。通知只能是提示，不能替代数据库状态。

**源码依据**

- 当前轮询：[use-submission-polling.ts](../frontend/hooks/use-submission-polling.ts)

**常见追问**

- 为什么不用 WebSocket？
- 多实例如何广播？
- Last-Event-ID 如何使用？

### Q50：如何升级为更安全的执行平台？

**简短回答**

将 Executor 放到专用节点，Worker 只通过 TLS/mTLS 的内部协议访问；再使用 gVisor 或 Firecracker，并增加镜像、seccomp、磁盘和系统调用策略。

**完整回答**

高安全场景可把调度 Worker 和执行节点拆开，任务通过内部协议下发；每次执行使用强隔离运行时，节点无业务凭据，执行后销毁环境。

**源码依据**

- 当前边界：[Docker Runner](../internal/judge/docker_runner.go)

**常见追问**

- gVisor 和 Firecracker如何选择？
- Kubernetes Job 是否足够？
- 如何控制镜像供应链？

### Q51：为什么登录不用 JWT，而用 Cookie Session？

**简短回答**

因为当前系统是 Web 前端同源访问，登录态需要可撤销；服务端 Session 更简单，也更适合封禁、退出登录和权限变更。

**完整回答**

JWT 适合开放 API、移动端、多服务间身份传递，优点是服务端可以少查会话表。但这个项目的前端通过 Next.js `/api` 代理同源访问 Go API，不需要把身份令牌暴露给浏览器 JavaScript，也不需要跨多个外部客户端传递身份。

这里采用 `HttpOnly Cookie + server-side session`：浏览器只保存随机 session token，数据库保存 token 的 SHA-256 哈希。退出登录时删除服务端 session，Cookie 也被清空；如果后续要封禁用户、撤销会话、修改权限，也可以立即生效。相比直接把 JWT 放到 localStorage，这个方案降低了 XSS 直接窃取 token 的风险；相比完整 JWT access/refresh/rotation/revoke-list 方案，当前复杂度更低。

面试时要强调：不是 JWT 不安全，也不是 JWT 不会用，而是这个阶段的系统更需要“可撤销登录态”和“简单可验证的 Web 会话”。如果后续拆成多服务或开放移动端 API，可以引入短期 Access Token、Refresh Token rotation 和服务端 revoke/token-version 机制。

**源码依据**

- Cookie 代理：[Next.js API Proxy](../frontend/app/api/[...path]/route.ts)
- 登录态 API：[HTTP API](../internal/httpapi/server.go)
- 密码和 Session token 哈希：[Auth Helpers](../internal/auth/auth.go)
- 用户和 Session 持久化：[PostgreSQL Store](../internal/store/postgres.go)

**常见追问**

- JWT 退出登录为什么不天然立即失效？
- 为什么 Cookie 要设置 HttpOnly 和 SameSite？
- Session 表会不会成为性能瓶颈？
- 多服务架构下如何从 Session 演进到 JWT？

### Q52：排行榜是如何实现的，和比赛榜有什么区别？

**简短回答**

当前排行榜是全站练习榜，直接从 Accepted 提交中聚合；按用户 AC 的去重题目数排序，不包含比赛时间窗口、封榜和罚时规则。

**完整回答**

提交创建时会写入 `submissions.user_id`。Worker 判题完成后只更新 submission 状态，不需要知道排行榜。`GET /leaderboard` 查询时从 PostgreSQL 聚合 accepted submission：`solved` 是 `COUNT(DISTINCT problem_id)`，`acceptedSubmissions` 是 accepted 提交次数，`lastAcceptedAt` 是最近一次 AC 时间。

排序规则是：先按 `solved` 倒序，再按 `lastAcceptedAt` 升序，最后按用户名稳定排序。这个设计适合 MVP 练习平台，优点是实现简单、数据永远来自权威 submission 表，不需要维护额外排行榜状态。

它不是比赛榜。比赛榜通常还需要 contest、contest_problems、contest_participants、比赛时间窗口、罚时、封榜、赛后重算和权限规则。如果把当前排行榜包装成比赛系统，会夸大项目范围。

**源码依据**

- 排行榜查询：[PostgreSQL Store](../internal/store/postgres.go)
- 内存实现与测试：[Memory Store](../internal/store/memory.go)
- HTTP 路由：[HTTP API](../internal/httpapi/server.go)
- 前端页面：[Leaderboard Page](../frontend/app/leaderboard/page.tsx)
- Schema：[Auth + Leaderboard Migration](../migrations/005_auth_leaderboard.sql)

**常见追问**

- 为什么不维护单独 leaderboard 表？
- 如何避免重复 AC 同一道题被算多次？
- 如果要做比赛榜，需要新增哪些表？
- 为什么当前排行榜不需要登录？

## 10. 容易说错的内容

| 错误说法 | 正确说法 |
| --- | --- |
| 保证 exactly-once | at-least-once + 幂等效果 |
| 使用分布式事务解决 PostgreSQL/Redis 双写 | 使用 Transactional Outbox 实现最终发布 |
| Redis 锁保证唯一 Worker | PostgreSQL lease + fencing 控制最终写权限 |
| Docker 完全安全 | Docker 是 MVP 隔离，仍共享内核；高权限 Socket 已集中到 Executor |
| 支持线性扩容 | 固定负载下观察到 Judge P95 改善 |
| 系统吞吐 52 QPS | HTTP rate 包含轮询，当前没有最大吞吐结论 |
| MinIO 只完成编排 | object-backed 测试用例和提交 artifact 已实际接入 MinIO，PostgreSQL 保存 key/size/SHA256 metadata |
| 使用 MySQL | 当前实际使用 PostgreSQL，可解释 MySQL 8 等价方案 |
| 编译失败映射为 Runtime Error | 已有独立 Compile Error 终态和测试 |
| 不会重复执行 | 允许重复投递和极端情况下重复执行，但旧结果不能覆盖新结果 |
| JWT 一定比 Session 更适合后端项目 | 当前 Web 同源场景选择可撤销 Cookie Session，JWT 适合开放 API 或多端身份传递 |
| 已实现比赛排行榜 | 当前是全站练习排行榜，未实现比赛、罚时、封榜和赛制规则 |

## 11. 面试复习顺序

第一轮，只背这些问题：

```text
Q2 完整链路
Q17 双写问题
Q18 Outbox 语义
Q21 Pending/ACK
Q23 ACK 顺序
Q26 租约
Q27 fencing token
Q34 Docker 限制
Q36 安全边界
Q31 Worker / Executor 执行边界
Q39 固定负载
Q51 Cookie Session
Q52 排行榜口径
```

第二轮，结合源码讲：

```text
CreateSubmission
ClaimOutbox
RedisStreamsQueue.Dequeue
Processor.ProcessJob
ClaimSubmission
CompleteSubmission
DockerRunner.RunBatch
```

第三轮，用故障场景串联：

```text
Redis 宕机
Worker 崩溃
旧 Worker 恢复
数据库成功但 ACK 失败
XADD 成功但 Outbox 未标记
监控系统宕机
```

## 12. 相关文档

- [完整源码教学](codingjudge-complete-tutorial.md)
- [简历项目说明](codingjudge-resume-project.md)
- [Worker Scaling Benchmark](benchmarks/2026-07-03-worker-scaling.md)
- [Executor Topology Benchmark](benchmarks/2026-09-09-executor-topology.md)
- [README](../README.md)
