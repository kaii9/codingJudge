# GoJudge Interview Closeout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 提供有真实证据的微服务演示与面试教材，并如实处理远程 CI 阻断。

**Architecture:** 保持 Platform/Judge/Executor 三个领域边界；仅增加可选观测 overlay 和无业务写入的 trace 验证器。依赖修复不改变审计策略。

**Tech Stack:** Go testing、OTLP HTTP、Jaeger 2.22.0、Docker Compose、npm audit、Playwright。

**Spec:** `docs/superpowers/specs/2026-10-14-interview-closeout-design.md`

**当前状态（2026-10-15）：四个 Task 均完成。** 完整审计阻断通过 lint 工具链迁移解决；`75cf2af` 的远程五个 job 全 PASS，用户批准后快进合并到 main，合并后的主工作区重装/审计/测试/构建及 Go suite/vet 通过。[最终目标与合并验收](../../interview/microservices/verification-2026-10-15.md)区分当前结果、历史失败与生产边界。下方步骤是原始执行过程；Expected RED 指当时验证新增测试，而不是要求现在把正确实现重新改坏。

**执行状态（2026-10-14更新）：** Task 2/3 已实现并验证；Task 4 教材、源码链接和原分支独立审查完成，实际结果见 `docs/interview/microservices/verification-2026-10-14.md`。Task 1 可修复依赖与源码镜像（含 mc）已修复并获远程验证，但完整 npm audit 仍 FAIL，因此总体安全验收未完成、不自动合并。继续验收已修复编译并行度、浏览器平台/焦点、Linux目录身份与有界源码下载重试，本机重复E2E10 PASS/4设备互斥SKIP；head276b892远程后端/微服务/E2E/观测通过。历史本机失败仍保留；新修复由[剩余验收计划](2026-10-14-remaining-acceptance.md)与最新验收记录追踪，不把旧独立审查范围扩展到新提交。

## Global Constraints

- 维持 `npm audit --audit-level=high`，不接受 braces 审计例外。
- 不删除数据卷、不修改用户未跟踪文档、不更改全局 Docker 网络配置。
- 追踪不可写入源码、测试数据、密码或 token。
- CI 未通过前不宣称全部完成或自动合并收尾分支。

## Review Focus

- Jaeger 未就绪时演示应有界失败，不能无限等待。
- 不相关 trace、断开的父子链和缺失服务不能被验证器算作成功。
- SIGKILL 丢失 Executor span 应诚实说明，不能强制伪造完整 trace。
- overlay 重建 Executor 时应保留 collector 设置，基础部署仍不依赖 Jaeger。
- 未修复开发依赖仍阻断 CI；运行时升级不得回退 API/lint/E2E能力。

### Task 1: CI dependency reproducibility

**Files:** `frontend/package{,-lock}.json`, `deploy/Dockerfile.minio`, `docker-compose.yml`, `deploy/compose/microservices.yml`, `.github/workflows/ci.yml`, `Makefile`.

**Interfaces:** Produces 固定来源 MinIO 演示镜像、更新锁文件；不改变业务端口/数据卷/审计命令。

- [x] 保存远程失败证据：`gh run view 38505707075`，Expected: MinIO pull denied / npm audit failure。
- [x] 升级 Next 与配套 ESLint 到 16.3.8、DOMPurify 到 3.4.17，更新可修复传递依赖；`npm install`，Expected: 锁文件一致。
- [x] `npm audit --audit-level=high`，Expected: 可修复项消失；如剩未修复项，标记该任务安全验收未完成。
- [x] 官方固定版本构建 MinIO（原 RELEASE.2025-04-22T22-12-26Z），`docker build -f deploy/Dockerfile.minio -t gojudge-minio:2025-04-22 .`，Expected: 不依赖旧镜像仓库，`--version` 可运行。
- [x] `npm run lint && npm run typecheck && npm run test:run && npm run build`，Expected: PASS，不因 audit 阻断而跳过这些单独核验。
- [x] 提交依赖与可复现构建改动。

### Task 2: Observable attempts and genuine exporter

**Files:** `internal/telemetry/telemetry{,_test}.go`, `internal/judgeworker/processor.go`, `internal/judgeworker/telemetry_test.go`.

**Interfaces:** Produces `service.instance.id` resource、`gojudge.execution_id` / `gojudge.attempt` span attributes，保持业务返回值。

- [x] 写 exporter 测试：httptest 接收 `/v1/traces` protobuf，断言 `EXECUTOR_INSTANCE_ID` 和 hostname fallback。写 Worker 测试：失败 attempt 保留错误状态及执行 ID。
- [x] `go test ./internal/telemetry ./internal/judgeworker -run 'TestInit|TestAttemptSpan' -count=1`，Expected: FAIL 因缺失 instance/attempt attributes。
- [x] 最小实现 instance resource 与 attempt/error标记。
- [x] 同命令，Expected: PASS；`go test -race ./internal/telemetry ./internal/judgeworker -count=1`，Expected: PASS。
- [x] 提交。

### Task 3: Opt-in trace demo

**Files:** `deploy/compose/microservices-tracing.yml`, `scripts/trace-report/{main,main_test}.go`, `scripts/microservices_tracing_test.go`, `scripts/verify-microservices-tracing.sh`, 现有 smoke/fault 脚本和 `Makefile`。

**Interfaces:** Consumes Task 2 attributes；Produces `go run ./scripts/trace-report -trace-id HEX -min-attempts N`，stdin 为 Jaeger v3 JSON envelope；环境 `CJ_MICRO_TRACING=1` 在所有生命周期脚本启用相同 overlay。

- [x] 验证器测试覆盖：真实 OTLP JSON字段结构，四服务连通；缺服务/混入其他trace/断parent/无attempt/两次attempt/错误envelope/NDJSON。
- [x] Compose 测试通过 `docker compose ... config --format json` 验证合并后变量、loopback UI、collector不出公网；不能用源码 grep 代替行为测试。
- [x] `go test ./scripts/... -count=1`，Expected: RED；实现最小解析和 overlay 后 Expected: GREEN。
- [x] 演示脚本 curl 连接/总时限分别3s/10s；结果轮询120次、trace轮询60次；只在成功连接四服务后打印成功。故障脚本仅打印关联trace ID。
- [x] `make microservices-tracing-up` 和 `make verify-microservices-tracing`，Expected: 真实 Accepted + 四服务 span树；`CJ_MICRO_TRACING=1 make verify-microservices`、故障脚本，Expected: 独立重建和 attempt恢复。
- [x] 提交。

### Task 4: Interview curriculum and final evidence

**Files:** `docs/interview/microservices/*.md`, README 与旧简历/问答文档的模式标记，验收报告。

**Interfaces:** Consumes Task 1–3真实结果；Produces 从入口→源码→测试→演示→问答的学习路线。

- [x] 写架构/所有核心调用链、可靠性故障窗口、RPC/追踪、分层测试/红绿重构、问答/简历边界；历史压测仅标为历史拓扑结果。
- [x] 检查 Markdown相对链接与命令/测试名存在；不写虚构能力与验证结果。
- [x] `go test ./... -count=1`、`go vet ./...`、选定 race/数据库集成/Playwright，Expected: 分别记录真实结果。
- [x] 提交，按 executing-plans 要求做一次 whole-branch fresh-context review；重要问题一次修复并验证。
- [x] 推送收尾分支，核对远程 CI。安全门槛未通过则不自动合并，最终说明未完成项与文档入口。
