# 2026-10-14 收尾验收记录

这是收尾分支 `feat/gojudge-interview-closeout` 的记录，不表示已合并，也不表示全项绿灯。基础提交 `1cf5ded`；新增实例/attempt tracing、可选Jaeger、固定来源MinIO兼容镜像，修复已应用迁移的空白checksum、编译预算/身份和浏览器输入问题。最终状态以本节更新及对应远程CI版本为准，后文首轮失败记录不删除。

## mTLS追加验证（2026-10-14）

用户要求继续开发后，核对发现已实现TLS选项只有配置单测，缺少真实握手证据。本次以d75f825为基础，补齐[既有能力验证计划](../../superpowers/plans/2026-10-14-mtls-verification.md)，不新增服务或生产业务，不改运行时TLS/授权实现。

- 3组`TestMTLS*`真实loopback TCP/gRPC测试连续10轮通过，无SKIP；覆盖8种客户端/协议拒绝、过期服务器证书与正向对照、Judge/Executor两个真实业务RPC适配器。
- rpcsecurity/executor/judgerpc三包race、完整`go test ./... -count=1`及`go vet ./...`通过。macOS race linker仍有既有LC_DYSYMTAB警告，不是测试失败。
- 正向调用观察到TLS1.3、已验证客户端链和fixture身份；可信证书配错误token时，Executor不可重试且不增加Runner次数，Judge不增加提交。
- CA、私钥和叶证书由标准库临时生成，私钥0600，listener仅127.0.0.1随机端口；未修改全局信任库、现有Compose、数据卷或用户文档。TLS观察拦截器不替生产代码执行拒绝策略。
- 原实现首次运行即通过；本次补充已有能力的验证，没有伪造RED或声称修复了不存在的运行时漏洞。CI新增显式`verify mutual TLS contracts and token authorization` race步骤，默认Go suite也执行，不需手动启用环境变量。

