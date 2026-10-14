# 2026-10-15 主线合并与目标收尾

用户批准合并后，`main` 从 `1cf5ded` 快进到 `75cf2af`，没有重新改写判题、迁移、鉴权或依赖实现。下述运行时和测试证据明确对应 `75cf2af`；本报告及关联状态更新属于后续文档改动，其远程结果应查看 main 上对应提交的 CI，不能用本文自行证明自身 CI 已通过。

## 1. 目标是否完成

| 原定目标 | 结果 | 可核对依据 |
| --- | --- | --- |
| 明确服务边界、数据归属与内部协议 | 完成 | Platform/Judge/Executor 三个领域，数据库账号隔离，版本化 gRPC；[第1章](01-architecture-call-chains.md) |
| 超时、取消、健康选择及有界恢复 | 完成 | 真实 RPC、冷编译/取消、重试与幂等测试；[第2章](02-reliability-failure-windows.md)、[第3章](03-grpc-tracing.md) |
| 独立部署和真实失效恢复 | 完成 | 独立 Executor 重建、在途 SIGKILL 与强制 drain，attempt 恢复；[第4章](04-tests-ci-demo.md) |
| 跨服务可观测性 | 完成 | 实际 OTLP exporter、实例/attempt 属性、collector-backed 四服务 trace 与结构验证器 |
| 已有 mTLS 的真实传输验证 | 完成 | TLS1.3 正向、八类拒绝、过期服务器、可信证书配错误 token；默认 Compose 仍不因此变成 mTLS 部署 |
| 完整前端依赖审计阻断 | 已解决 | 移除 braces 传递链，没有豁免、omit-dev 或不兼容降级；[第7章](07-frontend-dependency-security.md) |
| 测试、面试教材和陈述边界 | 完成 | 七章教程、调用链、故障窗口、实验命令、问答与简历；[学习入口](README.md) |

结论：面向当前简历/面试演示的原定开发与验收目标已完成，无需再新增框架或拆出更多服务。生产级扩展并非本轮欠交项，也不可以冒充已经实现。

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
