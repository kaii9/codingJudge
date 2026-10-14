# GoJudge 文档入口与版本说明

整理日期：2026-10-15。准备当前微服务版本的简历和面试，请从本页进入，不混用旧 HTTP 链路与新 gRPC 链路。

## 1. 当前学习主线：微服务模式

| 目标 | 首选资料 |
| --- | --- |
| 按顺序系统学习 | [八章学习路线](interview/microservices/README.md) |
| 架构与各功能调用链 | [第1章](interview/microservices/01-architecture-call-chains.md) |
| Outbox、Streams、租约和CAS token | [第2章](interview/microservices/02-reliability-failure-windows.md) |
| gRPC、取消、健康选择、mTLS、追踪 | [第3章](interview/microservices/03-grpc-tracing.md) |
| 测试、E2E、红绿重构和故障演示 | [第4章](interview/microservices/04-tests-ci-demo.md) |
| 简历描述、1/3/5分钟介绍、追问 | [第5章](interview/microservices/05-interview-resume.md) |
| 冷编译、Linux权限、浏览器排障 | [第6章](interview/microservices/06-debugging-compiler-e2e.md) |
| braces依赖链与审计门槛 | [第7章](interview/microservices/07-frontend-dependency-security.md) |
| readiness、共享预算与脱敏诊断 | [第8章](interview/microservices/08-readiness-rpc-diagnostics.md) |
| 运行命令与配置 | [微服务部署指南](microservices-guide.md) |
| 已验证版本、失败及限制 | [2026-10-15验收记录](interview/microservices/verification-2026-10-15.md) |

新模式使用 `deploy/compose/microservices.yml`：Platform/Judge数据归属分开，内部Platform→Judge及Worker→Executor为gRPC，排行榜使用完成事件投影。公开浏览器API仍为HTTP JSON；“内部旧HTTP模式”不等于“浏览器不能再使用HTTP”。

学习时先画架构，再跟一条提交到最终结果的链路，最后亲自跑第4章和第8章实验。简历只采用自己能解释并指向源码/测试的能力，不把辅助完成的部分说成独立从零设计。

## 2. 旧 HTTP 模式：保留用于对照

旧模式仍有部署路径，不删除文档，也不移动文件破坏既有链接。但以下资料不是当前微服务简历的首选入口：

| 旧资料 | 保留用途 | 不应直接套用的部分 |
| --- | --- | --- |
| [旧简历项目说明](codingjudge-resume-project.md) | 查看早期组织方式与历史实验 | 内部HTTP、单库链路、阶段性性能陈述 |
| [旧面试问答手册](codingjudge-interview-handbook.md) | 补充基础术语与旧模式问答 | 新数据归属、gRPC、完成事件投影、当前安全参数 |
| [旧完整教学](codingjudge-complete-tutorial.md) | 对照单库提交Outbox与Worker机制 | 新模式两个Outbox、分库、gRPC、当前沙箱权限 |

旧文中的“当前”只在旧模式/当时写作范围内理解；文件中源码链接指向现有仓库，不是冻结的历史checkout。核对历史实现时使用相应提交，核对今天的实现时以当前源码和新版教材为准。旧 `cap-drop ALL` 描述不代表当前可信运行包装器不需要SETUID/SETGID；旧模式性能也不代表新拓扑性能。

## 3. 历史证据：保留原始结果

| 类别 | 入口 | 阅读规则 |
| --- | --- | --- |
| 初次微服务本地验收 | [2026-10-14原始报告](microservices-verification-2026-10-14.md) | “尚未推送”、无collector等只描述当时阶段 |
| 收尾失败与修复过程 | [2026-10-14收尾记录](interview/microservices/verification-2026-10-14.md) | 旧audit FAIL、未合并不代表当前状态；原失败不改成PASS |
| 固定负载实验 | [Worker scaling](benchmarks/2026-07-03-worker-scaling.md) | 文件名日期不是唯一依据，以报告内Date/commit/环境/数据为准 |
| 饱和与积压排空 | [Saturation](benchmarks/2026-09-08-saturation-scaling.md) | 仅代表指定负载与拓扑，不能冒称当前生产容量 |
| Executor拓扑对照 | [Topology](benchmarks/2026-09-09-executor-topology.md) | 独立daemon不等于独立宿主，不能归因于后来的gRPC改造 |
| 初期需求与阶段计划 | [PRD](prd.md)、[原开发计划](plan.md)、[阶段计划](development-plan.md) | 设计意图与当时状态，不是当前欠交清单 |
| 分阶段设计与实施记录 | [设计档案](superpowers/specs/)、[实施档案](superpowers/plans/) | 按文件日期和提交阅读；后续验收优先看版本化报告 |

历史表格中的成功、失败、跳过、环境、时间和提交号原样保留。不用旧报告中的绿灯证明新head，也不把旧红灯继续描述成当前阻断。教材引用数字时必须与原报告相符，不能把轮询HTTP rate当提交QPS。

## 4. 可跨版本阅读的补充资料

- [PostgreSQL与MySQL对照](postgresql-vs-mysql-for-mysql-users.md)：理解数据库选型与事务机制；服务数据归属仍以新版第1/2章为准。
- [题目输入格式](hot20-input-formats.md)：写测试程序、复现判题时使用。
- [OpenAPI](openapi.yaml)：公开HTTP接口契约；内部RPC协议另见`api/proto/gojudge/`。

源码、文档和验证是三种证据：文档一致性检查不等于重新跑了全部业务实验，CI绿灯也不等于生产多租户安全、跨主机HA、证书轮换或exactly-once。
