# 前端依赖安全：不降低审计门槛的工具链迁移

本章记录 GoJudge 前端 `braces` 漏洞的处理方法。没有改页面、后端接口或沙箱预算，也没有把所有 ESLint 检查直接换成另一工具的默认配置。

## 1. 先分清漏洞、传递依赖和实际验收

旧依赖链：

```text
eslint-config-next@16.3.8（开发依赖）
  → @next/eslint-plugin-next@16.3.8
  → fast-glob@3.3.1
  → micromatch@4.0.8
  → braces@3.0.3
```

[GHSA-vfj7-8cjw-p6xm](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm) 描述的是深层嵌套模式造成栈耗尽的拒绝服务；处理时上游尚无修复版本。完整 `npm audit` 的 5 项 high 是这一条链的传递报告，不能写成 5 个已证实的生产漏洞。开发依赖不是“可以不管”，工具在开发和 CI 中也会执行。

升级 fast-glob 或 micromatch 仍会带入 braces。npm 推荐的 eslint-config-next 14.x 降级不匹配当前 Next 16；禁用某条 lint 规则也不会让已安装依赖消失。因此没有使用 `audit fix --force`、`--omit=dev`、审计白名单、换包名或修改第三方源码来伪装修复。

## 2. 为什么选择两种 lint 工具

- ESLint 保留原来的 React recommended、React Hooks recommended（含 React Compiler 检查）、TypeScript recommended、6 项可访问性检查和匿名默认导出提示。原 warning/error 级别保持。
- Biome 2.5.15 负责 Next domain；显式启用全部该 domain 规则，并将同步脚本设为 error，避免它的默认 warning 降低原 Core Web Vitals 门槛。额外显式保留 Google Fonts display 提示。
- Biome 的 Hooks 两条规则关闭，由原 React Hooks 插件负责，避免两套算法重复报告或误以为两者完全等价。
- Biome 没有提供的关键 App Router 约束，使用 ESLint 自带的 AST selector / restricted imports，不复制 Next 插件，也不另造解析器。

Next、React、Monaco 等运行时版本不变。移除 eslint-config-next 后，其原来隐式带入的插件成为直接、固定版本的开发依赖。锁文件一并更新，全新 `npm ci` 用它重建依赖树。

