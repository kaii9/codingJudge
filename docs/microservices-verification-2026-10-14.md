# 微服务改造验收记录

日期：2026-10-14（Asia/Shanghai）。来源分支：feat/gojudge-microservices。源码初次验收提交：28db4f3；审查修正源码提交：a97ec5c；已按用户选择将 1531538 快进合并到本地 main。四阶段完成，本地验收通过，尚未推送。

合并后重新执行 go test ./... -count=1、重点八包 race -count=1 和 go vet ./...，均通过。原有未跟踪架构文档未修改。临时工作树中的审查 ledger 已保留到主目录的 .superpowers/sdd/2026-10-13-gojudge-microservices。

## 原目标闭环

| 能力 | 实现 | 验收依据 |
|---|---|---|
| 服务边界 | 平台 / 判题 / Executor 三域；Judge API 与 Worker 同属判题域，只有 Executor 持有 Docker Socket | 运行中平台、Judge API、Worker mounts 均为空；真实三语言判题 |
| 数据归属 | 两个数据库与独立账户；提交使用不可变题目快照；排行榜消费结果投影 | 跨库 CONNECT 被拒绝；快照重放、原子事件、去重和迟到事件集成测试 |
| 调用治理 | gRPC 鉴权、deadline、取消、有界 admission、Health/Watch、三次 attempt 与独立收尾预算 | 真实 loopback RPC 故障测试；任务连续超时最终终止；无隐式 RunBatch 重放 |
| 独立部署 | 独立镜像和迁移 target；旧协议 fixture；兼容的公开 HTTP API | 重建一个 Executor 时平台、Judge API、Worker 容器 ID 不变 |
| 故障验证 | 租约接管、fencing、结果 Outbox 与幂等投影、drain 和孤儿沙箱清理 | 实际 SIGKILL / 强制 drain 后第 2 次 attempt 达到 AC；旧 token 被拒绝；两条 Stream Pending 归零 |

## 已执行的验证

| 验证 | 结果 |
|---|---|
| go test ./... | 通过 |
| 重点包 race | 通过；macOS 系统 linker 输出 LC_DYSYMTAB 警告，无 race/test failure |
| go vet ./... | 通过 |
| 两个专用数据库的集成测试 | 通过：快照重放、事件事务回滚、旧 token、重复/迟到投影、跨库 CONNECT 拒绝 |
| 真实 Docker 回归 | 通过：短程序预算、非特权身份、控制密钥不可读、TLE 清理、owner 隔离清理 |
| Compose 配置与独立镜像构建 | 通过 |
| verify-microservices.sh | Go/C++/Python AC；一个 Executor 停机后继续 AC；独立重建 Executor 后其余三个容器 ID 不变 |
| 提交历史和远程产物 | 通过；平台经 Judge RPC 下载源码产物 |
| 排行榜 | 三次 AC 同一题，solved=1、acceptedSubmissions=3 |
| Redis Streams | 判题与结果两条 Stream 的 Pending 均归零 |
| verify-microservices-faults.sh | 活跃沙箱期间 SIGKILL、100ms grace 的实际 SIGTERM，各在第 2 次 attempt 达到 AC；默认 30 秒 grace 已还原 |
| Playwright | 主目录最终复测 5 passed，2 skipped（desktop/mobile 项目中的条件性跳过），25.4 秒 |
| trace context 持久化 | 主目录复测已观察到 28 条非空 trace_parent；未配置 OTLP Collector，不宣称追踪 UI 已部署 |

## 真实验收暴露并修复的问题

第一次微服务验收的正确 Python 程序因 Docker 生命周期耗时被判 TLE，改为可信控制帧后真实三语言验收通过。最初采用 300ms 的短程序测试；最终复测在宿主调度/IO 抖动下 2/5 次超时，不能作为稳定的环境断言。现在测试在真实 Docker 启动前注入 2 秒延迟，再执行 1 秒预算的短程序，3/3 次通过，直接证明生命周期延迟不占程序预算；生产判题配置没有放宽。此处测量是观察到控制帧的墙钟时长，不是独立机器上的 CPU 时间基准。

慢产物保存超过数据库租约时，原 Worker 心跳已停止，导致完成失败。回归测试固定慢保存后，把心跳延伸到持久化阶段，测试通过。

Executor drain 后健康探测可能把服务重新置为 SERVING。生命周期回归测试失败后，加上 drain 状态保护，测试通过。

Docker 自动移除和强制移除同时发生时，原清理会返回 already in progress。清理现在有界等待该过程完成，回归测试通过。

## 独立审查修正

全分支审查报告无 Critical、两个 Important、一个 Minor，均已修正并做 RED → GREEN 回归：

- 任务总 deadline 原先同时取消重试/终止写入：执行与收尾预算分离；收尾最多 5 秒，仍响应父 Worker 停机取消，且仍须当前 token/有效租约。超限接管不再执行代码；连续三次超时可收敛为 internal_error。
- 强制 drain 原先返回永久 canceled：调用方仍存活时把服务端退出归为可重试故障；显式下一 attempt 选择健康实例，保留无隐式 RPC 重放。
- 已取消的容量等待者可能拿到刚释放的槽：在 admission commit 锁内再次检查取消并释放槽位。

删除临时工作树后的主目录故障复测：SIGKILL 为 sub-81da72a3-c213-467c-bad3-be493b44d4a4；forced-drain 为 sub-d00d2b45-d418-49cb-a6e4-6a6ff6cd8ea2，均 attempts=2、final=accepted。主目录完整验收脚本亦通过，说明运行不依赖已删除的工作树。最后一轮真实 Docker integration 全部通过，重点八包 race 和 go test ./... -count=1 通过。

## 收尾状态

- 已合并到本地 main，临时 feat/gojudge-microservices 分支和工作树已删除；所有源码提交仍保留在 main。
- PostgreSQL 初始化脚本挂载已切换到主目录，仍使用原微服务数据卷；切换前后 21 条提交和 12 个用户数量一致。后续验收生成的新测试数据另行保留，未清空既有数据。
- 用户原有未跟踪架构文档 SHA256 保持 5c26ae9ad533b80d608d70ee1bed455160057b519fefc2ce2784158d586a463f，未纳入本次提交。
- 恢复验收时本项目八个容器处于 Paused，仅 unpause 这八个服务后恢复 ready；主目录部署验收、两种运行中故障和浏览器 E2E 均通过。该过程未修改业务实现。
- 原四阶段计划没有未完成项；未执行推送或新增生产环境部署。

## 验证边界

- 本机 Docker Desktop 约 6 GiB 内存，两个 Executor 共用宿主 Docker daemon；这里的结果不是跨物理机器扩容实验。
- 运行中 Executor SIGKILL 是实际容器故障；慢响应、响应丢失与过载使用真实 loopback gRPC 和受控 Runner fixture。
- mTLS 提供配置与失败关闭逻辑；本次 Compose 使用内部明文加服务 Token，不能宣称已完成远程 mTLS 部署。
- CI 工作流已增加微服务验收 job；尚未推送，因此未声称 GitHub CI 已通过。
- 原项目数据卷未迁移或删除；微服务模式使用独立数据库和数据卷。
