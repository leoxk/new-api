# Glimo AI Gateway B2B 运营 SOP

## 不可越过的边界

- 不自动迁移现有用户；仅在客户审批完成后由管理员手工改为 `b2b`。
- B2B API Key 使用 `auto`，不可启用跨组回退到 `default`。
- 不删除、停用或改写现有 DeepSeek channel。
- 不向 B2B 开放 `codex-auto-review`、`dall-e-3` 或其他未批准模型。
- 注册赠送、邀请奖励、签到奖励和订阅保持关闭；只使用兑换码发放促销额度。
- Stripe Promotion Codes 和 Amount Discount 保持关闭。
- Glimo Lab 当前只接入 Stripe；不得在 staging 或生产注入 PayPal 凭证或向客户显示 PayPal 入口。
- Stripe 最低充值金额固定为 US$20，快捷充值选项不得包含低于 US$20 的金额。
- Komodo 仅监控；GitHub Actions 是唯一自动部署路径。

## 客户审批与测试账号

1. 核对公司主体、业务用途、预计用量、联系人和账单邮箱。
2. 在 staging 创建初始 quota 为 0 的受控客户账号。
3. 手工设为 `b2b`，创建 group=`auto` 的测试 API Key。
4. 确认模型列表恰好来自 `b2b` 与 `b2b-deepseek` abilities 的并集。
5. 完成 10 个批准模型、2 个排除模型、一个任意未批准模型的调用验证。
6. 生产 canary 必须另行批准；不得把测试凭证发给外部人员。

## 价格复核

支付启用前逐项复核并保存证据；任何生产价格变更仍须 Leo 明确批准：

- 6 个 GPT 文本/推理模型只采用 OpenAI Standard / Short context；input、cached input、cache write、output、reasoning 分别核对，最终有效倍率为 `0.1`。
- 2 个 DeepSeek 模型的 base rate 分别等于官方 cache hit、cache miss/input、output 价格，再应用内部组 `b2b-deepseek=1.10`，用于覆盖税费和支付成本。
- 2 个 GPT Image 模型按实际开放的 endpoint、size、quality、input image 与 output token/图片维度逐项核对，倍率为 `0.1`。
- `TopupGroupRatio` 中 `b2b` 与 `b2b-deepseek` 均保持 `1.0`；DeepSeek 的 1.10 只作用于调用扣费，不作用于充值价值。

保存官方价格来源 URL、核对日期、New API 配置导出和最小测试扣费证据。未批准前不得改生产价格。

## 兑换码

1. 记录客户、审批人、金额、用途和到期日。
2. 创建单次兑换码；通过安全渠道交付。
3. 兑换后检查 Total Balance 与 Promotional Credit 增加相同额度，Recharge Balance 不变。
4. 促销额度不可退款、不可转移。

## 支付对账

每日或有交易日核对支付处理器成功交易与 `top_ups`：订单号、用户、金额、币种、状态和完成时间必须一致。Stripe webhook 必须先验签，再核对本地订单金额与 USD 币种；重复事件不得二次入账。异常订单停止人工补单并保留 request/event ID。

Stripe 在本分支支持以 `STRIPE_API_SECRET`、`STRIPE_WEBHOOK_SECRET`、`STRIPE_PRICE_ID` 运行时环境变量覆盖旧设置；Glimo staging/production 必须使用 GitHub Environment Secrets 注入，不在 New API 数据库选项或主机长期文件中保存生产 secret。Stripe Promotion Codes 与 Amount Discount 继续关闭。

对账时单独记录 Stripe 原交易手续费、结算币种、Stripe FX 汇率和净入账金额。客户 Recharge Balance 始终按 USD 付款金额 1:1 入账，不按 HKD 净结算额折算或减少。

## 人工退款

1. 收到申请后冻结该笔退款处理，导出客户成功充值、历史退款和完整 Usage Logs。
2. 以 Wallet 当前显示的 Recharge Balance 为系统退款上限。系统仍只扣减 `users.quota`，但 Promotional Credit 使用兑换记录中的 `used_quota_at_redemption` 快照按兑换顺序推导，不能再用 `min(users.quota, 历史净现金充值)` 作为唯一算法。
3. 同时导出成功充值、兑换码、历史退款和 Usage Logs 做人工复核；如显示余额与人工记录不一致，暂停退款并升级调查，不要通过直接改 quota 强行对平。
4. 复核退款金额不超过原订单未退金额且不超过当前 Recharge Balance，然后在 Stripe 原路退款。Pilot 的真实退款必须经过授权人员人工确认；争议、chargeback 或金额异常时升级给 Leo。
5. 只有处理器显示完成后，管理员在 Billing History 选择 `Record Completed Refund`，填写处理器 refund ID、金额和原因。
6. 确认系统在一次事务中写入退款字段并扣除等额 quota；核对 Total = Recharge + Promotional。
7. Pilot 阶段不得从客户退款金额中扣除 Stripe 不退还的原交易手续费；将该手续费记录为 Glimo Lab 支付成本。
8. 保存处理器凭证、审计日志和客户通知。不得用此按钮发起支付处理器退款，也不得手工回写 `users.quota`、`used_quota` 或兑换快照。

Chargeback 确认后按同一人工流程登记；如当前 Recharge Balance 不足，先暂停账号并由负责人决定追偿或坏账处理，禁止把余额扣成负数。

## 路由和异常处理

- GPT/GPT Image 应记录实际 group=`b2b`、group ratio=`0.1`。
- DeepSeek 应记录实际 group=`b2b-deepseek`、group ratio=`1.10`，并在内部对账中同时保留官方 base rate、10% 成本覆盖和最终扣费。
- 日志至少核对客户、API Key、模型、实际 group、group ratio、token/image 维度、quota、request ID、时间和状态。
- 错路由时先暂停受影响模型 ability，不修改 DeepSeek 生产 channel；保留日志后回滚最近配置或代码版本。

## 暂停、恢复和事件沟通

出现密钥泄露、欺诈、chargeback、异常突增或违反使用政策时可暂停账号。记录原因、证据、操作者和时间。恢复必须由另一名授权人员复核。Pilot 事故通知说明影响时间、受影响模型、临时措施和后续更新，不承诺固定恢复时限。

## 月度导出

按客户、API Key、模型类别、单模型、`b2b`/`b2b-deepseek`、日期、成功/失败导出请求数、token/image 数量和费用。Recharge/Promotional 分类只按资金来源计算，不按模型类别或倍率重新分类。

客户销售报价和登录后私有费率卡可说明 DeepSeek 为“官方 API 基础价 × 1.10，含税费与支付成本覆盖”。公开网站继续只描述可用模型和 B2B 服务能力，不公开详细倍率、内部组名或完整费率表。
