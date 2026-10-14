# 2026-10-15 主线合并与目标收尾

用户批准合并后，`main` 从 `1cf5ded` 快进到 `75cf2af`，没有重新改写判题、迁移、鉴权或依赖实现。下述运行时和测试证据明确对应 `75cf2af`；本报告及关联状态更新属于后续文档改动，其远程结果应查看 main 上对应提交的 CI，不能用本文自行证明自身 CI 已通过。

## 1. 原定目标在 `75cf2af` 的验收情况

| 原定目标 | 结果 | 可核对依据 |
| --- | --- | --- |
| 明确服务边界、数据归属与内部协议 | 完成 | Platform/Judge/Executor 三个领域，数据库账号隔离，版本化 gRPC；[第1章](01-architecture-call-chains.md) |
| 超时、取消、健康选择及有界恢复 | 完成 | 真实 RPC、冷编译/取消、重试与幂等测试；[第2章](02-reliability-failure-windows.md)、[第3章](03-grpc-tracing.md) |
| 独立部署和真实失效恢复 | 完成 | 独立 Executor 重建、在途 SIGKILL 与强制 drain，attempt 恢复；[第4章](04-tests-ci-demo.md) |
| 跨服务可观测性 | 完成 | 实际 OTLP exporter、实例/attempt 属性、collector-backed 四服务 trace 与结构验证器 |
| 已有 mTLS 的真实传输验证 | 完成 | TLS1.3 正向、八类拒绝、过期服务器、可信证书配错误 token；默认 Compose 仍不因此变成 mTLS 部署 |
| 完整前端依赖审计阻断 | 已解决 | 移除 braces 传递链，没有豁免、omit-dev 或不兼容降级；[第7章](07-frontend-dependency-security.md) |
| 测试、面试教材和陈述边界 | 完成 | 七章教程、调用链、故障窗口、实验命令、问答与简历；[学习入口](README.md) |

上述完成结论只对应已通过的 `75cf2af`。后续文档提交 `aac45b8` 的 main CI 微服务 job 失败，见第6节；当前不能宣称最新主线全部验收通过。无需因此盲目新增框架或更多服务。生产级扩展也不可冒充已经实现。

## 2. 同一版本的远程证据

