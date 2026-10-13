# GoJudge 微服务改造设计

日期：2026-10-13。状态：用户已在会话批准整体方案并要求执行。

## 目标与边界

把现有 GoJudge 演进为平台、判题、Executor 三个业务域，保留公开 HTTP API、前端和判题规则。平台负责用户、会话、题目；判题域负责提交、幂等、任务、租约、结果、产物元数据；Executor 负责受限代码执行。判题 API、Relay、Worker 是同一个业务域的不同部署单元，可访问该域数据库。

不引入微服务框架、服务网格或 Kubernetes。使用 Go 标准库、现有 chi、gRPC/Protobuf、PostgreSQL、Redis Streams、MinIO、Compose。保留单仓库。使用独立数据库凭据证明边界，禁止运行时跨库 JOIN/写表。

## 迁移约束

- 原有公开 HTTP 路由和状态码保持兼容。
- 现有 submission + outbox 原子性、数据库幂等、租约与 fencing 保留。
- 消息是 at-least-once，允许重复执行，不承诺物理 Exactly Once。
- 原 main 与未提交教学文档保持原样；工作在 feat/gojudge-microservices。
- 保留原部署模式作为回滚路径，以新 Compose overlay 演示微服务模式。
- 对历史数据的迁移使用显式工具/流程，不自动删除现有数据库或数据卷。
- 各步骤提供测试和验收记录；未经用户要求不推送或合并。

## 1. 内部协议与调用治理

内部协议使用 gojudge.executor.v1 和 gojudge.judge.v1。Executor 提供 RunBatch。判题 API 提供 CreateSubmission、FindSubmission、GetSubmission、ListSubmissions、ListArtifacts 与 GetArtifact、统计查询。协议与业务类型分离，不暴露数据库对象结构。

外部 HTTP 保留，内部默认采用 gRPC。迁移期间通过显式配置选 HTTP 或 gRPC，禁止失败后偷偷回落协议并重放 RunBatch。Executor 标准 Health/Watch 参与服务选择。内部请求验证服务 Token；远程环境配置 TLS/mTLS，本地 Compose 的明文仅为内网演示。

gRPC 客户端禁用执行请求自动重试；Worker 统一处理执行 attempt。错误分类至少包含 invalid_request、unauthorized、unavailable、overloaded、deadline、canceled、protocol、execution_unknown。参数、鉴权与协议错误不重试；过载/不可达有界等待；已发出后断开/超时视为结果未知。健康选择和错误分类保持 HTTP 模式兼容。

客户端整体 deadline 传入 Executor，服务端限制等待容量和时间，并把取消传给 Runner。业务 TLE/CE/RE/WA 是成功 RPC 中的判题结果，不能作为基础设施错误重试。Executor 退出先 NOT_SERVING，再 drain，超时取消并等待清理。

## 2. 数据归属与判题服务提取

平台数据库保存 users、user_sessions、problems、problem_test_cases、problem_tags。判题数据库保存 submissions、judge_outbox、submission_artifacts、submission_problem_snapshots 及判题完成事件。

判题表不再通过 FK 引用平台 users/problems，保留外部标识；判题域内 FK 保留。同一 PostgreSQL 实例可提供两个数据库与不同账户，各自迁移。平台凭据无法访问判题表，Worker 不拥有平台数据库凭据。

平台鉴权后通过内部 RPC 创建/查询提交；判题 RPC 从经认证服务接受用户身份，并按 user_id 检查读取权限。用户不能通过公开请求选择其他身份。平台在提交创建时提供服务端取得的题目快照，包含限制、隐藏用例或对象清单。判题域保存快照，后续 Worker 读取提交对应的快照，不访问平台库。快照与 submission/outbox 同事务创建。幂等 fingerprint 以客户端请求为准，重放使用首次快照。

## 3. 完成事件与排行榜

最终结果条件更新和 SubmissionFinished outbox 同事务提交。事件包括 event_id、submission_id、user_id、problem_id、status、version、finished_at；不包含源码或隐藏数据。

平台独立消费完成事件，把 processed_event 和 projection 更新放进同一事务。排行榜从平台读模型聚合并结合本域用户资料；重复事件、重复 AC、同一道题多次 AC 不重复计算 solved。按提交版本防止迟到旧事件回退状态。支持从事件重建读模型；查询提交的权威状态仍经判题 RPC。

## 4. 独立部署、追踪与验证

独立镜像 targets：platform、judge-api、worker、executor、migrate。平台/判题镜像不包含 docker-cli。迁移独立执行，兼容演进采用 expand/contract。新旧 RPC 客户端/服务端用契约 fixture 测试，冻结旧字段编号，新增可选字段，未知枚举防守处理。

追踪沿 HTTP -> Judge RPC 与 Outbox -> Worker -> Executor 建立关联。trace context 写入消息；重试建立新的 attempt span。日志带 submission_id/attempt/execution_id；禁止记录源码、测试数据和 Token。现有 Prometheus 指标保留，增加治理指标。

## 验收矩阵

- 正确 Go/C++/Python 程序在微服务模式下均 AC；原有登录/历史/产物/排行榜 E2E 保持。
- 单 Executor 故障时健康节点继续处理，故障节点恢复后重新接流量。
- 慢响应受 deadline 限制，Runner 收到取消，资源清理。
- 响应丢失不被判为 WA；可重复执行但最终结果 fencing 正确。
- Worker 失联后接管，旧 token 更新结果失败。
- 等待容量满时明确过载，不无界阻塞。
- 所有 Executor 故障时有界恢复策略，不热循环耗尽资源。
- 结果与事件同事务；重复事件排行榜不重复计数。
- 平台账号不能读写判题表，Worker 不访问平台数据。
- 只升级一个 Executor，旧 Worker 和其他服务保持运行。
- go test ./...、race、真实基础设施集成、Compose 配置及实际故障脚本有结果记录。

## 执行记录规则

四阶段依次实施，进展写入对应计划 ledger。依赖缺失先使用可验证替代路径并记录；若必须用户提供外部账户/生产资源则报告限制。用户已授权执行，此设计及计划不再重复要求批准。
