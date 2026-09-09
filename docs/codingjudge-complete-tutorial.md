# GoJudge 完整教学：从 HTTP 提交到安全判题

> 适用读者：具备 Go 基础，但尚未系统学习异步任务、消息队列、分布式一致性和不可信代码执行。
>
> 本文以当前仓库真实实现为准。前端只讲清调用关系，后端重点讲业务目标、代码路径、设计取舍、故障恢复、测试依据和面试追问。

你需要能够脱离文档讲清：

```
提交
→ 登录态校验
→ PostgreSQL 事务写 submission + outbox
→ Relay 发布 Redis
→ Consumer Group 分配任务
→ Worker 获取租约和 token
→ Docker 判题
→ 条件更新结果
→ 成功后 XACK
```

并回答六个核心问题：

1. 为什么不能写完数据库直接发 Redis？
2. 为什么必须先保存结果再 ACK？
3. 租约和 fencing token 分别解决什么问题？
4. Worker 崩溃和旧 Worker 恢复时会发生什么？
5. Docker 为什么不是绝对安全？
6. 固定负载与最大吞吐测试有什么区别？
7. 为什么这个 Web 项目选择 Cookie Session 而不是 JWT？
8. 排行榜如何从判题结果中聚合出来？

## 1. 先建立正确的项目认识

GoJudge 是一个在线代码评测系统。用户选择题目、编写代码并提交后，系统异步运行代码，再返回 Accepted、Wrong Answer、Compile Error、Runtime Error 或 Time Limit Exceeded 等结果。

它表面上是几个 HTTP 接口，真正困难的是三件事：

1. **不可信代码执行**：用户可以提交死循环、内存消耗程序、恶意系统调用或海量输出。
2. **异步链路可靠性**：数据库已保存提交，但消息可能没有发到 Redis；Worker 也可能在处理中崩溃。
3. **多 Worker 并发正确性**：同一任务可能重复投递、被重新认领，旧 Worker 不能覆盖新 Worker 的结果。

当前项目的核心能力包括：

- Go API、Judge Worker 与 Sandbox Executor 进程隔离；
- PostgreSQL 持久化题目、提交、结果、租约和 Outbox；
- HttpOnly Cookie + 服务端 Session 登录；
- 提交记录绑定用户，用户只能查看自己的提交；
- 基于 Accepted 提交聚合全站排行榜；
- Redis Streams Consumer Group 分发判题任务；
- Transactional Outbox 消除数据库与 Redis 的双写丢失窗口；
- PostgreSQL 租约和 fencing token 控制结果写权限；
- Worker 通过带 Bearer Token 的内部 HTTP 协议调用并发受限的 Executor；
- Executor 使用 Docker 沙箱运行 Go、C++ 和 Python；
- 重试、死信、Pending 接管和优雅关闭；
- Prometheus、Grafana、k6 与故障恢复测试；
- MinIO 承载 object-backed 测试用例和判题 artifact，PostgreSQL 保存 object key、size、SHA256 与状态 metadata。

当前项目没有实现的内容也要明确：

- 没有比赛、管理后台、角色权限和榜单冻结；
- 小规模样例仍可保存在 PostgreSQL，大测试用例与判题证据文件已支持存入 MinIO；
- Docker 是 MVP 隔离方案，不等于强多租户安全沙箱；
- 对外状态已区分 Compile Error 和 Runtime Error；
- 当前基准是固定负载测试，不是最大吞吐量测试。

## 2. 总体架构

```mermaid
flowchart LR
    Browser["浏览器 / Next.js"] --> API["Go API"]
    API --> PG[("PostgreSQL<br/>users / sessions / submissions / outbox")]
    API --> Relay["Outbox Relay"]
    Relay --> Redis[("Redis Streams")]
    Redis --> W1["Judge Worker 1"]
    Redis --> W2["Judge Worker 2"]
    W1 --> PG
    W2 --> PG
    W1 --> Pool["Executor Client Pool"]
    W2 --> Pool
    Pool --> E1["Sandbox Executor 1"]
    Pool --> E2["Sandbox Executor 2"]
    E1 --> Docker1["Docker Daemon / Sandbox"]
    E2 --> Docker2["Docker Daemon / Sandbox"]
    API --> PM["Prometheus"]
    W1 --> PM
    W2 --> PM
    PM --> Grafana["Grafana"]
```

服务职责必须分清：

| 服务 | 主要职责 | 明确不负责 |
| --- | --- | --- |
| Frontend | 登录注册、题目展示、代码编辑、提交、轮询、排行榜 | 不执行代码、不直接访问数据库 |
| API | 参数校验、认证、查询题目、创建提交、查询结果、排行榜 | 不消费判题任务、不运行用户代码 |
| Outbox Relay | 将已提交的数据库事件可靠发布到 Redis | 不判题 |
| Redis | 任务分发、Consumer Group、Pending 记录 | 不决定最终结果写权限 |
| Worker | 消费任务、获取数据库租约、调用 Executor、写结果 | 不提供用户 HTTP API、默认不持有 Docker Socket |
| Executor | 验证内部 Bearer Token、限制并发、调用 Docker Runner | 不访问用户 Session、Redis 或业务数据库 |
| PostgreSQL | 持久化数据、事务原子性、租约、fencing | 不执行代码 |
| Docker | 隔离编译和运行环境 | 不管理业务状态 |

这体现了项目最重要的边界：**API 是低风险入口，Worker 是高风险执行组件。**

对应入口：

- API：[cmd/api/main.go](../cmd/api/main.go)
- Worker：[cmd/worker/main.go](../cmd/worker/main.go)
- Executor：[cmd/executor/main.go](../cmd/executor/main.go)
- Compose：[docker-compose.yml](../docker-compose.yml)

## 3. 一次提交的完整时序

```mermaid
sequenceDiagram
    participant F as Frontend
    participant A as API
    participant P as PostgreSQL
    participant O as Outbox Relay
    participant R as Redis Streams
    participant W as Worker
    participant E as Sandbox Executor
    participant D as Docker

    F->>A: POST /submissions + Cookie
    A->>P: SELECT session + user
    A->>P: BEGIN
    A->>P: INSERT submissions(user_id,status=queued)
    A->>P: INSERT judge_outbox
    A->>P: COMMIT
    A-->>F: 202 Accepted + submission id

    loop Outbox polling
        O->>P: ClaimOutbox(SKIP LOCKED)
        O->>R: XADD judge:submissions
        O->>P: MarkOutboxPublished
    end

    W->>R: XREADGROUP / XAUTOCLAIM
    W->>P: ClaimSubmission + lease + token
    P-->>W: ClaimAcquired
    par 判题
        W->>E: POST /v1/run-batch + Bearer Token
        E->>D: compile/run test cases
        D-->>E: stage/stdout/stderr/exit code
        E-->>W: batch results
    and 心跳
        W->>P: RenewSubmissionLease
        W->>R: XCLAIM touch Pending
    end
    W->>P: CompleteSubmission WHERE token matches
    P-->>W: update affected 1 row
    W->>R: XACK
    F->>A: GET /submissions/{id}
    A->>P: SELECT submission
    A-->>F: terminal result
```