[run 38614563014](https://github.com/kaii9/codingJudge/actions/runs/38614563014) 对应完整 head `75cf2aff2ae7d153705a27135ab573c05bf0aa5d`：

- frontend、test、microservices、e2e、observability 五个 job 全部成功。
- Linux Node 22：全新 npm ci、开发依赖包含在内的完整 audit 0、两套 lint、typecheck、25 项 lint 契约、183 项应用单测和生产构建通过。
- 远程浏览器 5 PASS、2 设备互斥 SKIP，21.9s；Go/C++/Python 实际 Accepted。它是功能验收时间，不是响应延迟 SLO。
- 后端全量、race、真实 Docker/PostgreSQL/Redis/MinIO 路径、mTLS、冷编译/取消、数据库归属、独立 rollout、故障恢复和 collector 验证通过。
- 原始收尾分支与前端增量均有独立只读 review；前端两项 Minor 修正后复核无遗留问题。不能把 reviewer 没有重跑的生产实验说成它独立完成了。

历史失败没有被删掉：[2026-10-14 记录](verification-2026-10-14.md)继续描述当时的 audit、镜像、编译与浏览器问题；不能再将其中的“未合并”视作当前状态。

## 3. 本地主工作区的合并后复验

环境：macOS arm64、Go 1.25.4、Node 25.8.2、npm 11.14.1。主工作区为仓库根目录，未复制或覆盖用户未跟踪的架构文档。

| 检查 | 结果 |
| --- | --- |
| npm ci + 完整 npm audit --audit-level=high | PASS，0 漏洞 |
| npm run lint / typecheck | PASS |
| npm run test:run | PASS，25 契约 + 183 应用单测 |
| npm run build | PASS，Next 16.3.8 |
| go test ./... -count=1 / go vet ./... | PASS |
| 原微服务运行状态 | 原服务仍在运行，有健康检查的服务为 healthy；不把状态检查冒充重新部署或全套故障实验 |

首次把 npm ci 与 Go 扫描并发执行时，Go 报 `frontend/node_modules/nanoid` 不存在。npm ci 正在删除重建目录；安装结束后路径存在，重新运行完整 Go suite 与 vet 通过。这是验证任务的共享文件系统冲突，不是通过修改业务代码修复的故障。以后在同一 checkout 中先完成 npm ci，再运行 Go 的 `./...` 扫描；已安装的部分第三方包包含 Go 文件，也会被该命令发现。

## 4. 保留的限制和后续维护

- 同机双 Executor 不是跨主机 HA；两个逻辑库不是两个独立数据库集群。
- 至少一次消息/可重复执行不等于 exactly-once。数据库 fencing 和投影幂等控制最终状态，不禁止所有重复执行。
- 已有 mTLS 测试不证明生产证书 ACL、轮换、吊销或跨主机 PKI 已验收。
- 固定源码的 MinIO 兼容镜像是演示修复，不是可直接上线的受维护生产对象存储方案。
- Jaeger 内存演示、有限 trace 覆盖和本机实验不等于生产采样、长期保留或性能承诺。
- ESLint 9 的维护风险仍在第7章明示；目前完整审计 0 不等于所有组件都在维护期。兼容插件支持后再升级，不强装 peer 依赖。
- CI 对旧 Actions 的 Node runtime 和 ubuntu-latest 迁移存在非阻断提示。后续升级 Actions/固定 runner 时应单独验证所有 job；本轮没有把提示消失说成新的验收结果。

这些不阻断本轮既定简历目标；若将目标升级为生产多租户或跨主机部署，需要另行定义验收，不能仅凭当前 CI 绿灯宣布生产安全。

## 5. 面试准备的下一步

优先执行第4章的命令，自行记录一条正常提交、一条超时/取消、一条 Executor 失效恢复和一条 trace。然后用第5章练习 1/3/5 分钟介绍，能指出失败窗口及最终写权限。新增框架的收益低于你亲手复现并能解释现有机制。

## 6. 最新 main CI 的失败与获批准的补齐

[run 38617959365](https://github.com/kaii9/codingJudge/actions/runs/38617959365) 对应 `aac45b89aa6fcd7357da526768f07cb76645c969`。frontend/test/e2e/observability 成功，microservices 首次 POST /submissions 约5010ms后 HTTP500，真实提交/rollout 后续验收没有完成。缺少底层 RPC 错误证据，不能确定是连接、DNS、handler 或存储慢，更不能把预期的数据库权限/回滚测试日志误认作根因。

用户批准在 `fix/judge-rpc-readiness` 补齐：Platform readiness 通过业务客户端检查带鉴权的 Judge Health；RPC 日志只记录方法、code、连接状态、耗时和有效 trace ID。保持 readiness 2s、Judge业务5s、禁配置型重试与既有沙箱限制；见[第8章](08-readiness-rpc-diagnostics.md)。

本轮已通过全量 Go suite、vet、API/Judge/mTLS/HTTP race 和新增两包 race 五次重复；macOS race 链接器产生 LC_DYSYMTAB 警告，测试退出0且没有竞态报告，不把警告描述成业务错误或声称零警告。显式 CI 步骤已加入，但尚未推送触发新 head 的远程 CI。

原数据栈只重建 Platform，不删除数据卷。本地首次 verify 的 Python/Go/C++ 均 Accepted，但排行榜校验失败。只读核对新用户三条 Accepted 投影存在、solved=1、实际排名56；接口固定取前50，因此不是投影丢失。后续使用独立空数据验证栈，不删除原有数据或修改生产接口迎合脚本。

独立 Compose project 为 `gojudge-rpc-readiness-verification`，API/PG/Redis/MinIO/Jaeger 端口分别为38080/35432/46379/39000/36686；数据卷与原栈分开，初始化 SQL 从主工作区绑定。实际通过：

- Python/Go/C++ Accepted、排行榜收敛、单 Executor 停机恢复、Executor 独立重建（Platform/Judge/Worker IDs未变）、历史与远程 source 产物读取、两条 Streams pending 归零。
- 三项真实 PostgreSQL 集成测试：快照与结果Outbox事务、投影重复/迟到、账号不可跨领域访问。
- collector-backed正常提交，trace `91335e6de0bd3fd207577209e5d78a0a`，四服务、8 spans、1 attempt、0 Error spans。独立栈停止后Jaeger内存数据不保留。
- 停止独立栈的 Judge API：Platform `/readyz` 返回503及 `component=judge_rpc`，`/healthz` 仍200；实际日志有 Health/Check、DeadlineExceeded/CONNECTING 和 Unavailable/TRANSIENT_FAILURE、耗时及trace ID，无原始错误或token。重新启动后 `/readyz` 恢复200。连接结束状态没有被当作历史CI500的根因证据。

本轮新增变更仅针对 Platform 客户端和 readiness；此前在途SIGKILL/强制drain有历史与远程证据，但没有把它们说成本轮重新实测。新提交仍须独立远程复验，不能用本地结果宣称当前 main CI 全绿。
