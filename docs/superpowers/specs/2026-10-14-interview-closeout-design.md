# GoJudge 收尾与面试教学设计

## 目标与已批准范围

完成已有微服务模式的可复现演示、远程 CI 核对和面试教学。延续用户已批准的收尾方案，不新拆服务，不增加业务功能。2026-10-14 用户明确要求维持 `npm audit --audit-level=high`，不接受 braces 审计例外。

## 设计

1. 修复 CI 对不可拉取 MinIO 镜像的依赖；保持原 MinIO 版本与数据卷，不通过存储升级改变现有数据。必要时从官方固定源码构建演示镜像，并说明社区版本维护和生产使用限制。
2. 升级可修复前端依赖。开发依赖同样参与原审计门槛；剩余未修复漏洞要明确阻断验收，不使用白名单或强制降级。
3. 使用可选 Compose overlay 接入 Jaeger 2.22.0。仅 UI 暴露在 loopback；OTLP collector 只走 Compose 内网。未启用 overlay 时业务部署不依赖 Jaeger。追踪不可写入源码、测试数据、密码或 token。
4. 对真实 OTLP 导出、实例资源标识及 Worker attempt 标识增加测试。演示从 HTTP 提交到 Judge RPC、Worker、Executor 的同一 trace，使用 Jaeger 稳定 v3 查询协议验证父子关系。故障演示标识 trace，但不要求 SIGKILL 进程把未结束 span 导出。
5. 文档按架构链路、可靠性、gRPC/追踪、测试演示、面试问答拆分。所有“已验证”来自实际命令输出；明确本地双 Executor 共享 Docker daemon，不是跨主机 HA；随机 token 是数据库 CAS 所有权版本，不是单调存储 fencing。

## 验收与边界

- Go 单测、race、vet 和真实微服务 smoke/fault；前端 audit/lint/typecheck/unit/build、浏览器 E2E分别报告，不把跳过视为通过。
- 追踪以 collector 数据验证，不以存在 traceparent 字符串替代验证。
- 不删除数据卷、不修改用户未跟踪文档、不更改全局 Docker 网络配置。
- CI 未通过前不宣称全部完成或自动合并收尾分支；推送收尾分支用于远程复验。
- 文档和 Jaeger 为教学演示，不宣称公网生产部署、TLS 轮换、跨主机容灾或生产吞吐。