必须记住三个顺序：

1. **先在同一数据库事务写 submission 和 outbox，再返回 202。**
2. **Worker 先获取数据库租约，再执行代码。**
3. **先成功写入最终结果，再 XACK。**

顺序颠倒会产生数据丢失或错误确认。

## 4. 代码目录如何阅读

```text
cmd/api                 API 进程装配
cmd/worker              Worker 进程装配
cmd/executor            Sandbox Executor 进程装配
internal/domain         领域模型和状态枚举
internal/httpapi        路由、Handler、中间件
internal/store          内存与 PostgreSQL Store、Outbox、租约
internal/outbox         Outbox Relay
internal/queue          内存队列与 Redis Streams
internal/judgeworker    Worker Pool、任务处理、心跳和重试
internal/executor       内部 HTTP 协议、并发槽位、Client Pool
internal/judge          判题规则与 Docker Runner
internal/metrics        Prometheus 指标和 Pending 采样
migrations              PostgreSQL 表结构与题库种子
deploy                  Prometheus、Grafana 配置
loadtest                k6 场景
scripts                 故障测试和基准脚本
frontend                Next.js 工作台
```

推荐阅读顺序不是按目录字母顺序，而是：

```text
domain
→ httpapi
→ postgres.CreateSubmission
→ outbox.Relay
→ RedisStreamsQueue
→ judgeworker.Processor
→ postgres lease/fencing
→ executor.ClientPool / executor.Server
→ judge.Service
→ DockerRunner
→ metrics/tests
```

## 5. 前端调用链简述

前端使用 Next.js、React 和 Monaco Editor。

关键文件：

- 页面入口：[frontend/app/page.tsx](../frontend/app/page.tsx)
- 顶部导航与登录态：[frontend/components/app-shell.tsx](../frontend/components/app-shell.tsx)
- 排行榜页面：[frontend/app/leaderboard/page.tsx](../frontend/app/leaderboard/page.tsx)
- 题目工作台：[frontend/components/workbench.tsx](../frontend/components/workbench.tsx)
- 编辑器：[frontend/components/code-workspace.tsx](../frontend/components/code-workspace.tsx)
- API Client：[frontend/lib/api.ts](../frontend/lib/api.ts)
- 轮询 Hook：[frontend/hooks/use-submission-polling.ts](../frontend/hooks/use-submission-polling.ts)
- 同源代理：[Next.js API 代理](../frontend/app/api/[...path]/route.ts)

前端流程：

1. 首页服务端请求题目列表并跳转到第一题。
2. 顶部栏请求 `/api/auth/me`，根据 Cookie 判断当前用户。
3. Workbench 并行加载题目列表、题目详情和当前用户提交历史。
4. Monaco 按“题目 + 语言”保存本地草稿。
5. 点击 Submit 后请求 `POST /api/submissions`。
6. Next.js 代理把请求和 Cookie 转发到内部地址 `API_INTERNAL_URL`。
7. API 返回 202 后，前端每秒轮询提交详情。
8. 状态进入终态后停止轮询并刷新历史。
9. Leaderboard 页面请求 `/api/leaderboard` 展示 AC 排名。

为什么使用 Next.js 同源代理：

- 浏览器只访问前端域名，减少 CORS 配置；
- 容器内部通过服务名 `api:8080` 通信；
- 代理再次限制请求体大小；
- 代理转发浏览器 Cookie，并保留 Go API 返回的 `Set-Cookie`；
- 前端不会暴露内部服务地址。

这里的前端重点不是复杂算法，而是把后端异步状态清晰展示出来。

## 6. 领域模型与状态机

领域类型位于 [internal/domain/domain.go](../internal/domain/domain.go)。

### 6.1 主要对象

- `Problem`：题目、限制、标签和隐藏测试用例。
- `User`：登录用户，包含 ID、用户名和创建时间。
- `Session`：服务端会话，数据库只保存 session token 哈希。
- `Submission`：用户提交、语言、代码、状态和结果。
- `JudgeResult`：stdout、stderr、退出码和耗时。
- `Job`：Redis 中传递的轻量任务。
- `OutboxEvent`：待发布事件。
- `SubmissionClaim`：Worker 对提交的认领结果。
- `LeaderboardEntry`：排行榜行，包含 rank、用户、AC 题目数和 AC 提交数。

Redis Job 不携带源代码和测试用例，只携带 submission ID、outbox ID、尝试次数和 Redis receipt。这样可以：

- 控制消息大小；
- 避免源码复制到多个系统；
- Worker 总是从权威数据库读取最新状态；
- 降低敏感信息进入消息日志的风险。

### 6.2 Submission 状态机

```mermaid
stateDiagram-v2
    [*] --> queued
    queued --> running: Worker 获得租约
    running --> queued: 基础设施失败，释放后重试
    running --> accepted
    running --> wrong_answer
    running --> runtime_error
    running --> time_limit_exceeded
    running --> internal_error
    running --> running: 租约过期后被新 Worker 接管
```

终态由 `IsTerminalSubmissionStatus` 统一判断。API、Worker 和前端必须使用同一组语义，否则可能出现后端已结束、前端仍轮询的问题。

### 6.3 Claim 状态

| ClaimState | 含义 | Processor 行为 |
| --- | --- | --- |
| acquired | 当前 Worker 获得执行权 | 执行判题 |
| terminal | 已有终态 | ACK 重复消息 |
| active_same_receipt | 同一 Pending 消息仍在活动 | 不重复执行，也暂不 ACK |
| active_other_receipt | 另一个有效任务已持有租约 | ACK 当前重复消息 |
| missing | submission 不存在 | 放入死信 |

ClaimState 把复杂并发判断封装进 Store，Processor 不需要直接理解 SQL 行锁。

## 7. API 进程如何启动

入口是 [cmd/api/main.go](../cmd/api/main.go)。

启动顺序：

1. 读取配置；
2. 校验数据库和 Redis 必须同时配置；
3. 创建信号感知 Context；
4. 创建 PostgreSQL 或 Memory Store；
5. 创建独立 Prometheus Registry；
6. Redis 模式下启动 Outbox Relay 和 Pending 采样器；
7. 创建 HTTP Server；
8. 监听 SIGINT/SIGTERM 并优雅关闭。

### 7.1 为什么数据库和 Redis 必须一起配置

`ValidateAPI` 拒绝只配置其中一个：

- 只有 PostgreSQL，没有 Redis：提交会持久化，但永远不判题；
- 只有 Redis，没有 PostgreSQL：Worker 无法获取源码、测试用例和租约；
- 两者都没有：允许内存模式用于单元测试和简单 API 演示。