业务测试使用MemoryStore和计数Runner，不在这组测试中运行Docker/真实PostgreSQL；真实的是TLS/gRPC及生产RPC适配器。不能由此宣称默认Compose已部署mTLS、生产证书ACL/轮换/吊销或跨主机PKI验收完成。教学见[3.6.1](03-grpc-tracing.md#361-真实mtls验证tls通过不等于业务授权通过)，完整远程验收按相应新提交的CI判断，不用下方旧head冒充新测试已获远程验证。

`braces`再次核对仍为3.0.3、上游未有patched版本；完整审计门槛保持，因此收尾分支仍不自动合并。新增mTLS测试不能消除或豁免该独立依赖漏洞。[官方公告](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm)

## 继续验收更新（2026-10-14晚间）

实现与原因见[第6章](06-debugging-compiler-e2e.md)：b4b71c6限制Go编译并行度并新增真实冷编译/取消测试；fdfe68c按模拟浏览器UA选择快捷键；5052c14点击可见Monaco画布并检查焦点；65e4138使编译身份匹配普通Linux调用者拥有的目录。本轮另新增保持checksum校验的MinIO源码下载重试。

| 检查 | 本轮结果 | 证据与范围 |
| --- | --- | --- |
| Go全量 / vet | PASS | `go test ./... -count=1`、`go vet ./...` |
| judge / worker / executor / scripts race | PASS | 4个包；macOS linker的LC_DYSYMTAB警告非测试失败 |
| 真实Go冷编译 / CE / 在途取消 | PASS | 本机通过；Linux CI head65e4138分别约8.10s/3.89s/0.23s |
| 整套Docker Runner集成 | PASS | head276b892显式同时启用两个TEST标志，27.256s，无SKIP；本机Go冷编译10.17s/CE4.72s/取消0.88s |
| 专用PostgreSQL集成 | PASS | head276b892新跑3项事务/投影/账号隔离测试，0.815s |
| frontend lint / typecheck / unit | PASS | head276b892新跑13文件183测试；应用界面未修改 |
| 本机重复浏览器E2E | PASS | 新镜像部署后52.1s，10通过、4设备互斥跳过；单worker重复两轮，不依赖失败重试 |
| MinIO源码镜像重构建 | PASS | 原release/commit未变，Go1.25.11；mc来自固定源码，version为DEVELOPMENT.GOGET，不冒充官方发布二进制 |
| 下载重试单测 | PASS | 一次瞬时失败成功/3次失败终止/永久及checksum错误立即失败/正常成功；参数、校验环境与退出码保持 |
| 新镜像正常三语言/独立部署/artifact/streams | PASS | Executor及MinIO重新部署后Go/C++/Python AC，投影收敛，artifact下载成功，两流pending0 |
| 新部署真实SIGKILL / 100ms drain | PASS | 两个在途故障均attempt2最终AC，恢复后的Executor健康 |
| collector-backed trace | PASS | 正常四服务8span；强制drain四服务11span、同提交2attempt、2Executor实例 |
| 完整npm audit | FAIL | 5个high报告来自braces链，保持原门槛，无例外 |

下载重试fake先因为原实现仅一次尝试失败，增加有界传输重试后通过。随后真实源码镜像构建成功，重建MinIO保留原数据卷；真实bucket `mc alias/stat`与微服务artifact下载通过。上限仅为3次尝试/两次2s间隔，不宣称为每次Go下载新增端到端deadline。源码commit或校验错误不能绕过；兼容镜像仍不是生产安全升级。

单核冷构建13.22s→9.85s只是一次对照，不能写成通用吞吐提升。30s编译上限、CPU1/512MiB/PID64/64MiB tmpfs与网络/权限限制均未放宽；E2E功能等待调整为120s/flow150s不改变用户程序限时，也不充当SLO。目录身份修复已验证普通Linux与Mac Docker Desktop；rootless/userns/远程daemon和真实Windows未验证。

手工浏览器QA使用独立CLI session：注册→A+B默认Go→Accepted→历史，对应`sub-b6d06967-7963-4294-bf61-d823dbc03fb9`。已查看AC页与历史截图，页面身份正确且这两个状态未见framework overlay。认证后导航console为0 error/0 warning；认证前两次401是匿名检查，不隐去。未做全状态console/视觉回归，CLI session已关闭；截图保留在本机忽略目录`output/playwright/remaining-acceptance/`，不是长期远程CI artifact。

最新本地可查记录：

- 正常：`sub-45e64bfa-8e5e-47d6-99ad-35a862705492`，trace `87dce5d09838e69433941d626b8be37a`，四服务8span。
- SIGKILL：`sub-9db01fc8-d14b-4a15-a897-5f76bfc4ff4c`，trace `d7f0d0a1c6ca4c160e3d49ca8f2d4748`，attempt2 AC；不保证被kill实例span完整flush。
- 强制drain：`sub-1d593b6e-9348-429a-b91d-aae60cc6caae`，trace `2b75efbc86430f4a5fcbafb7e280e00b`，验证器`-min-attempts 2`通过，11span/3error span反映第一次失败，不强行抹掉错误。

本地日志位于忽略目录`.superpowers/sdd/2026-10-14-interview-closeout/`；Jaeger内存数据重启后丢失。持续证据是提交的测试/脚本及对应远程run，ID不是永久线上查询接口。

### 本轮远程CI与未完成项

- [run38570319676](https://github.com/kaii9/codingJudge/actions/runs/38570319676)，headb4b71c6：新增真实编译测试暴露Linux目录Permission denied；test遇sum.golang.org HTTP/2传输错误；audit仍FAIL。
- [run38571367833](https://github.com/kaii9/codingJudge/actions/runs/38571367833)，head5052c14：浏览器/观测PASS；Linux权限问题尚未修复，MinIO构建再次遇HTTP/2错误。不能据浏览器通过声称该head全验收。
- [run38572861979](https://github.com/kaii9/codingJudge/actions/runs/38572861979)，head65e4138：test（含完整legacy asset smoke）、microservices（含新增Linux真实Go编译/取消、故障及collector）、e2e（5 PASS/2设备互斥SKIP）、observability全部PASS。frontend仅因完整audit FAIL，后续lint/typecheck/unit/build在该job被跳过，其本地结果单独列出。这个head不包含随后新增的下载重试脚本，后续提交须独立复验。
- [run38575469705](https://github.com/kaii9/codingJudge/actions/runs/38575469705)，head276b892：包含本轮全部运行时/测试修复与下载重试。test（含新脚本单测、真实MinIO构建与完整legacy asset smoke）、microservices（冷编译/取消、三语言、独立部署、真实在途故障与collector）、e2e（14.3s，5 PASS/2设备互斥SKIP）、observability全部PASS；唯一失败步骤仍是frontend的`npm audit --audit-level=high`。源码构建该次未出现已识别的传输错误，不据此声称真实CI触发过重试；重试分支由隔离行为测试验证。后续验收记录更新只改教材/计划，不改变276b892之后的运行时与测试实现。

2026-10-14再次查[官方公告](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm)：braces≤3.0.3受影响，patched versions仍为None。5项high是其开发依赖传递链，不是5个独立已证实的生产漏洞；production-only审计0仅用于诊断。用户选择保持完整审计门槛，因此没有豁免、强制降级不匹配插件、关闭校验或自动合并main。内部业务验收通过不等于整体安全验收完成。

## 首轮本地证据（历史，保留失败）

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