[Next 官方说明](https://nextjs.org/docs/app/getting-started/installation#set-up-linting)支持直接使用 ESLint 或 Biome；Next 16 的 build 不自动执行 lint，故 CI 必须单独执行 lint。[Biome 迁移说明](https://biomejs.dev/installation/migrate-eslint-prettier/)也提醒规则并不全部等价。

## 3. 原 Next 规则的覆盖与差异

这是针对本项目 App Router 的迁移，不宣称全量复制 eslint-config-next。表中“旧规则”均指原 `@next/next/` 规则。

| 旧规则 | 本项目替代或边界 |
| --- | --- |
| inline-script-id | Biome useInlineScriptId，error；验证普通及别名导入 |
| no-sync-scripts | Biome noSyncScripts，显式 error；defer 正向对照 |
| no-html-link-for-pages | ESLint no-restricted-syntax：静态根相对、相对路径、字符串表达式与模板前缀使用 next/link；download、协议 URL、`//`、fragment、query 允许 |
| no-assign-module-variable | AST 禁止声明名为 module 的变量，error |
| no-document-import-in-page | App Router 全局禁止 next/document，error；使用 app/layout.tsx |
| no-head-import-in-document | App Router 全局禁止 next/head，error；使用 Metadata API，约束比旧规则更严格 |
| no-duplicate-head / no-script-component-in-head | 上述两个旧模块的静态导入被禁止，阻断其原使用路径；不是单独复刻两条 Pages Router 规则 |
| no-async-client-component | Biome noNextAsyncClientComponent，error（原为 warning）；异步 Server Component 仍允许 |
| no-before-interactive-script-outside-document | Biome 同名对应规则，遵循其 App/Pages 路径判定，不承诺完全相同 |
| no-head-element | Biome noHeadElement；其对 app 目录的处理与旧规则存在差异 |
| no-img-element | Biome noImgElement，提示保留；不是禁止所有图片 |
| no-unwanted-polyfillio | Biome noUnwantedPolyfillio，提示保留；不是完整外部脚本安全扫描 |
| google-font-preconnect | Biome useGoogleFontPreconnect |
| google-font-display | 显式 Biome useGoogleFontDisplay，warning |
| next-script-for-ga / no-css-tags / no-location-assign-relative-destination / no-page-custom-font | 没有一对一替代；原为 warning。当前代码不使用这些模式；引入手写第三方脚本、stylesheet、location.assign 或外链字体时须专门 review，而不能宣称 lint 自动兜底 |
| no-styled-jsx-in-document / no-title-in-document-head / no-typos | Pages Router 特有路径没有单独复刻。本项目不使用 pages/_document 或 getStaticProps/getServerSideProps；增加 Pages Router 必须重新评估规则，不能沿用本表声称兼容 |

链接检查是静态质量约束，不是运行时 URL 安全验证：任意变量的 href、运行时拼接、JSX spread 无法在这里可靠求值；模板只判断第一个静态片段，不错误地拦截外部 URL 的后续路径。本项目约定站内导航包括新窗口导航也使用 Link，比旧插件允许部分 target=_blank 更严格。

配置入口：[ESLint](../../../frontend/eslint.config.mjs)、[Biome](../../../frontend/biome.json)、[依赖和命令](../../../frontend/package.json)。没有关闭格式要求来掩盖代码问题：本项目此前没有 formatter gate，本次只迁移 lint，不做全库格式重写。

## 4. 如何验证迁移没有把检查丢掉

[lint 契约测试](../../../frontend/tests/lint-contract.test.mjs)运行真实工具和仓库配置，不 mock 规则、不 grep 配置源码：

- 错误代码：条件调用 Hook、render 中 setState、列表缺 key、显式 any、内部原生 a 导航、module 变量、旧 Document/Head 导入、同步脚本、没有 id 的 Next Script、async Client Component。
- 正向对照：Link、下载、外部链接和带插值的外部 URL、async Server Component、正常 Client Component、有 id 的 Script、defer 脚本。
- warning 仍可见：effect 缺依赖、img 缺 alt、普通 img、重复 polyfill、Google Fonts display。

新增内部链接和外部模板边界测试先失败，修正 selector 后通过；Google Fonts display 在只开 Next domain 时没有提示，显式补齐后通过。同步脚本的 severity 另做降级 mutation 对照，证明仅“有诊断”还不够，必须使进程失败。原来已经存在的 React/Hooks 规则是迁移保护测试，不把其原本通过伪称为业务修复 RED。

Biome 标准输入模式在本环境仅返回通用的 contents aren't fixed，不能据此判断命中了哪条规则。测试因此在系统临时目录创建真实 tsx fixture，指定本项目配置，断言诊断类别和退出码；finally 删除测试自身创建的目录。临时文件不落入仓库，也不自动修复待测代码。

```bash
cd frontend
npm ci
npm audit --audit-level=high  # 完整图，包含开发依赖
npm run lint                # ESLint 和 Biome 都执行
npm run typecheck
npm run test:lint            # 可单独看 lint 的负向/正向对照
npm run test:run             # 上述契约 + 全部 Vitest；不允许无测试假绿
npm run build
```

E2E 需要真实后端、判题镜像及与之连接的前端，参见[第4章](04-tests-ci-demo.md)。本次本地使用微服务 API 28080 和独立前端 23011：

```bash
API_INTERNAL_URL=http://127.0.0.1:28080 npm run start -- --hostname 127.0.0.1 --port 23011
# 另一个终端
PLAYWRIGHT_BASE_URL=http://127.0.0.1:23011 npm run test:e2e -- --workers=1 --repeat-each=2
```

本地完整重装后 audit 0、lint/typecheck/unit/build 通过；浏览器两轮 10 PASS、4 设备互斥 SKIP，Go/C++/Python 实际 AC，不是 mock 结果。远程以[收尾分支对应提交的 CI](https://github.com/kaii9/codingJudge/actions?query=branch%3Afeat%2Fgojudge-interview-closeout)为准，不能拿上一个仍 audit 失败的 head 冒充本次完成，也不能把本地通过等同远程通过。历史失败保留在[原验收记录](verification-2026-10-14.md)。

## 5. 维护边界：audit 0 不等于永远安全

完整 high 门槛没有放宽，未来新增高危仍会阻断 CI。Biome 的二进制平台包也属于锁定依赖，需随工具版本评估、更新和复验，而不是永久固定后不再维护。

另有已识别的维护风险：[ESLint 9 已结束官方维护](https://eslint.org/version-support/)。处理时 eslint-plugin-react、jsx-a11y、import 的最新 peer range 仍不支持 ESLint 10；本次保留项目原来兼容的 9.39.5，不使用 force/legacy-peer-deps 强装。它目前完整 audit 无已报告漏洞，但不能据此宣称工具链所有组件都在官方维护期。下一次插件升级应核对 peer range；可兼容后升级 ESLint 10 并重跑契约测试。若需要生产供应链“全部组件受支持”的政策，则这是额外未满足条件，不能用当前 audit 0 替代。

## 6. 面试怎样讲

“我发现完整依赖审计被一个无上游补丁的开发依赖阻断。先沿依赖树确认来源，再用兼容的 lint 工具组合移除整条链；没有排除 dev、降级框架或增加漏洞白名单。迁移不是只看 audit 变绿，我保留 React/Hooks/TS 检查，明确 Next 规则差异，用实际错误代码和正向对照验证，再跑 clean install、构建和真实三语言浏览器判题。audit 0 表示当前数据库没有报告依赖漏洞，不意味着没有安全风险，也不代表 mTLS、沙箱或生产供应链已全部验收。”

可追问的重点：为什么是开发依赖仍会失败、禁用规则为何不能解决审计、为什么不强制安装建议版本、两种 linter 的代价、负向测试与 E2E 各证明什么、如何区分漏洞修复与兼容/维护风险。