这叫做 **fail fast**：在启动时暴露错误，而不是运行后制造半可用系统。

### 7.2 Context 与优雅关闭

`signal.NotifyContext` 在收到终止信号后取消根 Context。HTTP Server 使用 5 秒 shutdown deadline，Relay 和采样器监听同一个 Context。

设计取舍：

- 不直接 `os.Exit`，让正在处理的请求有机会完成；
- 设置最大等待时间，避免关闭无限阻塞；
- Worker 另有更细的“停止接新任务”和“完成当前任务”机制。

## 8. HTTP API 业务实现

路由位于 [internal/httpapi/server.go](../internal/httpapi/server.go)。

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/healthz` | 健康检查 |
| GET | `/metrics` | Prometheus 指标 |
| POST | `/auth/register` | 注册并创建登录态 |
| POST | `/auth/login` | 登录并创建登录态 |
| POST | `/auth/logout` | 删除当前登录态 |
| GET | `/auth/me` | 查询当前用户 |
| GET | `/problems` | 题目列表 |
| GET | `/problems/{id}` | 题目详情 |
| POST | `/submissions` | 创建当前用户提交 |
| GET | `/submissions` | 当前用户提交历史 |
| GET | `/submissions/{id}` | 查询当前用户判题状态 |
| GET | `/leaderboard` | 全站 AC 排行榜 |

### 8.1 登录与 Session

当前项目使用 `HttpOnly Cookie + server-side session`，而不是把 JWT 存到前端。

相关源码：

- 密码与 token helper：[internal/auth/auth.go](../internal/auth/auth.go)
- 登录 Handler：[internal/httpapi/server.go](../internal/httpapi/server.go)
- Next.js Cookie 代理：[frontend/app/api/[...path]/route.ts](../frontend/app/api/[...path]/route.ts)

注册流程：

1. 校验用户名格式：`A-Za-z0-9_-`，长度 3 到 32；
2. 校验密码长度：8 到 72；
3. 使用 bcrypt 生成密码哈希；
4. 创建 `users` 行；
5. 生成随机 session token；
6. 数据库存 token 的 SHA-256 哈希；
7. 浏览器收到 `gojudge_session` Cookie。

Cookie 设置：

```text
HttpOnly
SameSite=Lax
Path=/
MaxAge=7 days
```

为什么这样做：

- `HttpOnly` 让前端 JavaScript 不能直接读取 session token；
- 服务端保存 session，可随时删除，实现真正退出登录；
- 用户封禁、权限变化或强制下线时可以即时撤销；
- 当前项目是 Web 同源访问，不需要把身份令牌暴露给多端客户端。

JWT 并不是不能用。JWT 更适合移动端、开放 API 或多个服务之间传递身份。但如果要支持安全退出、refresh token rotation、revoke list 和权限即时生效，JWT 方案复杂度会明显上升。这个项目选择 Cookie Session 是为了用较少代码实现可撤销登录态。

### 8.2 创建提交

`createSubmission` 依次执行：

1. 读取 `gojudge_session` Cookie；
2. 对 session token 做 SHA-256，查询未过期 session 和用户；
3. 未登录返回 `401 unauthenticated`；
4. 使用 `http.MaxBytesReader` 限制整个请求；
5. 解码 JSON；
6. 验证 problemId、language、code；
7. 代码限制为 64 KiB；
8. 检查语言是否为 Go/C++/Python；
9. 验证题目存在；
10. 调用 Store 创建提交，并写入 `user_id`；
11. 成功后记录指标；
12. 清除响应中的源码；
13. 返回 `202 Accepted`。

为什么返回 202，不返回 200：

- 请求已经被可靠接受；
- 判题尚未完成；
- 客户端应通过 submission ID 查询后续状态。

### 8.3 用户提交隔离

`GET /submissions` 和 `GET /submissions/{id}` 都要求登录，并且只查询当前用户的数据。

这样做的意义：

- 后续排行榜可以直接按 `submissions.user_id` 聚合；
- 用户不能通过猜测 submission ID 查看他人提交；
- 旧的 Worker 逻辑不需要知道用户身份，仍只按 submission ID 判题。

### 8.4 为什么不把源码返回给列表接口

创建、详情和列表响应都会清空 `Code`。这可以：

- 减小响应体；
- 避免提交历史无意泄露源码；
- 和用户隔离一起形成更清楚的读取边界。

### 8.5 排行榜

`GET /leaderboard` 不要求登录，直接按已 Accepted 的提交聚合：

```text
solved = 用户 AC 过的去重题目数
acceptedSubmissions = 用户 AC 提交次数
lastAcceptedAt = 最近一次 AC 时间
```

排序规则：

1. `solved` 多的在前；
2. `lastAcceptedAt` 更早的在前；
3. 用户名作为稳定排序。

这是一个全站练习榜，不是比赛榜。它没有比赛时间窗口、封榜、罚时和赛制规则。

### 8.6 错误响应

统一格式：

```json
{
  "error": {
    "code": "unsupported_language",
    "message": "unsupported language"
  }
}
```

稳定的机器可读 `code` 比只返回自然语言更适合前端判断。

### 8.7 中间件

[internal/httpapi/middleware.go](../internal/httpapi/middleware.go) 提供：

- AccessLog：记录 method、path、status、duration；
- ObserveHTTP：按 chi 路由模板记录指标。

指标使用 `/submissions/{id}`，不能使用真实 `/submissions/sub-123` 作为 label，否则每个 ID 都会创建新时间序列，造成高基数问题。

## 9. Store 抽象与两种实现

HTTP 层依赖 `ProblemStore`，Worker 依赖 `LeaseStore`，Relay 依赖 `OutboxStore`。接口由使用方定义，体现 Go 的小接口原则。

登录和排行榜加入后，HTTP 层的 Store 接口还包含：

- `CreateUser`
- `GetPasswordHashByUsername`
- `CreateSession`
- `GetUserBySessionTokenHash`
- `DeleteSession`
- `ListSubmissionsByUser`
- `GetSubmissionForUser`
- `ListLeaderboard`

这些方法仍然放在 HTTP 使用方接口中，而不是让 Worker 依赖用户系统。Worker 不需要知道用户、密码、Session 或排行榜，只需要通过 submission ID 判题。

### 9.1 Memory Store

[internal/store/memory.go](../internal/store/memory.go) 使用 map + RWMutex：

- 便于单元测试；
- 不依赖外部服务；
- 能复用同一套业务接口；
- 模拟租约和 Outbox 状态机。
- 模拟用户、Session、按用户提交隔离和排行榜聚合。

它不是生产数据库，也不能让 API 和独立 Worker 跨进程共享数据。

### 9.2 PostgreSQL Store

[internal/store/postgres.go](../internal/store/postgres.go) 使用 pgxpool：

- 连接池负责复用连接；
- 所有查询接收 Context；
- `RowsAffected()==1` 用于判断条件更新是否成功；
- nullable 字段扫描到指针后转换为领域对象。
- `ErrConflict` 将用户名唯一约束错误映射为业务冲突。
- 排行榜使用 SQL 聚合 `COUNT(DISTINCT problem_id)`，不维护单独物化表。

这层的职责是持久化和并发原子性，不承担 HTTP 或 Docker 逻辑。

## 10. PostgreSQL 表结构

迁移文件：

- [migrations/001_init.sql](../migrations/001_init.sql)
- [migrations/003_reliable_workers.sql](../migrations/003_reliable_workers.sql)
- [migrations/004_hot20_problem_set.sql](../migrations/004_hot20_problem_set.sql)
- [migrations/005_auth_leaderboard.sql](../migrations/005_auth_leaderboard.sql)

核心表：

### 10.1 problems

保存题面、难度、集合、排序、时间和内存限制。标签拆到 `problem_tags`，测试用例拆到 `problem_test_cases`。

### 10.2 submissions

除基础字段外，还保存：

| 字段 | 作用 |
| --- | --- |
| user_id | 提交所属用户，用于提交隔离和排行榜聚合 |
| judge_token | fencing token |
| judge_worker_id | 当前租约持有者 |
| judge_receipt | 对应 Redis 消息 ID |
| lease_expires_at | 租约到期时间 |
| judge_attempts | 已认领次数 |
| last_error | 最近基础设施错误 |

### 10.3 users 与 user_sessions

`users` 保存账号信息：

| 字段 | 作用 |
| --- | --- |
| id | 用户 ID |
| username | 展示用户名 |
| username_normalized | 小写归一化用户名，唯一约束 |
| password_hash | bcrypt 密码哈希 |
| created_at | 创建时间 |

`user_sessions` 保存服务端会话：

| 字段 | 作用 |
| --- | --- |
| token_hash | session token 的 SHA-256 哈希，主键 |
| user_id | 所属用户 |
| expires_at | 过期时间 |
| created_at | 创建时间 |

注意：数据库不保存原始 session token。浏览器 Cookie 里是随机 token，服务端每次收到后先哈希再查询。这降低了数据库泄露时直接复用 token 的风险。

### 10.4 judge_outbox

| 字段 | 作用 |
| --- | --- |
| submission_id | 对应提交，唯一 |
| published_at | 是否成功发布 |
| claimed_by | Relay 所有者 |
| claim_expires_at | Relay 抢占过期时间 |
| publish_attempts | 发布次数 |
| next_attempt_at | 下次重试时间 |
| last_error | 最近发布错误 |

索引围绕实际查询条件设计：

- 提交恢复：状态 + lease expiry；
- Outbox 发布：published、next attempt、id；
- 题目：collection、sort order；
- 标签：tag、problem id。
- 登录：session token hash、session user 和过期时间；
- 提交历史：user_id、updated_at、id；
- 排行榜：status、user_id、problem_id、updated_at。

### 10.5 题库与隐藏测试用例

题库由 `problems`、`problem_tags` 和 `problem_test_cases` 三部分组成。当前种子包含 20 道 Hot 题和 2 道 Starter 题。

API 查询题目时，Store 会读取测试用例，但 Handler 在响应前把 `TestCases` 清空。这样做是因为隐藏用例只服务于 Worker 判题，不能发送给浏览器，否则用户可以针对固定答案硬编码。

同一个 `GetProblem` 接口同时服务 API 和 Worker，因此“是否暴露测试用例”的策略放在 HTTP 边界，而不是从 Store 中删除数据：

- Worker 调用 Store，得到完整题目和测试用例；
- API Handler 调用 Store 后，主动移除 TestCases；
- 领域模型保持统一，不需要维护两套题目结构。

更严格的系统可以拆成 `ProblemSummary`、`PublicProblem` 和 `JudgeProblem` 三种 DTO，进一步降低误返回隐藏数据的风险。

## 11. Transactional Outbox

### 11.1 它解决什么问题

错误做法：

```text
INSERT submission
COMMIT
XADD Redis
```

如果数据库提交后、XADD 前进程崩溃，用户已经拿到 submission ID，但任务永久丢失。

反过来先发 Redis：

```text
XADD Redis
INSERT submission
```

如果 Redis 成功、数据库失败，Worker 会收到一个不存在的 submission。

普通本地事务不能同时覆盖 PostgreSQL 和 Redis。项目采用 Transactional Outbox：

```text
BEGIN
INSERT submission
INSERT outbox
COMMIT
```

submission 和“未来要发送的意图”位于同一个数据库事务，因此要么同时存在，要么同时不存在。

实现位于 `PostgresStore.CreateSubmission`。

### 11.2 Relay 如何并发发布

[internal/outbox/relay.go](../internal/outbox/relay.go) 每 250ms 拉取一批事件。

`ClaimOutbox` 使用：

- `FOR UPDATE SKIP LOCKED`：多个 Relay 不互相等待同一批行；
- claim owner + expiry：Relay 崩溃后，其他实例可重新认领；
- `UPDATE ... RETURNING`：抢占和返回事件合并成一个原子 SQL。

发布成功后调用 `MarkOutboxPublished`。更新条件包含 owner，防止过期 Relay 把新 Relay 的状态覆盖。

发布失败则：

1. 清除 claim；
2. 保存 last error；
3. 计算指数退避；
4. 设置 next attempt；
5. 等待后续轮询重试。

退避从 250ms 开始，最大 30 秒，避免 Redis 故障时形成高频重试风暴。

### 11.3 Outbox 的“重复发布窗口”

如果 XADD 成功，但 `MarkOutboxPublished` 前 API 崩溃，该事件会再次发布。

所以 Outbox 解决的是“不丢”，不天然保证“不重复”。后续 Redis Consumer Group、租约和幂等终态写入共同处理重复。

### 11.4 设计取舍

为什么不直接使用 Kafka 事务：

- MVP 只有 PostgreSQL 和 Redis；
- 引入 Kafka 增加部署和学习成本；
- 最终仍需要处理消费者幂等；
- Outbox 更能展示数据库事务和故障窗口理解。

为什么不让 API 同步等待 Redis：

- 增加接口延迟；
- Redis 故障会让提交不可用；
- 仍然无法原子覆盖数据库提交和 Redis 写入。

### 11.5 高频追问

**问：Outbox 能保证 exactly-once 吗？**

不能。它提供事件最终可发布的基础，发布可能重复。系统整体采用 at-least-once 投递，通过租约、fencing token 和终态幂等实现“重复消息不产生错误结果”。

## 12. Redis Streams 队列

实现位于 [internal/queue/redis_streams.go](../internal/queue/redis_streams.go)。

关键名称：

```text
Stream: judge:submissions
Group: judge-workers
Dead letter: judge:submissions:dead
```

### 12.1 初始化

`XGROUP CREATE ... MKSTREAM ... 0`：

- Stream 不存在时自动创建；
- 起点 0 允许消费已有消息；
- BUSYGROUP 表示组已存在，不视为错误。

### 12.2 入队

`XADD` 写入：

- submission_id；
- outbox_id；
- attempt。

Redis 不保存源码和测试数据。

### 12.3 正常消费

`XREADGROUP` 使用 `>` 读取从未分配给 Consumer Group 的新消息。消息被读取后进入 Pending Entries List，只有 XACK 后才从 Pending 中移除。

### 12.4 Pending 接管

每次 Dequeue 先尝试 `XAUTOCLAIM`：

- 找到 idle 超过 30 秒的 Pending；
- 将消息转移给当前 consumer；
- 继续处理崩溃 Worker 遗留任务。

为什么先 claim Pending，再读新消息：优先清理可能卡住的旧任务，降低永久积压风险。

### 12.5 Touch

Worker 心跳时调用 `XCLAIM MinIdle=0` 刷新消息 idle 时间。数据库租约和 Redis Pending 的活跃时间必须同时维护：

- 数据库租约决定谁能写结果；
- Redis Pending 决定何时允许其他 Consumer 接管消息。

### 12.6 ACK 时机

正确顺序：

```text
完成数据库终态写入
→ XACK
```

如果先 ACK 再写数据库，Worker 在两步之间崩溃，消息已经消失而结果仍是 running。

当前设计中，如果数据库写成功但 XACK 失败：

- 消息仍在 Pending；
- 后续被重新认领；
- Store 发现 submission 已是终态；
- Processor 只执行 ACK，不再运行代码。

这就是幂等恢复。

### 12.7 重试与死信

基础设施错误时，Redis 使用 TxPipeline 在同一 Redis 事务中：

1. XADD 新消息；
2. XACK 旧消息。

达到最大次数后，目标 Stream 改为 dead letter。

格式错误的消息也会被复制到死信流并 ACK，避免同一坏消息无限阻塞消费者。

### 12.8 设计取舍

为什么选 Redis Streams，不选普通 List：

- Consumer Group；
- Pending 可见；
- ACK；
- 消费者接管；
- 多 Worker 横向扩展。

为什么不依赖 Redis 锁决定结果所有权：

- Redis 消息可能重复；
- 锁过期与长任务存在竞争；
- 结果本身保存在 PostgreSQL；
- 在权威数据库上做条件更新更容易保证最终一致性。

## 13. Worker 启动与并发槽位

入口是 [cmd/worker/main.go](../cmd/worker/main.go)。

Worker 必须配置 PostgreSQL 和 Redis。每个进程可通过 `WORKER_CONCURRENCY` 创建多个 slot，每个 slot 拥有独立 consumer ID：

```text
worker-id-slot-1
worker-id-slot-2
...
```

每个 slot 是一个 Processor goroutine。这样扩容有两个维度：

- 增加 Worker 进程；
- 增加单进程 concurrency。

生产环境通常优先增加进程或容器，隔离性和资源调度更清晰。

## 14. Processor：判题业务核心

核心实现：[internal/judgeworker/processor.go](../internal/judgeworker/processor.go)。

### 14.1 ProcessJob 主流程

```text
生成随机 token
→ ClaimSubmission
→ 根据 ClaimState 分支
→ 读取题目
→ 启动判题与心跳
→ 条件写入终态
→ XACK
```

token 使用 16 字节加密随机数，编码为十六进制字符串。

### 14.2 为什么 Redis 已分配消息，还要数据库 Claim

Redis Consumer Group 只能说明“消息当前分给谁”，不能保证：

- 消息没有重复；
- Worker 没有失去数据库写权限；
- 旧 Worker 不会在超时后继续写结果；
- submission 不是终态。

数据库 Claim 才是业务所有权判定。

### 14.3 行锁认领

`ClaimSubmission` 在事务中：

1. `SELECT ... FOR UPDATE` 锁住 submission；
2. 判断终态、有效租约或过期租约；
3. 生成新 expiry；
4. attempts 加一；
5. 写入 worker、receipt 和 token；
6. 提交事务。

行锁保证两个 Worker 不会同时完成“检查后都认为自己获得租约”。

## 15. 租约、心跳与 fencing token

这是项目最值得面试展开的部分。

### 15.1 只有租约为什么不够

假设 Worker A 获得 30 秒租约，然后发生长时间暂停：

1. A 的租约过期；
2. Worker B 接管并完成任务；
3. A 恢复运行；
4. A 也尝试写结果。

如果只检查“我曾经获得过租约”，A 会覆盖 B。

### 15.2 fencing token 如何阻止旧 Worker

每次成功认领都会生成新 token。最终更新条件包含：

```sql
WHERE id = ?
  AND judge_token = ?
  AND status = 'running'
  AND lease_expires_at > now
