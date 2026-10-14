# 2026-10-14 收尾验收记录

这是收尾分支 `feat/gojudge-interview-closeout` 的记录，不表示已合并，也不表示全项绿灯。基础提交 `1cf5ded`；运行时代码新增实例/attempt tracing、可选Jaeger与固定来源MinIO兼容镜像，另修复已应用迁移的空白checksum回归。最终状态以最新远程CI和下方更新为准。

## 本地证据

环境：macOS arm64，Go按go.mod1.25，Node25.8.2/npm11.14.1，Docker Desktop。微服务API28080；升级后的生产构建前端使用127.0.0.1:23010，Jaeger127.0.0.1:26686。桌面1440×960与Pixel7；Browser plugin未启用，使用已有Playwright。没有修改React界面。

| 检查 | 结果 | 说明 |
| --- | --- | --- |
| `go test ./... -count=1` | PASS | 包含迁移checksum和trace验证器回归 |
| 8个相关包race | PASS | Executor/JudgeRPC/Platform/Store/Worker/Telemetry/TLS/trace-report |
| `go vet ./...` | PASS | 无诊断 |
| 3个专用PostgreSQL集成测试 | PASS | 事务、投影、跨域账号权限 |
| `TEST_DOCKER_RUNNER=1 ... ./internal/judge` | PASS | 真实Docker集成 |
| frontend lint / typecheck / unit / build | PASS | 13个测试文件、183个单元测试 |
| 完整npm审计 | FAIL | 5项high来自未修复braces传递链，无豁免 |
| production-only审计（诊断） | PASS | 0；不替代完整审计门槛 |
| 三语言smoke / 独立重建 / artifact / stream drain | PASS | 第一次并行Docker测试时Go冷编译超时，串行复验通过 |
| 实际在途SIGKILL / 100ms强制drain | PASS | 两者均attempt2最终AC |
| collector-backed正常trace | PASS | 同trace四服务、8个span、1个attempt |
| collector-backed强制drain trace | PASS | 四服务、11个span、2个attempt、两个Executor实例 |
| 本地浏览器E2E | 未全通过 | 布局2项通过，2项互斥skip；Go冷编译30s上限仍有失败，不能报作5/5通过 |
| Markdown本地链接及两份截图 | PASS | 所有目标存在，截图已查看 |

E2E首先因新Chromium未安装而未启动；安装后发现Python输入拼到默认模板前，实际错误为EOFError。测试已改成等待Monaco实际model与完整selection、核对模型和真实提交字节，保留Python精确文本校验。之后在本机Go编译上限仍出现Compile Error/30s等待失败；没有通过放宽沙箱预算或跳过Go将其变绿。远程独立runner的对应结果须单独看，不能掩盖本地资源敏感性。

MinIO源镜像确认server原release和commit；补齐官方固定源码client后，真实bucket及本次source artifact的`mc alias/stat`通过。完整legacy asset smoke以最新远程CI复验为准，不把单个mc命令等同完整smoke。

## 可查的submission / trace

正常提交：`sub-b954a741-2363-488b-91c2-c752fb2e6f84`，trace `86e3279e66c9714619dde6608556b037`。

SIGKILL：`sub-f673ae34-fc30-42c8-aa8b-280eb6a4c0ab`，attempts2，AC，trace `5e9f998596d3ffe5660e15ebd52026db`。

强制drain：`sub-bc9beb9f-2cb7-46f4-896c-512ad9425ff8`，attempts2，AC，trace `d25f1660f50166d0b003a43f7a0b0624`。验证器`-min-attempts 2`确认这两个attempt属于同一提交。

截图与解读见 [gRPC/追踪章节](03-grpc-tracing.md)。外部synthetic parent未导出，因此UI显示Incomplete；四服务内部因果树是验证覆盖范围。Jaeger内存数据随重启丢失，ID不是长期远程证据。

## 远程CI

- [main首次run38505707075](https://github.com/kaii9/codingJudge/actions/runs/38505707075)：失败。MinIO旧镜像pull denied、前端audit失败；observability通过。
- [分支run38534383516](https://github.com/kaii9/codingJudge/actions/runs/38534383516)，head0869a81：microservices、e2e、observability通过；frontend因原审计门槛失败；test的Go/race/integration/vet通过，随后asset smoke因缺mc失败。该次E2E通过发生在Monaco测试加强之前，不冒充最新提交的结果。
- MC和Monaco修正后的新提交还需最新CI复验；没有自动合并main，没有创建漏洞例外。

## 已修复但应记住的问题

重建迁移镜像发现1531538把4个已应用SQL的EOF空行删除，引起checksum mismatch；296677a恢复原始字节。回归测试先红（4文件）再绿，现有两库migration正常退出0；没有编辑数据库checksum记录，没有删卷。

用户已有未跟踪架构文档未修改，SHA256仍为`5c26ae9ad533b80d608d70ee1bed455160057b519fefc2ce2784158d586a463f`。历史基准、跨主机HA、真实TLS轮换、生产长期trace存储及MinIO维护迁移均不属于本次已验证能力。
