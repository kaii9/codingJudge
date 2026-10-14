# 2. Outbox、Streams、租约与 fencing

这四项不是四种重复的锁。Outbox 保证本地提交与发消息意图一致；Streams 提供消费组、待确认消息和恢复入口；租约决定临时所有权；fencing 决定哪个所有者的写入仍有效。

## 2.1 两条 Outbox 为什么需要

直接“写数据库 → 发 Redis”会留下已入库但没消息的提交；反过来发消息再写库又会留下消费者找不到的数据。把 submission、snapshot、judge_outbox 放在 Judge 一个本地事务里，提交成功便留下可恢复的发布意图。不跨库做分布式事务。

结果同理：最终状态更新与 completion_outbox 同事务。如果只改状态然后发结果事件，事件丢失会让平台排行榜永久落后。事务 Outbox 保留了重发依据，但不能使 PostgreSQL 和 Redis 同时原子提交。

| 故障点 | 留下的状态 | 恢复与边界 |
| --- | --- | --- |
| 本地事务提交前崩溃 | 全部回滚 | 客户端可用同一幂等键重试 |
| commit 后 HTTP/RPC 响应丢失 | 已有提交与 Outbox | Find / Create 重放原提交，不创建第二份 |
| XADD 前 Relay 崩溃 | 未发布 Outbox | claim 到期后 Relay 接管 |
| XADD 成功、Mark published 失败 | 消息已存在，Outbox 仍未发布 | 可能重复 XADD，消费者必须容忍 |
| Worker 完成后、XACK 前崩溃 | 已有最终结果和完成事件，消息 pending | 重投递识别 terminal，只 ACK，不再执行 |
| Projector commit 后、ACK 前崩溃 | 投影已更新，结果消息 pending | 重投递 event_id 去重，再 ACK |

Relay 使用 `FOR UPDATE SKIP LOCKED` 分批认领，避免多个发布者等待同一批行；claim 自身有过期时间。发布失败退避从 250ms 增长，上限 30s。它减小争抢，不是“永不重复”的锁。

## 2.2 Redis Streams 的消费生命周期

`XADD` 写入 stream；`XREADGROUP ... >` 给某个 consumer 分配新消息，并留下 PEL（Pending Entries List）。业务成功后 `XACK` 删除 pending 记录。ACK 不等于从 stream 删除历史消息，存储保留需要另行治理。

Worker 失联时，其他消费者用 `XAUTOCLAIM` 接管空闲过久的 pending。当前执行者心跳触碰 pending，避免长任务被误认为失联。但 Redis 消息归属并不能独自保护 PostgreSQL 最终结果，因此还需要数据库 Claim。

必须区分：stream 长度（历史条目）、consumer group lag（尚未消费）、pending（已投递未确认）、Outbox 未发布数、数据库 running 租约。不能用一个 `XLEN` 判断系统是否完全清空。

源码：[Streams 队列](../../../internal/queue/redis_streams.go)、[真实 Redis 集成测试](../../../internal/queue/redis_integration_test.go)。Redis AOF 改善持久性，不代表无条件零丢失；若 Redis 持久数据整体丢失，已标 published 的 Outbox 不会自动全部重发，灾备恢复仍需对账/重放策略。

## 2.3 租约与随机 token 的准确含义

Worker 认领提交时将其改为 running，写入 worker ID、receipt、新随机 token、lease expiry，并增加 `judge_attempts`。默认租约 30s、心跳 10s；心跳在拉对象、执行、artifact 持久化、最终更新期间持续运行，不只覆盖 Docker 调用。

最终写入类似以下条件（以实际 Store 为准）：

```sql
UPDATE submissions SET status = $final_status
WHERE id = $id
  AND judge_token = $token
  AND status = 'running'
  AND lease_expires_at > $now;
```

更新行数为 0 就不能认为成功。A 因 GC / 网络阻塞超过租约，B 接管并换 token；A 恢复后即使拿到了旧执行结果，也无法覆盖 B。没有 B 接管时，A 的过期 token 也不能写结果。