```

B 接管后数据库里的 token 已变化。A 使用旧 token 更新时影响 0 行，于是得到 `ErrLeaseLost`，无法覆盖新结果。

fencing 的本质是：**资源端必须拒绝旧世代请求。**

### 15.3 心跳

判题运行期间，另一个 goroutine 定期：

1. 延长 PostgreSQL lease；
2. Touch Redis Pending。

任一步失败都会取消判题 Context。

默认值：

- lease：30 秒；
- heartbeat：10 秒；
- heartbeat 必须短于 lease。

### 15.4 为什么结果更新还检查 lease 未过期

仅 token 相同仍可能出现“当前 Worker 已超时但新 Worker 还没接管”的窗口。检查 expiry 可以阻止已失效租约继续提交结果。

### 15.5 高频追问

**问：token 用自增版本号是否更好？**

可以。单行版本号递增天然表达世代顺序。当前随机 token 只需要保证不可预测和极低碰撞，并通过等值条件拒绝旧写入。面试时可说明两者都属于 fencing 标识。

**问：Worker 和数据库时间不一致怎么办？**

当前时间由应用传入数据库查询，存在时钟偏差风险。更严格实现可以统一使用数据库时间，或保证节点 NTP 同步。

## 16. 基础设施错误、业务结果与重试

必须区分两类失败。

### 16.1 用户程序结果

- Wrong Answer；
- Runtime Error；
- Time Limit Exceeded；
- Accepted。

这些是正常业务终态，不应重试。相同代码重新执行通常不会改变答案，重复只会浪费资源。

### 16.2 基础设施错误

- Docker CLI 无法启动；
- 数据库查询失败；
- 心跳失败；
- Redis Touch 失败；
- Runner 返回系统错误。

这些可能是暂时故障，应重试。

### 16.3 重试流程

未达到最大次数：

1. ReleaseSubmission：验证 token 后把状态改回 queued；
2. 清除租约字段；
3. 保存 last_error；
4. Redis 新增带 attempt 的消息；
5. ACK 旧消息。

达到最大次数：

1. 将 submission 写成 internal_error；
2. 写入死信 Stream；
3. ACK 原消息。

默认最大尝试次数为 3。

### 16.4 为什么先 Release 再重新入队

如果先入队，新消息可能立刻被其他 Worker 读取，但数据库仍显示旧 Worker running，造成无意义竞争。

### 16.5 组合错误

代码使用 `errors.Join` 保留原始错误和清理错误。例如 Docker 失败后，Release 又失败，日志不能只保留后一个错误，否则根因会丢失。

## 17. Worker Pool 与优雅关闭

实现：[internal/judgeworker/pool.go](../internal/judgeworker/pool.go)。

Pool 使用两个 Context：

- acquireCtx：是否继续领取新任务；
- workCtx：当前任务是否继续运行。

收到退出信号时：

1. 先取消 acquireCtx，不再领取新任务；
2. 等待正在执行的任务；
3. 超过 shutdown grace 后取消 workCtx；
4. 等待所有 slot 退出。

如果直接取消一个公共 Context，正在编译或运行的任务会立即中断，增加 Pending 接管和临时资源清理压力。

## 18. Judge Service：如何判定结果

实现：[internal/judge/service.go](../internal/judge/service.go)。

Judge Service 不知道 Redis、租约或 HTTP。它只负责：

```text
题目 + 语言 + 代码
→ Runner 执行测试用例
→ 比较结果
→ JudgeResult
```

结果判定顺序：

1. TimedOut → Time Limit Exceeded；
2. ExitCode 非 0 → Runtime Error；
3. 输出不相等 → Wrong Answer；
4. 所有用例通过 → Accepted。

输出比较只去除末尾空格、换行、回车和 Tab，不修改内部空格。

### 18.1 BatchRunner

DockerRunner 实现 BatchRunner：

- Go/C++ 只编译一次；
- 多个测试用例复用产物；
- 每个用例仍在单独运行容器中执行；
- 遇到超时或非零退出提前停止。

这避免每个测试用例都重新编译。

### 18.2 设计边界

Judge Service 与 Runner 通过接口解耦，因此单元测试可以使用 fake Runner，不需要真的启动 Docker。

### 18.3 当前限制

题目没有测试用例时直接 Accepted。这适合早期 MVP，但严格系统应拒绝发布无测试用例题目。

编译阶段通过 `RunResult.Stage` 与运行阶段区分，编译超时或非零 ExitCode 最终映射为独立 Compile Error 状态。

## 19. Docker Runner

跨进程边界实现在 [internal/executor](../internal/executor)，Docker 执行实现在 [internal/judge/docker_runner.go](../internal/judge/docker_runner.go)。默认 Compose 中，Worker 使用轮询 Client Pool 调用 Executor，Executor 再调用 Docker Runner；未配置 Executor URL 时仍保留本地 Runner 作为开发降级路径。

### 19.1 工作目录

每次提交创建临时目录：

```text
/tmp/codingjudge-xxxx/
  main.go / main.cpp / main.py
  input.txt
  program
