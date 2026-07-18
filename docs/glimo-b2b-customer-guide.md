# Glimo AI Gateway B2B 客户指南

> 服务阶段：Best-effort Pilot，不承诺固定 SLA。客户账号须经 Glimo Lab 审批并手工加入 `b2b` 组。

## 快速开始

1. 登录客户控制台，在 API Keys 中创建密钥。B2B 密钥使用自动路由，无需选择内部渠道。
2. Base URL：`https://llm.glimolab.com/v1`
3. 请将密钥放入环境变量，不要写进代码仓库、聊天记录或前端代码。

```bash
export GLIMO_API_KEY='your-api-key'

curl https://llm.glimolab.com/v1/chat/completions \
  -H "Authorization: Bearer $GLIMO_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5.6-sol",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

```python
from openai import OpenAI

client = OpenAI(base_url="https://llm.glimolab.com/v1", api_key="your-api-key")
response = client.chat.completions.create(
    model="deepseek-v4-pro",
    messages=[{"role": "user", "content": "Hello"}],
)
print(response.choices[0].message.content)
```

```javascript
import OpenAI from 'openai'

const client = new OpenAI({
  baseURL: 'https://llm.glimolab.com/v1',
  apiKey: process.env.GLIMO_API_KEY,
})

const response = await client.chat.completions.create({
  model: 'gpt-5.5',
  messages: [{ role: 'user', content: 'Hello' }],
})
console.log(response.choices[0].message.content)
```

GPT Image 的开放 endpoint、size 和 quality 组合只以登录后 Model Catalog 中标记为已验证的项目为准。不要假设上游支持但目录未标记的组合可用。

## 批准的模型目录

| 类别 | 模型 | 客户计价基础 |
|---|---|---|
| GPT Text & Reasoning | `gpt-5.4`、`gpt-5.4-mini`、`gpt-5.5`、`gpt-5.6-luna`、`gpt-5.6-terra`、`gpt-5.6-sol` | OpenAI Standard / Short context 对应参考价的 10% |
| DeepSeek | `deepseek-v4-flash`、`deepseek-v4-pro` | DeepSeek 官方 API 基础价 × 1.10（含税费与支付成本覆盖） |
| GPT Image | `gpt-image-1`、`gpt-image-2` | OpenAI Standard 图像对应参考价的 10% |

`codex-auto-review`、`dall-e-3`、OpenAI Long context、Batch、Flex、Priority 以及其他未批准模型不属于 B2B 产品。模型目录和有效价格以登录后的私有客户控制台及销售报价为准；公开网站不展示详细倍率或费率表。

## 余额、充值和兑换码

- Total Balance：Recharge Balance 与 Promotional Credit 的合计，也是 API 实际可使用的总余额。
- Recharge Balance：现金充值后形成的余额，可申请人工退款审核。
- Promotional Credit：兑换码或人工赠送形成的额度，优先消费、不可退款、不可转移。
- 现金充值保持 1:1：支付 US$1，形成 US$1 Recharge Balance。模型倍率只影响调用费用，不改变充值价值。
- Stripe 最低充值金额为 US$20；控制台不会提供低于最低金额的快捷充值选项。
- 充值仅支持 Stripe 托管的信用卡/借记卡 Checkout；Glimo Lab 不接触或保存银行卡号。
- Stripe 提供付款和退款收据；如需公司发票，请联系支持人员人工处理。

## 人工退款

通过支持邮箱提交订单号、付款账号、退款原因和联系人。运营人员会核对成功充值、历史退款、当前余额及完整用量。仅未使用的 Recharge Balance 可原路退回；Promotional Credit 无现金价值。控制台没有自动退款按钮，任何退款都必须经人工确认。Pilot 阶段经审核批准的退款不会从客户退款金额中扣除原交易的 Stripe 手续费；该费用由 Glimo Lab 承担。

## 用量与排错

Usage Logs 可按 API Key、模型、日期、状态和 request ID 查询。联系支持时请提供 request ID，切勿提供完整 API Key。

- `401`：密钥缺失、错误、过期或已被删除。
- `403`：账号被暂停、无权使用目标分组或模型不在批准目录。
- `429`：达到请求速率、并发或余额限制。
- `5xx`：网关或上游暂时异常；Pilot 阶段可在确认请求幂等后重试，并保留 request ID。

建议为不同系统分别创建密钥，定期轮换；人员离职或疑似泄露时立即删除旧密钥。账号应启用 2FA 或 Passkey。

## 支持范围

本服务面向已批准 B2B 客户，不开放消费者自助注册获客。支持联系方式以控制台 `Refund & Support` 区域公布的信息为准。
