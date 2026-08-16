# GlimoLab AI Gateway staging 验收与回滚

## 环境边界

- `staging-llm.glimolab.com` 是支付专用 staging：独立 PostgreSQL、Redis、容器、网络和 volumes，只验证 Stripe、Wallet、兑换码、退款登记及客户/运营界面。
- 支付 staging 不注入生产 channel、上游密钥或伪造 abilities。Model Square 显示 0 models 属于环境边界，不代表生产目录为空。
- 8 个当前 Pilot 文本模型的成功调用、真实 endpoint、路由倍率和扣费验收必须在另一个具有独立测试上游凭据的集成环境完成，或在单独批准的生产 canary 中完成。两个 GPT Image 模型应验证为不可见且返回 model-not-available。
- 不得为了让支付 staging 的目录“看起来完整”而创建不可调用的假 channel；这会产生错误的客户预期。

## 验收证据

- 受控客户账号：group=`b2b`、初始 quota=0、API Key group=`auto`。
- 受控运营账号：能创建兑换码、查看充值历史、登记已完成退款、查询混合模型用量和暂停客户。
- 客户桌面端与移动端截图：Wallet 双余额、充值、兑换码、订单历史、退款支持、模型目录、Usage Logs、API Key 安全设置。
- 管理员桌面端与移动端截图：客户分组、ability、充值订单、退款登记、审计日志和用量筛选。

## 自动与人工测试矩阵

以下矩阵分为支付 staging 与模型集成环境两部分。支付 staging 完成第 4 至第 7 项及界面验证；模型集成环境完成第 1 至第 3 项：

1. 8 个当前批准文本模型各完成一次成功调用，记录 request ID、实际路由组、倍率和扣费。
2. `codex-auto-review`、`dall-e-3` 和一个任意未批准模型返回 model-not-available，且无 default 回退。
3. GPT 文本分别覆盖 input、cached input、cache write、output、reasoning并验证标准外部客户 `b2b=0.3`；DeepSeek 覆盖 cache hit/miss/output并验证官方 base rate × `b2b-deepseek=1.10`；`gpt-image-1` 与 `gpt-image-2` 在 B2B `/v1/models` 中不可见且调用返回 `model_not_found`。
4. Stripe Test Mode 覆盖成功、取消、失败、延迟、重复 webhook、金额不符和币种不符；PayPal 不属于当前商业和验收范围。
5. 验证 US$20 最低充值：US$19 请求被拒绝，US$20 Checkout 与 Recharge Balance 按 1:1 处理，快捷充值选项不出现低于 US$20 的金额。
6. 覆盖只有现金、只有促销、混合余额、部分消费、全额/部分人工退款和重复退款登记；Pilot 退款不从客户金额中扣除 Stripe 原交易手续费。
7. 确认默认用户、生产 DeepSeek channel 和现有客户分组不变。

## 2026-07-19 支付矩阵完成记录

- Stripe 托管 Checkout 成功、客户返回取消、测试卡拒付、Stripe 原路全额退款均已通过真实 Sandbox 客户路径。
- 取消订单 `ref_b7030bf30a5286e1256e174dc2fdc81b8b91aace` 和拒付订单 `ref_cb000b65b5146ee4ac97076e39a0c15966bc8603` 均未入账；两次流程前后余额保持 Total US$60、Recharge US$35、Promotional US$25。
- PR `#26` 的签名 webhook 自动矩阵覆盖：无效签名、重复成功事件、延迟成功、异步失败、过期、金额不符、币种不符，以及 US$19/US$20 最低充值边界。
- PR 验证 run `29651630424` 已通过完整前端构建和 `go test ./...`。
- staging 部署 run `29651710761` 已通过不可变 ARM64 镜像构建、独立环境部署、本机/公网健康检查、客户 Docs 配置和临时凭证清理；镜像为 `ghcr.io/leoxk/new-api:staging-f777960028fee4859644c814d9f30defea2c9fae`。
- 部署后 Wallet、Billing History、`/docs` 和支持邮箱均已复核；390 x 844 视口下 Wallet 与 Docs 的页面宽度均为 390 px，无横向溢出。

支付专用 staging 的验收范围已经闭环。10 模型真实调用、实际路由和价格扣费仍属于独立模型集成环境或获批生产 canary，不得在没有上游凭据的支付 staging 中伪造完成。

## 发布门禁

必须附上测试日志、截图、价格映射和回滚演练结果，由 Leo 单独批准商户账号、生产支付凭证、最终价格、生产 canary、真实付款与退款。未经批准，workflow 只允许 staging 环境。

staging GitHub Environment 必须配置 `DEPLOY_HOST`、`DEPLOY_USER`、`DEPLOY_PATH`、固定的 `DEPLOY_SSH_KNOWN_HOSTS`、内外健康检查 URL，以及 `DEPLOY_SSH_PRIVATE_KEY` 和 `STAGING_RUNTIME_ENV` Secrets。目标是公网 VPS，GitHub Actions 直接通过项目专用 SSH key 连接，不需要 Cloudflare Access Service Token。运行时支付凭证只通过 `/dev/shm` 临时文件注入并在部署结束时删除；staging Compose 必须显式引用这些环境变量。

`Glimo B2B staging operations` 是独立的手动 workflow，只使用受保护的 `STAGING_ADMIN_ACCESS_TOKEN` 登记已经在 Stripe Sandbox 完成的退款。它必须先验证受控客户、订单状态、支付处理器、未退款状态、金额和 `re_...` ID；不得扩展为任意用户、任意余额或生产退款工具。

当前 staging 固定使用 Compose project `glimo-b2b-staging`、本机监听 `127.0.0.1:3100`、独立 Docker network `glimo-b2b-staging-network`，以及带 `glimo-b2b-staging-` 前缀的 4 个 named volumes。验收时必须证明 staging 容器没有加入 `vps-oci-sgp-new-api_new-api-network`，也没有挂载任何生产 volume。

## 回滚

1. GitHub Actions 选择上一个已通过验收的不可变镜像 SHA。
2. staging 部署脚本拉取该镜像并仅重建 New API 服务。
3. 复核容器状态、数据库迁移兼容性、staging 本地健康检查和外部 staging URL。
4. 新增退款列为向后兼容字段，旧镜像会忽略；不得为回滚而删除列或退款数据。
5. 如余额推导或退款功能异常，先隐藏管理员退款入口并停止退款登记，保留支付处理器与数据库证据后人工对账。
