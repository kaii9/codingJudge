# 剩余验收修复：编译并行度与本地浏览器

**当前状态（2026-10-15）：完成并经用户批准合并。** 新的前端工具链移除了 braces 传递链；`75cf2af` 对应 run38614563014 五个 job 全 PASS，合并后的主工作区复验通过。见[主线合并验收](../../interview/microservices/verification-2026-10-15.md)。下方“不合并/审计 FAIL”保留为2026-10-14历史，不能作为当前未完成项。

用户批准继续未完成项。工作分支延续 `feat/gojudge-interview-closeout`，基础提交788913f，不合并main，不触碰已有用户文档或数据卷。

## 目标与边界

1. 重新核对完整CI、npm依赖与官方公告。未修复braces继续阻断完整audit，不能降级不匹配Next插件、屏蔽审计、换包名隐藏漏洞或维护未经充分验证的框架fork。
2. 诊断Go冷编译：一个CPU、512MiB编译内存、64PID、64MiB tmpfs、30s compile budget均保留。检查默认并行编译是否与限额匹配，通过真实冷编译对照决定最小调整。
3. 对调整写失败测试，包含编译参数、保留安全限制、Go正常/语法错误和父取消的真实集成验证。只改变构建并行度时禁止引入跨用户可写缓存。
4. 重建Executor，运行正常三语言、真实fault/trace/数据库/Go suite，再做本机浏览器操作和已有E2E核验。浏览器调试按当前Playwright技能使用CLI；已有仓库测试保留。
5. 更新面试教材解释编译与运行预算、冷缓存和资源竞争，记录通过/失败范围；推送同一分支重新核对远程CI，审计未通过则保留分支不合并。

执行中补充的必要修复：真实Linux compiler集成发现cap-drop后的root无法写runner拥有的0755目录，按已验证daemon挂载语义匹配编译UID/GID，不增加权限。MinIO源码构建两次遇到sum.golang.org瞬时HTTP/2错误，新增仅限传输错误的3次重试；checksum和永久错误不得重试，不修改GOSUMDB。

## 验收记录

2026-10-14初查：CI38538827449，head788913f，test/microservices/e2e/observability PASS，frontend audit FAIL。npm braces最新3.0.3；Next eslint最新16.4.0仍依赖fast-glob3.3.1。官方GHSA-vfj7-8cjw-p6xm暂无patched版本。当前本地栈健康，未删卷。

2026-10-14继续执行：b4b71c6编译并行度与真实Go测试；fdfe68c按模拟浏览器UA选择快捷键；5052c14点击可见Monaco画布并断言焦点，本机重复E2E10 PASS/4设备互斥SKIP；65e4138匹配编译目录身份，远程CI38572861979的test/microservices/e2e/observability均PASS（含Linux真实Go冷编译与取消），frontend仅完整audit FAIL。完整历史、后续镜像重试与最终head验收见[收尾记录](../../interview/microservices/verification-2026-10-14.md)。

收尾：276b892补齐最多3次、checksum保持的下载重试；真实镜像重构建/部署、mc bucket stat、三语言/故障/trace通过。新镜像上的本机重复E2E10 PASS/4布局SKIP（52.1s），完整Go/vet、选定race、整套真实Docker Runner、PG3项、前端13文件183测试通过，Markdown链接通过。远程CI38575469705验证276b892，test/microservices/e2e/observability全PASS，仅完整audit FAIL。实施与业务复验完成；本计划目标内剩余阻断是上游braces未修复的完整安全审计。保持收尾分支，不合并、不删除工作区证据、不把总体安全验收标为完成。
