# 微服务改造验收记录

日期：2026-10-14（Asia/Shanghai）。开发分支：feat/gojudge-microservices。源码初次验收提交：28db4f3；审查修正源码提交：a97ec5c。四阶段完成，本地验收通过；尚未合并或推送。

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
| Playwright | 最终复测 5 passed，2 skipped（desktop/mobile 项目中的条件性跳过），40.4 秒 |
| trace context 持久化 | judge_outbox 已观察到 7 条非空 traceparent；未配置 OTLP Collector，不宣称追踪 UI 已部署 |

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

最终故障复测提交：SIGKILL 为 sub-aef40e17-b888-4b3c-af12-6b6c745f56c2；forced-drain 为 sub-0c729c4b-e371-4ce2-8832-84d95eb7fc62，均 attempts=2、final=accepted。最后一轮真实 Docker integration 全部通过，重点八包 race 和 go test ./... -count=1 通过。

## 验证边界

- 本机 Docker Desktop 约 6 GiB 内存，两个 Executor 共用宿主 Docker daemon；这里的结果不是跨物理机器扩容实验。
- 运行中 Executor SIGKILL 是实际容器故障；慢响应、响应丢失与过载使用真实 loopback gRPC 和受控 Runner fixture。
- mTLS 提供配置与失败关闭逻辑；本次 Compose 使用内部明文加服务 Token，不能宣称已完成远程 mTLS 部署。
- CI 工作流已增加微服务验收 job；尚未推送，因此未声称 GitHub CI 已通过。
- 原项目数据卷未迁移或删除；微服务模式使用独立数据库和数据卷。
