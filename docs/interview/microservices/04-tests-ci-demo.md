# 4. 测试、CI、红绿重构与十分钟演示

## 4.1 测试分层：它们证明的东西不同

| 层级 | 本项目例子 | 能证明 | 不能证明 |
| --- | --- | --- | --- |
| 单元 | Processor 使用小接口 fake，投影重复/迟到事件 | 分支、调用顺序、预算和状态机 | 真实 SQL 锁、网络、Docker 权限 |
| 集成 | PostgreSQL 事务、Redis PEL、MinIO Get/Put | 真实组件语义与权限 | 浏览器整体体验、生产 HA |
| RPC/契约 | loopback gRPC、冻结旧客户端 | 协议、错误、deadline、取消、兼容 fixture | 公网随机丢包、所有版本兼容 |
| 系统 smoke / fault | Compose + 真实三语言 + Executor SIGKILL | 部署连通、真实恢复、最终收敛 | 跨主机灾难恢复、峰值吞吐 |
| 浏览器 E2E | 注册→Monaco→提交→AC→历史 | 前后端整条可见链路 | 所有语言错误分支、安全无漏洞 |
| 性能 | k6 固定到达率、拓扑对照 | 给定环境负载下的延迟变化 | 生产 QPS、线性扩容保证 |

fake 不是天然低质量：例如预算测试里的 fake 主动拒绝 expired Context，能发现“执行超时后拿同一个过期 Context 做收尾”的真实设计错误。反之，一个永远返回成功的 fake 会把缺陷掩盖掉。

## 4.2 RED → GREEN → REFACTOR

红（RED）：先写会失败的行为测试，运行它，确认因目标能力缺失失败，而不是 import/拼写错误。绿（GREEN）：做满足测试的最小实现。重构（REFACTOR）：不改变行为，整理结构，再跑测试。

本次追踪例子：先让 httptest 接收真正 `/v1/traces` protobuf，断言 resource 包含实例 ID；旧实现确实导出了 span，但没有实例字段，测试失败。再增加资源字段后通过。不是用源码 grep 判断“包含 instance 字样”。

Worker attempt 例子：失败 attempt 必须有 execution ID、attempt 和 Error 状态；旧实现没有，先失败，再实现。验证器例子：四服务但父子关系断开也必须失败；不能用“四个名字都出现”冒充完整调用链。

测试断言应围绕结果与不变量：旧 token 更新行数为 0、最终版本不回退、重放返回原 submission、响应丢失后一次 RPC 不自动再次调用 Runner。不要绑定无关私有实现或只断言函数被调用。

## 4.3 最低成本的本地验证

以下在仓库根运行，Go 版本按 go.mod（1.25），Node 按 frontend engines。

```bash
go test ./... -count=1
go test -race ./internal/executor ./internal/judgerpc ./internal/platform \
  ./internal/store ./internal/judgeworker ./internal/telemetry ./internal/rpcsecurity
go vet ./...
npm --prefix frontend ci
npm --prefix frontend audit --audit-level=high
npm --prefix frontend run lint
npm --prefix frontend run typecheck
npm --prefix frontend run test:run
npm --prefix frontend run build
```

`-count=1` 避免测试缓存冒充新验证；race 查数据竞争，不证明没有所有并发业务错误；vet 是静态检查，不替代运行测试。审计失败时先修复，不能用 `|| true` 把安全门槛变绿；需要诊断其他检查时可以单独运行并分别报告。

## 4.4 数据库、Docker 和 RPC 的针对性测试

先启动微服务栈，再运行专用测试：

```bash
make judge-images
make microservices-up
TEST_JUDGE_DATABASE_URL='postgres://cj_judge:cj-judge-local-only@127.0.0.1:25432/cj_judge?sslmode=disable' \
TEST_PLATFORM_DATABASE_URL='postgres://cj_platform:cj-platform-local-only@127.0.0.1:25432/cj_platform?sslmode=disable' \
go test -tags integration ./internal/store -run TestMicroservice -count=1 -v
TEST_DOCKER_RUNNER=1 TEST_GO_COMPILER=1 \
go test -tags integration ./internal/judge -count=1 -v
go test ./internal/executor -run 'TestGRPC|TestFrozen|TestDrain|TestForced' -count=1 -v
go test ./internal/rpcsecurity -run TestMTLS -count=10 -v
```

