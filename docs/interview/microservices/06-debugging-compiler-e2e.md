# 6. 实战排障：Go 冷编译与本地 E2E

本章记录继续验收时定位的三个不同问题：编译并行度与单核配额、浏览器平台快捷键、虚拟编辑器点击焦点。不要将它们统一归结为“机器慢”，也不要看到一次通过就删掉先前失败记录。

## 6.1 编译时间不等于用户程序运行时间

| 预算 | 当前值 | 作用 |
| --- | --- | --- |
| 编译沙箱 | 30s、1 CPU、512MiB、64 PID | Go/C++编译，不使用题目64MiB运行内存 |
| 单用例用户程序 | 题目TimeLimitMS，例如2s | 可信包装器起止帧计时，非Docker全生命周期 |
| 单用例生命周期 | 运行限时 + 20s | 有界处理Docker启动/退出开销 |
| Executor RPC | 120s | 实例等待、编译、多个用例执行 |
| Worker单次任务 | 3min | Claim、对象读取、RPC、持久化 |
| 浏览器正常流程等待 | verdict120s、整个测试150s | 功能验收，与RPC预算对齐，不是性能SLO |

之前E2E只等30s，实际提交约29.6s落库AC，浏览器下一次轮询尚未显示便报错。它没有证明编译失败。另一方面，历史也确实有 `compilation timed out`，不能因为某个提交后来AC就把所有失败解释为测试超时。

正确排查是先看提交ID的最终status/stderr/attempts，再看trace与日志，分清UI等待失败、编译错误、用户代码错误和基础设施重试。新的等待仍有上限，遇到CE/RE/WA/TLE/internal_error会立即失败，不能等到成功才算通过。

## 6.2 为什么显式限制Go编译并行度

每次编译容器有自己的tmpfs GOCACHE，不共享用户可写缓存。因此标准库可能冷编译；扩大共享缓存虽可能更快，但需要分析缓存污染/跨用户隔离，不能为了演示速度直接挂同一可写缓存。

目前命令为：

```sh
GOMAXPROCS=1 GOCACHE=/tmp/go-build HOME=/tmp \
  go build -p 1 -o /workspace/program /workspace/main.go
```

`-p 1` 约束同时构建的包数，`GOMAXPROCS=1` 约束Go工具/编译器的并行执行。两者与单核容器配额一致。Go1.25会考虑cgroup CPU配额，但并非所有情况下都恰好选一个线程；本项目不依赖自动推断。没有修改用户程序的运行并发或放宽生产超时。[Go1.25官方说明](https://go.dev/doc/go1.25#runtime)

本机一次同参数冷构建对照为默认13.22s、显式单并行9.85s；后者真实Runner双用例测试约9.19s。这只说明该环境下的观测，不能给简历写“吞吐提升25%”或保证任何负载下都低于30s。

源码：[编译命令与参数](../../../internal/judge/docker_runner.go)。单元测试 `TestGoCompilerParallelismMatchesSingleCPUSandbox` 先因缺失并行度配置失败，再通过；同时检查CPU、内存、PID、无网络、cap-drop、no-new-privileges、tmpfs和30s预算没有被放宽。

## 6.3 真实集成：正常、语法错误、实际在途取消

```bash
docker pull golang:1.25-alpine
TEST_GO_COMPILER=1 go test -tags integration ./internal/judge \
  -run 'TestColdGoCompiler|TestInFlightGoCompile' -count=1 -v
```

[测试源码](../../../internal/judge/go_compile_integration_test.go)：

- 正常Go程序完成冷编译后，两组输入分别输出3和2，结果Stage为run；不是只看`go build`退出0。
- 语法错误返回StageCompile、非0退出码、错误信息，不得当作TLE或AC。
- 取消测试先用真实`docker ps`确认带独有owner标签的编译容器正在运行，再取消父Context；必须返回context.Canceled且没有留下该owner的沙箱。不是启动前取消Context的假在途测试。

测试启用需要显式环境变量；没有变量而SKIP不算集成通过。CI微服务job先准备判题镜像，再运行这些测试。测试结束只检查自己的owner标签，不删其他项目容器。

## 6.4 宿主系统与模拟浏览器不是一回事

本机Node运行在macOS，但Playwright `Desktop Chrome` 设备预设的UA是Windows。Monaco浏览器端按`navigator.userAgent`选择平台快捷键。因此根据`process.platform`发送Meta+A会错：后者回答的是“测试程序跑在哪里”，不是“编辑器模拟哪个平台”。

修复按编辑器同一平台判定：Macintosh用Meta+A，其他当前设备预设用Control+A；随后检查真实Monaco selection覆盖整个model。这里没有通过Monaco API直接setValue来跳过界面输入。

依据是所用Monaco0.55.1的本地 `node_modules/monaco-editor/esm/vs/base/common/platform.js`，不是猜测。若将来更换Monaco平台检测或iOS设备，需重新核对；当前不宣称所有浏览器组合已验证。

## 6.5 为什么不要force-click虚拟textarea

Monaco的inputarea是虚拟输入缓冲区，可能透明或随光标重新定位。强制点击跳过可交互检查，有时点到别的层，留下未聚焦输入。以前只检查textarea的selectionStart/End也可能误认“空缓冲区全选”为“整个源码全选”。

现在点击可见代码画布 `.view-lines`，断言真实textarea获得焦点，再按浏览器平台快捷键，检查完整model选区，键盘输入并核对model文本，最后检查实际POST字节。

Go/C++固定测试程序允许编辑器输入时的水平缩进变化；Python保持字节精确校验，因为缩进有语义。判断不以“Submit按钮点了”结束，而是确认服务收到的代码、终态和历史记录一致。

## 6.6 面试如何讲这个排障案例

推荐顺序：失败现象 → 提交ID/截图/日志证据 → 区分假设 → 最小修复 → 相同测试复跑 → 仍存在的边界。

可以回答：我没有直接加大判题限时。先确认编译和RPC预算不同，把编译并行度匹配CPU配额；浏览器侧又找到了模拟平台与宿主平台混淆，以及虚拟textarea的焦点问题，保留真实model与请求字节断言。功能等待与现有RPC预算一致，错误终态仍立即失败；性能验证另做，不用功能测试等待值充当SLO。

练习：如果页面显示Running但数据库已经AC，你会优先查什么？如果数据库CE并写了`compilation timed out`，又应该查什么？两者的修复不应相同。

完整结果和远程CI按 [最新验收记录](verification-2026-10-14.md) 的版本阅读。`braces`未修复开发依赖是另一条独立安全门禁，本章的业务/测试修复不构成审计豁免。
