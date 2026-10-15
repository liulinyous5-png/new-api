# Anthropic 通道使用策略

Anthropic 官方API的封禁因素很多，原因也不只一种。速率、额度、出口、账号状态和请求内容都可能单独触发限制。下面的执行策略是预防动作，不能保证账号不被封。



## 平台执行策略



### 1. 出口与账号登记地区保持一致

大额官key注册、充值、调用走同一地区。请求出口使用该账号在注册、充值或企业开通时已经限定的白名单 IP 地区。

- 一把密钥固定一个通道、一个地区、一条出口。
- 同一地区优先使用高质量静态 IP



### 2. 按账号等级把速率压在额度的 70%–80%

先按该账号等级核对每个模型的每分钟请求数（RPM）、每分钟输入 Token 数和每分钟输出 Token 数。平台分组限流按请求次数控制，不单独计 Token，所以请求频率按对应模型 RPM 的 70%–80% 设置，给 Token 额度留出余量。新密钥从更低水位开始，再逐步提高。

出现 429 时先看错误类型，只冷却这一条渠道，不对它连打：

| 429 类型 | 处理 |
| --- | --- |
| 带 `retry-after` | 冷却该渠道，过了等待时间后按原水位恢复 |
| 没有 `retry-after`，或正文表示月度额度、花费上限用尽 | 该渠道本周期停用，额度恢复后再启用 |



### 3. 错误冷却

系统按渠道自动统计 400 和内容拒绝。达到设定频率后自动冷却该渠道，冷却期间不再向该渠道发送流量，也不重试同一请求。单次 400 或 429 不写入全局自动禁用状态码，避免一次坏请求误关通道。

短时间大量 400（参数不合法，或使用政策拦截）会把单次失败放大成账号风险，参数错误集中出现同样有停号风险。系统统计到该渠道 400 达到频率后自动冷却。参数修正后，从低水位恢复。

请求内容也是封禁因素。上游返回内容拒绝时，系统按拒绝类别计数，不把同一请求换渠道重打。

蒸馏、逆向或复制模型输出被拒绝时，响应里会出现：

```json
"stop_details": {
  "type": "refusal",
  "category": "reasoning_extraction",
  "explanation": "This request was blocked as it seems to violate Anthropic's Terms of Service restrictions on reverse engineering or duplicating model outputs. To learn more, visit https://www.anthropic.com/legal/commercial-terms. API integrators: you can reduce refusals for your users by configuring a fallback model — see https://platform.claude.com/docs/en/build-with-claude/refusals-and-fallback"
}
```

- 系统统计 `category` 为 `reasoning_extraction` 的次数。达到设定频率后，自动冷却发出该请求的渠道。冷却期间这个调用方不再进入该渠道。
- 同类拒绝连续出现时，系统保持摘流，先去掉这类请求再恢复。

