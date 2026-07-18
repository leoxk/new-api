# Glimo AI Gateway staging 验收与回滚

## 验收证据

- 受控客户账号：group=`b2b`、初始 quota=0、API Key group=`auto`。
- 受控运营账号：能创建兑换码、查看充值历史、登记已完成退款、查询混合模型用量和暂停客户。
- 客户桌面端与移动端截图：Wallet 双余额、充值、兑换码、订单历史、退款支持、模型目录、Usage Logs、API Key 安全设置。
- 管理员桌面端与移动端截图：客户分组、ability、充值订单、退款登记、审计日志和用量筛选。

## 自动与人工测试矩阵

1. 10 个批准模型各完成一次成功调用，记录 request ID、实际路由组、倍率和扣费。
2. `codex-auto-review`、`dall-e-3` 和一个任意未批准模型返回 model-not-available，且无 default 回退。
3. GPT 文本分别覆盖 input、cached input、cache write、output、reasoning并验证 `b2b=0.1`；DeepSeek 覆盖 cache hit/miss/output并验证官方 base rate × `b2b-deepseek=1.10`；GPT Image 覆盖每个开放 size/quality 组合并验证 `b2b=0.1`。
4. Stripe Test Mode 覆盖成功、取消、失败、延迟、重复 webhook、金额不符和币种不符；PayPal 不属于当前商业和验收范围。
5. 覆盖只有现金、只有促销、混合余额、部分消费、全额/部分人工退款和重复退款登记。
6. 确认默认用户、生产 DeepSeek channel 和现有客户分组不变。

## 发布门禁

必须附上测试日志、截图、价格映射和回滚演练结果，由 Leo 单独批准商户账号、生产支付凭证、最终价格、生产 canary、真实付款与退款。未经批准，workflow 只允许 staging 环境。

staging GitHub Environment 必须配置 `DEPLOY_HOST`、`DEPLOY_USER`、`DEPLOY_PATH`、固定的 `DEPLOY_SSH_KNOWN_HOSTS`、内外健康检查 URL，以及 `DEPLOY_SSH_PRIVATE_KEY` 和 `STAGING_RUNTIME_ENV` Secrets。目标是公网 VPS，GitHub Actions 直接通过项目专用 SSH key 连接，不需要 Cloudflare Access Service Token。运行时支付凭证只通过 `/dev/shm` 临时文件注入并在部署结束时删除；staging Compose 必须显式引用这些环境变量。

当前 staging 固定使用 Compose project `glimo-b2b-staging`、本机监听 `127.0.0.1:3100`、独立 Docker network `glimo-b2b-staging-network`，以及带 `glimo-b2b-staging-` 前缀的 4 个 named volumes。验收时必须证明 staging 容器没有加入 `vps-oci-sgp-new-api_new-api-network`，也没有挂载任何生产 volume。

## 回滚

1. GitHub Actions 选择上一个已通过验收的不可变镜像 SHA。
2. staging 部署脚本拉取该镜像并仅重建 New API 服务。
3. 复核容器状态、数据库迁移兼容性、staging 本地健康检查和外部 staging URL。
4. 新增退款列为向后兼容字段，旧镜像会忽略；不得为回滚而删除列或退款数据。
5. 如余额推导或退款功能异常，先隐藏管理员退款入口并停止退款登记，保留支付处理器与数据库证据后人工对账。