这里 token 是随机值，靠“当前值相等”的 CAS 条件保护数据库。它不是递增序号，不能让任意外部存储比较 token 大小拒绝旧写入。更不能保证旧物理执行瞬间停止。对最终结果和 artifact metadata 的数据库保护，与对 Docker 进程取消是两层机制。

源码：[租约实现](../../../internal/store/postgres_reliability.go)、[租约测试](../../../internal/store/reliability_test.go)、[持久化全程租约测试](../../../internal/judgeworker/persistence_lease_test.go)。当前传入应用时间进行判断，多机部署需考虑时钟偏差；生产可进一步统一数据库时间，不能声称已完成时钟漂移验证。

## 2.4 五种 Claim 状态

| 状态 | Worker 动作 | 目的 |
| --- | --- | --- |
| acquired | 持有新 token，开始 attempt | 唯一有效数据库写入者 |
| terminal | ACK | 完成后重投递不再执行 |
| active_other_receipt | ACK 此重复消息 | 不干扰当前有效 receipt |
| active_same_receipt | 不执行、不 ACK | 保留当前执行对应的恢复入口 |
| missing | dead letter | 消息引用不存在的提交，不无限循环 |

“有效写入者唯一”不等于“CPU 执行只有一份”。故障时旧容器可能暂时仍运行，新 attempt 也可执行；最终持久化只能通过当前所有权条件。

## 2.5 重试有预算，而不是所有失败都重试

默认最多 3 个实际执行 attempt。RPC鉴权失败、协议错误和无效参数等永久错误不反复重试；过载、超时、取消和执行结果未知按 Worker 分类及预算处理。WA、RE、CE、TLE 是评测业务结果，通常直接完成，不当作基础设施重试。

每个任务默认 3 分钟总预算。任务预算用完，恢复还需要时间：当前恢复 Context 从未取消的父 Context 派生，最多 5s，继续要求有效 token 和未过期租约。若进程根 Context 已取消，恢复不被脱离后台继续写；pending 留给接管者。

如果先前 Worker 连续崩溃导致认领计数超过预算，新消费者可以认领来做终结恢复，但不能再执行用户代码。最终写 internal_error / dead letter，是停止无穷重试，不是假装 AC。

读 [任务预算测试](../../../internal/judgeworker/task_budget_test.go)：`TestTaskBudgetStillAllowsFencedRecovery`、`TestRepeatedTaskDeadlinesConvergeWithinAttemptBudget`、`TestExhaustedReclaimedAttemptNeverExecutesAgain`、`TestParentCancellationDoesNotDetachRecovery`。

## 2.6 Artifact 与投影的细节

artifact object key 含 submission / attempt / token，减少旧 attempt 覆盖新对象的风险；metadata 写入仍检查 token。对象 Put 成功后数据库拒绝写入，可能留下孤儿对象，当前没有完整垃圾回收。对象上传失败会记录告警并尽量继续保存可用 artifact，不是所有对象和最终结果组成一个跨存储原子事务。

Projector 用 `processed_completion_events(event_id)` 去重，和 projection 写入同一事务；更新只接受更高 version，迟到旧事件不回滚新状态。版本目前取判题 attempt，未来支持人工重判需要重新设计版本生成规则，不能自然假设现有 API 已支持重判。

## 2.7 面试时怎么证明

先讲一个具体窗口：XADD 已成功但 Mark 失败 → 重复消息 → Claim / terminal 拒绝重复执行或重复结果 → ACK。再讲另一个窗口：A 失租约、B 接管、A 迟到 → CAS 拒绝旧 token。最后解释这是 at-least-once + 幂等状态收敛，不是网络层 exactly-once。

练习：分别在提交事务、XADD、Docker执行、最终 commit、projection commit 后插入“进程退出”，逐一说明权威记录、恢复者和是否可能重复执行。