```

函数结束时 `defer os.RemoveAll` 清理。

### 19.2 语言流程

Go：

```text
go build main.go → program → program < input.txt
```

C++：

```text
g++ -O2 -std=c++17 main.cpp → program → program < input.txt
```

Python：

```text
python main.py < input.txt
```

Go/C++ 编译容器：

- 最多 30 秒，避免冷启动编译被运行时限误杀；
- 512 MiB；
- 工作目录读写挂载。

运行容器：

- 使用题目内存限制；
- 工作目录只读挂载；
- 时间限制为题目限制加 500ms 管理余量。

### 19.3 沙箱参数

```text
--network none
--memory <limit>m
--cpus 1
--pids-limit 64
--read-only
--cap-drop ALL
--security-opt no-new-privileges
--tmpfs /tmp:rw,noexec,nosuid,size=64m
```

分别防御：

| 参数 | 主要作用 |
| --- | --- |
| network none | 禁止外部网络 |
| memory | 限制内存 |
| cpus | 限制 CPU 配额 |
| pids-limit | 防 fork bomb |
| read-only | 容器根文件系统只读 |
| cap-drop ALL | 移除 Linux capabilities |
| no-new-privileges | 防止提权 |
| tmpfs noexec | 临时目录不可执行 |

stdout 和 stderr 分别最多捕获 1 MiB。limitedBuffer 在达到上限后继续报告写入成功，但不继续占用内存，最后追加 truncated 标记。

### 19.4 Context 超时

`exec.CommandContext` 在 Context 取消时终止 Docker CLI。Runner 根据 Context 状态把结果标记为 TimedOut，而不是把正常超时当成基础设施异常重试。

### 19.5 安全边界

默认 Compose 已经让 API 和 Worker 都不挂载 Docker Socket，只有 Executor 持有它。获得该 Socket 的进程通常拥有接近宿主机 root 的 Docker 控制能力。因此：

- API 和 Worker 都不应挂载 Docker Socket；
- Executor 应运行在独立主机或受控节点；
- 跨节点协议应从开发环境 Bearer Token 升级为 TLS/mTLS 和服务发现；
- 不应把当前 Docker 方案宣传为强对抗多租户沙箱；
- 更高安全等级可使用 gVisor、Firecracker、Kubernetes 隔离节点或专用执行集群。

## 20. 关键故障场景推演

### 场景 A：数据库提交后 API 崩溃

submission 与 outbox 已在同一事务提交。API 重启后 Relay 仍会发现 outbox 并发布，不丢任务。

### 场景 B：Redis 宕机

Relay 发布失败，保存 next_attempt_at 和 last_error，指数退避。提交仍保存在 PostgreSQL，Redis 恢复后继续发布。

### 场景 C：XADD 成功，标记 published 前崩溃

Outbox 会再次发布，产生重复消息。Worker 通过数据库 ClaimState 和终态检查避免重复执行或重复写结果。

### 场景 D：Worker 运行中崩溃

消息留在 Pending，数据库租约最终过期。其他 Worker 使用 XAUTOCLAIM 接管，并获得新 token。

### 场景 E：旧 Worker 恢复

旧 token 的条件更新影响 0 行，返回 lease lost，无法覆盖新 Worker。

### 场景 F：结果写库成功，XACK 失败

消息被重新认领时数据库已是终态，Processor 只 ACK，不重新判题。

### 场景 G：用户提交死循环

运行 Context 超时，Docker 进程被终止，结果为 Time Limit Exceeded。

### 场景 H：用户输出无限数据

limitedBuffer 只保留前 1 MiB，Worker 内存不会随输出无限增长。

### 场景 I：Worker 收到不存在的 submission

消息进入死信流，避免无限重试。

### 场景 J：Prometheus 或 Grafana 停止

它们不在判题依赖链上。指标不可采集，但 API、Relay、Redis 和 Worker 继续工作。

## 21. 可观测性

指标实现：[internal/metrics/metrics.go](../internal/metrics/metrics.go)。

覆盖：

- HTTP 请求量与延迟；
- 创建提交数；
- Outbox 发布结果与耗时；
- Redis 操作和 Pending；
- Worker slot、in-flight、结果和耗时；
- retry、dead letter、lease takeover；
- 测试用例结果和耗时。

### 21.1 为什么使用自定义 Registry

API 和每个 Worker 创建独立 Registry，避免全局注册器：

- 测试之间不会互相污染；
- 一个进程只暴露自己的指标；
- 可以显式注册 Go 和 Process Collector。

### 21.2 Label 设计

允许：

- method；
- route template；
- status class；
- language；
- bounded result。

禁止：

- submission ID；
- worker ID；
- receipt；
- 原始 URL path；
- error message。

后者会产生无限 label 值，导致 Prometheus 内存和存储膨胀。

### 21.3 Pending 采样

API 每 5 秒执行 XPENDING，并写入全局 Gauge。只由 API 采样，避免每个 Worker 都上报同一个全局值后被错误求和。

### 21.4 Worker 指标服务

Worker 在启动 Pool 前同步绑定 metrics 端口。端口冲突应立即终止启动，不能让 Worker 已经消费任务但监控端口绑定失败。

Prometheus 通过 Docker DNS 自动发现扩容后的 Worker。

## 22. k6 固定负载测试

文件：

- [loadtest/submissions.js](../loadtest/submissions.js)
- [scripts/run-worker-scale-benchmark.sh](../scripts/run-worker-scale-benchmark.sh)
- [docs/benchmarks/2026-07-03-worker-scaling.md](benchmarks/2026-07-03-worker-scaling.md)

场景使用 constant-arrival-rate：

```text
每秒 1 个提交
持续 2 分钟
分别运行 1、2、4 个 Worker
```

严格校验：

- created == iterations；
- accepted == created；
- HTTP failure == 0；
- logical failure == 0；
- dropped iterations == 0；
- 实际提交量接近期望；
- 每轮结束 Pending == 0。

当前本地 Python-only 结果：

| Worker | Judge P95 | HTTP P95 |
| --- | ---: | ---: |
| 1 | 9489ms | 19.18ms |
| 2 | 3125ms | 17.94ms |
| 4 | 837ms | 5.37ms |

正确解读：

- 固定相同到达率下，增加 Worker 降低排队和判题延迟；
- API 延迟始终较低，说明判题没有阻塞 HTTP 请求；
- 这不能证明最大吞吐量，也不能直接当成生产容量；
- macOS Docker Desktop 结果不等于 Linux 原生 Docker；
- Go/C++ 编译型语言应在 Linux 环境单独测量。

### 22.1 Executor 拓扑饱和实验

[受控拓扑报告](benchmarks/2026-09-09-executor-topology.md)固定 4 个 Worker、Python workload 和每个 Executor 1 个槽位，以三轮平衡顺序比较：

| 拓扑 | 中位吞吐 | 相对单 Executor |
| --- | ---: | ---: |
| 1 Executor / 1 daemon | 4.267/s | 1.00x |
| 2 Executor / 1 shared daemon | 6.390/s | 1.50x |
| 2 Executor / 2 independent daemon | 5.979/s | 1.40x |

540/540 个测量任务全部 AC，且每轮结束 Pending/Lag 归零。正确结论是“在该宿主机上，增加 Executor 槽位缓解了瓶颈，但在同一 Docker Desktop VM 内拆分 daemon 没有带来额外收益”。它不能证明独立机器的线性扩展，下一步应在两台 VM 重复实验并增加 Executor 健康摘除/恢复。

## 23. 测试体系

### 23.1 单元测试

重点测试：

- Handler 参数和错误响应；
- 状态机和终态判断；
- 输出比较；
- Docker 参数和输出截断；
- Processor 各 ClaimState；
- retry、dead letter、lease lost；
- Outbox 发布结果；
- Metrics 名称和 label；
- Pool 优雅关闭。

### 23.2 集成测试

使用 build tag `integration`：

- PostgreSQL Store；
- 行锁、租约接管、fencing；
- Redis Consumer Group；
- Pending 接管；
- Hot20 题库种子。

运行：

```bash
TEST_DATABASE_URL='postgres://codingjudge:codingjudge@localhost:15432/codingjudge_test?sslmode=disable' \
TEST_REDIS_ADDR='localhost:16379' \
go test -tags=integration ./internal/store ./internal/queue
```

### 23.3 race detector

```bash
go test -race ./...
```

它对 MemoryStore、Pool、Processor 测试特别重要，用于发现 map、goroutine 和状态访问竞争。

### 23.4 故障测试

[scripts/fault-test.sh](../scripts/fault-test.sh) 验证 Redis/Worker 故障后的恢复，而不是只验证理想路径。

### 23.5 CI

[.github/workflows/ci.yml](../.github/workflows/ci.yml) 包含：

- Go 单元测试；
- race test；
- PostgreSQL/Redis 集成测试；
- go vet；
- 前端 lint、typecheck、Vitest、build；
- Prometheus 配置检查；
- Compose loadtest profile 检查。

## 24. Docker Compose 与部署

Compose 启动：

- frontend；
- api；
- worker；
- executor；
- postgres；
- redis；
- minio；
- prometheus；
- grafana。

依赖使用 health condition，而不是只依赖容器“已启动”。数据库进程启动不等于已经接受连接。

API 容器：

- read-only root；
- tmpfs；
- drop all capabilities；
- no-new-privileges。

Worker 也 drop capabilities 且默认不挂载 Docker Socket。Executor 挂载 Socket，因此它才是需要部署到专用节点的高风险组件。

Dockerfile 使用多阶段构建：

1. Go 镜像编译 api、worker、executor 和迁移/导入工具；
2. Alpine 运行镜像只保留二进制、证书和 docker-cli。

同一镜像通过不同 command 启动 API、Worker 或 Executor，减少构建重复。

## 25. PostgreSQL 与 MySQL 8 对照

核心架构可以迁移到 MySQL 8 InnoDB，但不能只修改连接字符串。

| PostgreSQL | MySQL 8 |
| --- | --- |
| pgxpool | database/sql + MySQL driver |
| $1 参数 | ? 参数 |
| BIGSERIAL | BIGINT AUTO_INCREMENT |
| TIMESTAMPTZ | DATETIME(6)，应用统一 UTC |
| ON CONFLICT | ON DUPLICATE KEY UPDATE / INSERT IGNORE |
| array_agg | JSON_ARRAYAGG、GROUP_CONCAT 或单独查询 |
| 部分索引 | 普通联合索引或生成列 |
| UPDATE FROM RETURNING | 事务内 SELECT FOR UPDATE SKIP LOCKED + UPDATE + SELECT |

MySQL Outbox 抢占可以设计为：

```text
BEGIN
SELECT ids
  FROM judge_outbox
 WHERE ...
 ORDER BY id
 LIMIT N
 FOR UPDATE SKIP LOCKED

