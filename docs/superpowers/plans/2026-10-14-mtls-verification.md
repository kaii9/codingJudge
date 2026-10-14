# 已有 mTLS 能力的真实 RPC 验证

**当前状态（2026-10-15）：验证完成并合并。** mTLS 实现边界不变；独立前端 audit 阻断已解决，`75cf2af` 的完整 CI 五个 job 全 PASS，见[主线合并验收](../../interview/microservices/verification-2026-10-15.md)。后文是当时实施及受阻记录，不代表仍禁止合并。

用户要求继续开发。延续已批准的微服务调用治理与故障验证目标，补齐现有 TLS 配置的验证缺口，不新增业务、服务、框架或证书轮换。

## 范围

1. 真实 loopback TCP + gRPC，不用 fake TLS 或仅检查配置字段。
2. 临时 ECDSA CA、serverAuth/clientAuth 证书由 Go 标准库生成，只存测试临时目录，私钥0600；不写仓库、不修改机器信任库。
3. 验证正确双向证书可调用，TLS1.3与服务端已验证客户端身份；无证书、错误CA、过期证书、名称不匹配、错误EKU、TLS1.2及明文不能进入handler。
4. 通过真实 Judge/Executor 适配器证明 TLS options覆盖默认本地明文选项；可信证书不绕过服务Token，不写额外提交/不执行Runner。
5. 正向控制避免把“端口没开/所有连接都坏了”误认为安全通过。缺客户端证书的客户端能连接单向TLS对照服务，但不能连接现有mTLS服务。
6. 所有调用有deadline；关闭本测试listener/connection/server，不影响现有Compose、数据卷与用户文档。
7. 更新教材、测试命令与验收记录；完整Go/race/vet核验，推送收尾分支。完整npm audit门槛仍保持，未修复braces继续阻断合并。

## 判断边界

此阶段是对已有能力增加契约/传输层测试，已有实现正确时可直接通过，不伪造RED。若暴露实现缺陷，先记录真实失败，再做最小修复。不能由loopback测试宣称跨主机部署、证书热更新/轮换、吊销、服务级证书ACL或生产PKI已经完成。

## 本地执行结果

2026-10-14：新增3组真实测试首跑通过，10轮重复通过，无SKIP；选定3包race、完整Go suite、vet通过。现有TLS/业务授权运行时实现没有更改。加入独立CI race步骤与教材3.6.1，远程结果按新提交对应run核对，完整audit保持原门槛。具体测试范围及非生产边界见[验收记录](../../interview/microservices/verification-2026-10-14.md)。

远程验收：7464984对应run38588571845，新mTLS race步骤与test/microservices/e2e/observability全部PASS；唯一失败仍为原完整前端audit。此验证阶段完成，审计阻断保持，不合并收尾分支。未改变任何运行时TLS策略，也没有实现跨主机PKI、轮换或吊销。
