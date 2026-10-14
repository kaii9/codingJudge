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
| fa8677d远程浏览器E2E | PASS | 加强Monaco断言后的独立Linux runner，不替代本机失败记录 |
| Markdown本地链接及两份截图 | PASS | 所有目标存在，截图已查看 |

E2E首先因新Chromium未安装而未启动；安装后发现Python输入拼到默认模板前，实际错误为EOFError。测试已改成等待Monaco实际model与完整selection、核对模型和真实提交字节，保留Python精确文本校验。之后在本机Go编译上限仍出现Compile Error/30s等待失败；没有通过放宽沙箱预算或跳过Go将其变绿。远程独立runner的对应结果须单独看，不能掩盖本地资源敏感性。

MinIO源镜像确认server原release和commit；补齐官方固定源码client后，真实bucket及本次source artifact的`mc alias/stat`通过。完整legacy asset smoke以最新远程CI复验为准，不把单个mc命令等同完整smoke。

### 浏览器QA摘要

流程为注册→打开A+B→选语言/编辑Monaco→Submit→终态→提交历史。验证网址本机127.0.0.1:23010，远程CI使用其独立Compose前端；不宣称两个环境结果完全相同。

| 检查 | 覆盖与结果 |
| --- | --- |
| 页面身份 / 非空 | E2E打开指定问题并断言A+B标题与编辑器；本地失败截图也显示正确页面 |
| Framework overlay | 本地已查看失败截图未见；没有对每个状态统一自动拦截overlay |
| Console health | 未单独收集全部浏览器console，不能据通过测试宣称console零错误 |
| 截图证据 | 两份Jaeger截图已提交并检查；前端截图仅失败时生成，未做全面视觉回归 |
| 交互证明 | 远程fa8677d加强后的三语言提交/AC/历史与响应布局通过；本地Go编译失败保留 |
| 未覆盖 | Firefox/WebKit、所有错误/空态/高并发UI、跨主机网络环境 |

## 可查的submission / trace

正常提交：`sub-b954a741-2363-488b-91c2-c752fb2e6f84`，trace `86e3279e66c9714619dde6608556b037`。

SIGKILL：`sub-f673ae34-fc30-42c8-aa8b-280eb6a4c0ab`，attempts2，AC，trace `5e9f998596d3ffe5660e15ebd52026db`。

强制drain：`sub-bc9beb9f-2cb7-46f4-896c-512ad9425ff8`，attempts2，AC，trace `d25f1660f50166d0b003a43f7a0b0624`。验证器`-min-attempts 2`确认这两个attempt属于同一提交。

截图与解读见 [gRPC/追踪章节](03-grpc-tracing.md)。外部synthetic parent未导出，因此UI显示Incomplete；四服务内部因果树是验证覆盖范围。Jaeger内存数据随重启丢失，ID不是长期远程证据。

## 远程CI

- [main首次run38505707075](https://github.com/kaii9/codingJudge/actions/runs/38505707075)：失败。MinIO旧镜像pull denied、前端audit失败；observability通过。
- [分支run38534383516](https://github.com/kaii9/codingJudge/actions/runs/38534383516)，head0869a81：microservices、e2e、observability通过；frontend因原审计门槛失败；test的Go/race/integration/vet通过，随后asset smoke因缺mc失败。该次E2E通过发生在Monaco测试加强之前，不冒充最新提交的结果。
- [修正代码run38536652533](https://github.com/kaii9/codingJudge/actions/runs/38536652533)，head fa8677d：test（含完整legacy MinIO asset smoke）、microservices（含collector验证和故障恢复）、e2e（加强后的Monaco断言）、observability全部通过；frontend仅因保留的完整audit门槛失败。运行时代码/镜像与测试修复由该head验证，不把文档随后更新的commit冒充这个head。
- [教材提交run38538075541](https://github.com/kaii9/codingJudge/actions/runs/38538075541)，head2c74697亦已推送，触发独立CI；全分支尚未满足审计门槛，没有自动合并main，没有创建漏洞例外。该head没有改变fa8677d后的运行时或测试实现。

## 独立审查与判断边界

一次fresh-context整分支审查覆盖1cf5ded..2c74697，重点Go测试通过，未发现新增Critical/Important实现缺陷，无待处理Minor；结论是不能合并，不是验收豁免。

- 迁移恢复最初字节而不是更改DB checksum；若判断错仍会阻断部署，不通过篡改历史放行。
- 审查者不重跑有副作用的fault/image/UI实验，由本次作者实际验证；本机E2E失败继续记录，风险是环境相关问题尚存。
- 最新远程与advisory由作者核对，不以审查者转述当成新验证；若证据陈旧只影响报告，不能放宽安全门槛。
- 生产MinIO适用性、跨主机HA和TLS轮换不在本次收尾范围；错误外推有安全风险，因此禁止生产就绪表述。
- 外部父span/被SIGKILL的span可能缺失；追踪不能定位所有细节，数据正确性仍靠数据库不变量和故障测试。

## 已修复但应记住的问题

重建迁移镜像发现1531538把4个已应用SQL的EOF空行删除，引起checksum mismatch；296677a恢复原始字节。回归测试先红（4文件）再绿，现有两库migration正常退出0；没有编辑数据库checksum记录，没有删卷。

用户已有未跟踪架构文档未修改，SHA256仍为`5c26ae9ad533b80d608d70ee1bed455160057b519fefc2ce2784158d586a463f`。历史基准、跨主机HA、真实TLS轮换、生产长期trace存储及MinIO维护迁移均不属于本次已验证能力。