专用 `TestMicroservice*` 用随机 schema 验证并清理自己的 schema，仍建议用演示库。旧 `TEST_DATABASE_URL` 集成套件有初始化/清理行为，绝不能指向重要库。缺环境变量而 SKIP 是“未覆盖”，不是数据库集成通过。

新增Go冷编译测试需要`TEST_GO_COMPILER=1`，覆盖真实双用例、语法错误与确认运行中后取消/无沙箱泄漏；CI的microservices job也显式启用。参数单测与fake不能替代这条真实Linux权限路径。源码与实测见[第6章](06-debugging-compiler-e2e.md)。

mTLS测试默认运行，覆盖真实握手、证书/协议拒绝与两个真实业务RPC适配器的token授权。它只使用测试临时证书和loopback listener，无需Docker或现有业务库；不是fake TLS，也不是证书轮换测试。CI有单独的`verify mutual TLS contracts and token authorization` race步骤，解释见[第3章](03-grpc-tracing.md)。

必须认识三个测试名：[快照+结果事务](../../../internal/store/microservices_integration_test.go) 的 `TestMicroserviceSnapshotAndResultOutboxAtomic`，投影 `TestMicroserviceProjectionDuplicateAndLateEvent`，权限 `TestMicroserviceDatabaseCredentialsCannotCrossDomains`。

## 4.5 十分钟演示流程

准备（首次冷构建不算十分钟）：拉取判题镜像、编译服务和 MinIO、安装前端/浏览器依赖，确认端口无冲突。新 MinIO 演示镜像从官方固定源码构建，冷编译可能较慢。

1. 画架构（1min）：平台不挂 Docker Socket，Judge 拥有提交，Worker 不读平台库。
2. 正常提交（2min）：`make verify-microservices`；观察 Go/C++/Python AC、排行一题三次 AC、artifact 下载和 pending 归零。
3. 独立部署（1min）：上述脚本停止 A 后 B 继续工作、单独重建 A，断言其他服务容器 ID 不变。强调同机共享 daemon。
4. 故障恢复（3min）：`bash scripts/verify-microservices-faults.sh`；它在实际沙箱活跃时 SIGKILL，随后再演示 100ms 强制 drain；不是先停机再创建任务。最终必须 AC 且数据库 attempts≥2。
5. 追踪（2min）：`make microservices-tracing-up` 后 `make verify-microservices-tracing`，打开输出链接看四服务；启用 tracing 跑故障脚本时加 `CJ_MICRO_TRACING=1`，避免重建 Executor 丢失 exporter 配置。
6. 讲边界（1min）：消息/执行可重复、最终写入受 token 约束；本地进程恢复不代表物理 HA。

脚本创建演示用户和提交，故障脚本会短暂停止本项目 Executor，因此不要在重要在线环境运行。trap 会尽量恢复 Executor 默认配置；若机器/脚本强制退出，手动 `make microservices-up` 恢复。没有任何步骤需要 `down -v`。

## 4.6 浏览器 E2E

```bash
docker compose -f deploy/compose/microservices.yml --profile frontend up -d --build frontend
PLAYWRIGHT_BASE_URL=http://localhost:23000 npm --prefix frontend run test:e2e
```

流程：[judge-flow](../../../frontend/e2e/judge-flow.spec.ts) 为每种语言注册用户，打开 A+B，操作真正 Monaco 编辑器和语言选择器，提交等待 Accepted，然后查历史。不是直接调用 API 代替点击界面。[responsive](../../../frontend/e2e/responsive.spec.ts) 检查桌面布局和移动端标签页/溢出。

配置有 Chromium Desktop 1440×960 与 Pixel 7；两项按项目类型跳过是互斥布局断言（desktop-only/mobile-only），应明确报告，不能说 7 项全通过。截图只在失败时保存，trace 在首次重试保存；Playwright trace 用来重放浏览器操作，不是 OpenTelemetry 分布式 trace。