UPDATE judge_outbox
   SET claimed_by=?, claim_expires_at=?
 WHERE id IN (...)

SELECT claimed rows
COMMIT
```

迁移时重点重新验证：

- InnoDB 默认隔离级别与 PostgreSQL 的差异；
- gap lock 对范围查询的影响；
- UTC 时间与租约过期；
- 联合索引是否覆盖 claim 查询；
- RowsAffected 的驱动行为；
- SKIP LOCKED 多消费者并发测试。

面试时可以说：

> 当前实现使用 PostgreSQL，但核心设计依赖事务、行锁、条件更新和 SKIP LOCKED。MySQL 8 InnoDB 可以实现等价语义，只需改写 PostgreSQL 特有 SQL、时间类型和索引，并重新验证默认隔离级别下的锁行为。

## 26. 设计取舍总表

| 决策 | 选择 | 没选方案 | 原因 |
| --- | --- | --- | --- |
| API 框架 | net/http + chi | 重型框架 | 标准库风格、边界清晰 |
| 持久化 | PostgreSQL + pgx | ORM | SQL 和锁行为透明 |
| 队列 | Redis Streams | List/同步判题 | Consumer Group、Pending、ACK |
| 双写一致性 | Transactional Outbox | DB 后直接 XADD | 避免提交成功但任务丢失 |
| 投递语义 | at-least-once + 幂等 | exactly-once 宣称 | 符合真实故障模型 |
| 所有权 | DB lease + token | 只用 Redis 锁 | 权威数据侧拒绝旧写 |
| 执行边界 | Worker → Executor HTTP | Worker 持有 Docker Socket | 把高权限执行面与队列/业务状态分离 |
| 沙箱 | Docker | 宿主机 exec | 基础资源和权限隔离 |
| 扩容 | 多 Worker + slot | API 内 goroutine | 风险隔离、独立伸缩 |
| 状态查询 | 前端轮询 | WebSocket/SSE | MVP 简单可靠 |
| 监控 | Prometheus/Grafana | 日志单一观测 | 可聚合延迟、吞吐、队列 |
| 测试 | fake + integration + fault | 只写单测 | 并发和外部系统语义需真实验证 |

## 27. 高频面试追问

### 27.1 为什么 API 不直接运行代码？

不可信代码可能消耗资源、阻塞线程、访问网络或触发内核漏洞。独立 Worker 可以单独扩容、限制权限、隔离故障，并让 API 保持低延迟。

### 27.2 为什么用了 Redis 还需要 PostgreSQL 租约？

Redis 管消息分发，PostgreSQL 管权威业务状态。消息可能重复或接管，最终写权限必须在保存结果的数据库侧校验。

### 27.3 系统能保证 exactly-once 吗？

不能，也不应这样宣传。Redis 和 Outbox 提供 at-least-once，数据库条件更新和终态判断提供幂等效果。

### 27.4 fencing token 和分布式锁有什么区别？

锁表示当前持有权，fencing token 让资源端识别请求世代并拒绝旧请求。仅有会过期的锁无法阻止暂停后恢复的旧 Worker。

### 27.5 为什么完成结果后才 ACK？

保证崩溃时消息仍可恢复。若 ACK 后写库前崩溃，任务会永久丢失。

### 27.6 Redis 挂了，用户还能提交吗？

可以，但有前提。API 启动配置要求 PostgreSQL 和 Redis 地址同时存在；运行期间 Redis 宕机后，创建请求本身只在 PostgreSQL 事务中写 submission 与 outbox，因此仍可返回 202，Relay 会持续重试。长期故障会积累 Outbox，必须监控积压并规划数据库容量。

### 27.7 如何避免重复判题？

不能保证完全不重复执行，但能保证重复执行不产生错误终态。ClaimState 尽量减少重复执行，fencing token 保证只有有效 Worker 能写结果。

### 27.8 为什么不用消息中的代码直接判题？

代码和测试用例体积大且敏感；数据库是权威来源；轻量消息更容易重试和演进。

### 27.9 Docker 是否绝对安全？

不是。Docker 共享宿主机内核；默认架构虽然已把 Docker Socket 集中到独立 Executor，Executor 仍然是高权限组件。生产高风险场景应使用专用节点、TLS/mTLS 和更强隔离运行时。

### 27.10 为什么基准中 Worker 越多 P95 越低？

固定到达率下，并发执行槽位增加，任务等待时间降低。该数据证明的是特定环境下的延迟改善，不是最大吞吐能力。

## 28. 推荐动手练习

按顺序完成：

1. 用 curl 创建一次 Python submission，观察 queued → running → accepted。
2. 在 Redis 执行 XPENDING，观察消息处理前后变化。
3. 停止 Redis，创建提交，观察 outbox，恢复 Redis 后确认最终发布。
4. 判题中停止一个 Worker，等待 lease 过期，观察另一个 Worker 接管。
5. 构造死循环，确认 Time Limit Exceeded。
6. 构造错误输出，确认 Wrong Answer。
7. 构造非零退出，确认 Runtime Error。
8. 查询 Prometheus 的 worker in-flight 和 queue pending。
9. 将 Worker 从 1 扩到 2，观察 Prometheus target 自动发现。
10. 阅读 Processor 测试，手画 ClaimState 决策表。
11. 用 MySQL 伪 SQL 重写 ClaimOutbox，说明事务边界。
12. 构造编译失败，确认 Compile Error 终态、指标与前端展示一致。
13. 启动双 Executor 拓扑，检查请求轮询分流、独立 daemon ID 和最终 Pending/Lag 归零。

## 29. 常用命令

```bash
# 后端测试
make test
go test -race ./...
go vet ./...

# 完整 Compose
make compose-up

# 两个 Worker + 监控
make observability-up

# Grafana 配置验证
bash scripts/verify-grafana.sh

# Redis Pending
docker compose exec -T redis redis-cli XPENDING judge:submissions judge-workers

# Worker 扩容
docker compose up -d --scale worker=4

# k6 smoke
make load-smoke

# 固定负载 Worker 基准
make load-worker-scale

# 故障恢复
make fault-test
```

## 30. 最终记忆框架

面试前至少能不看代码讲清这条链路：

```text
API 校验
→ PostgreSQL 事务写 submission + outbox
→ Relay 使用 SKIP LOCKED 抢占并 XADD
→ Redis Consumer Group 分配任务
→ Worker 用行锁获取 lease + fencing token
→ 心跳续租并 Touch Pending
→ Docker 编译/运行
→ Judge Service 判定结果
→ 条件更新终态
→ 成功后 XACK
→ 前端轮询展示
```

再记住四条原则：

1. **API 不执行不可信代码。**
2. **Outbox 负责不丢，幂等负责不怕重复。**
3. **租约决定时间窗口，fencing token 拒绝旧写。**
4. **先写结果再 ACK，故障后才能恢复。**

掌握这四条，再结合故障场景解释代码，这个项目就不再是一组框架和中间件，而是一套完整、可论证的后端系统设计。