Monaco 的 textarea 是虚拟输入缓冲区，“选中了 textarea 全文”不等于选中了整个编辑模型。测试先等待实际语言 model、检查模型 selection覆盖全文，再用键盘输入；只读取公开 Monaco API核验，不通过它改源码。Go/C++允许输入过程产生不影响这些固定程序语义的缩进变化，Python检查精确文本；点击 Submit 后再检查真实浏览器请求字节等于编辑模型，避免把默认模板拼接问题误诊为后端错误。终态错误会立即失败，不盲等 Accepted。升级Playwright后还要安装匹配Chromium。

继续验收又补齐了模拟UA平台的快捷键、可见画布点击/真实焦点断言，并将正常判题的功能测试等待与120s RPC预算对齐；整个flow150s有界，生产编译仍30s。实际复现和原因见 [第6章](06-debugging-compiler-e2e.md)，不是通过放宽判题限制隐藏失败。

本次未启用 Browser plugin，按前端测试技能使用仓库已有 Playwright。前端依赖升级的实际运行记录见验收报告；构建成功本身不足以证明浏览器可用。不宣称已测试 WebKit/Firefox 或所有 UI 状态。

## 4.7 CI 是门禁，不是云部署

[工作流](../../../.github/workflows/ci.yml) 由 push/pull_request 触发，五个 job 分别验证后端、微服务、前端、观测配置、浏览器 E2E。每个 runner 是隔离临时环境，不依赖作者电脑缓存。当前没有自动发布到生产主机的 CD 流程，不能把 Compose 在 CI 启动称为上线。

本次 main 首次远程 [run 38505707075](https://github.com/kaii9/codingJudge/actions/runs/38505707075) 暴露两类问题：原 MinIO 镜像新 runner 无法拉取；npm audit 发现新披露漏洞。验证入口有效，但结果是红灯，不能拿观测配置单个 job 成功替代全流程通过。

处理方式：从原官方 MinIO 源码 commit `0d7408fc9969caf07de6a8c3a84f9fbb10a6739e` 重建演示镜像，保留版本/卷；升级可修复前端依赖，不执行 `npm audit fix --force` 强制降级 Next ESLint。2026-10-14 剩余 braces≤3.0.3 未有修复版本，5 项高危报告来自其传递链；用户决定不放行，原 `--audit-level=high` 保留。[braces 官方公告](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm)

两次源码构建还遇到校验服务器瞬时HTTP/2错误；辅助脚本仅对已列举传输错误最多重试3次，checksum mismatch/SECURITY ERROR仍立即失败。这不会豁免漏洞，也不是业务执行RPC的重放策略，具体测试与边界见[第6章](06-debugging-compiler-e2e.md)。

MinIO 社区仓库当前不再维护并采用源码分发；本项目兼容镜像是保持教学演示可复现，不是安全升级，也不能据此宣称对象存储适合公网生产。生产需要单独评估维护中的 S3 服务及迁移、备份、恢复方案。[MinIO 官方说明](https://github.com/minio/minio)

## 4.8 排查顺序与通过标准

先确认版本、容器状态和配置；再看 HTTP / RPC 错误码、数据库权威状态、Outbox未发布、group pending、Worker lease/attempt、Executor日志和 trace。失败时定位第一条错误，不反复提高超时或删除卷“重置一切”。

真实案例：重建 migration 镜像后出现 `migration 001 checksum changed after application`，原因是已应用 SQL 的末尾空行被格式化删除。修复恢复最初字节并加入 checksum 回归测试，不改数据库校验记录、不删卷。未来 schema 变化要追加新 migration，不能修改旧文件，即使只有空白差异。

一次有效结论应写清：commit、环境、命令、退出码、成功/失败/跳过数量、关键 submission/trace ID，以及未测范围。重复执行看是否可复现。最终 AC、正确投影和 pending 收敛都满足，才说故障恢复演示通过。
