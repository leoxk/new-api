# Glimo AI Gateway B2B 生产配置只读审计

本审计器把已批准的 10 模型目录、官方基础价格维度、客户倍率、充值倍率、自动路由和禁止回退规则固化为代码。它只读取数据库，不修改 option、用户、token、channel、ability、价格或余额。

## 执行

在具有生产 SSH 只读访问能力的受控终端，从仓库根目录执行：

```bash
ssh root@vps-oci-sgp.leocoral.com \
  "docker exec -i new-api-postgres sh -lc 'psql -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" -At'" \
  < scripts/export-glimo-b2b-policy-snapshot.sql \
  | node scripts/glimo-b2b-policy-audit.mjs
```

需要机器可读证据时添加 `--json`。脚本成功返回 0，发现配置漂移返回 1，输入或执行错误返回 2。输出不包含 API Key、用户密码、Stripe 密钥、channel key 或完整请求内容。

## 审计范围

- `b2b=0.1`、`b2b-deepseek=1.1`；
- 两组 `TopupGroupRatio=1.0`；
- `b2b` 恰好包含 6 个 GPT 文本模型和 2 个 GPT Image 模型；
- `b2b-deepseek` 恰好包含 2 个 DeepSeek 模型；
- `codex-auto-review`、`dall-e-3` 和其他模型没有进入两组；
- ability 和对应 channel 均启用；
- 6 个 GPT 文本的 Standard/Short input、cached input、cache write 和 output 映射；
- DeepSeek 官方 cache hit、cache miss 和 output base rate；
- GPT Image 的 text/image input、cached input 和 image output 映射；
- B2B token 使用 `auto`，并关闭跨组重试；
- B2B 内部组不在普通用户可选列表中，且没有 `default` 正向回退。

价格来源固定记录在 `config/glimo-b2b-commercial-policy.json`。每次官方价格变化必须更新 policy version、官方基础价格、New API 映射和自动测试，再重新生成生产快照；不得只改生产 option。

## 2026-07-19 首次快照

首次只读运行发现：目录、channels、DeepSeek 1.10、充值 1:1 和逐模型基础价格均符合批准规则，但生产 `GroupRatio.b2b` 为 `0.3`，而批准值为 `0.1`。因此 GPT/GPT Image 的当前有效扣费是批准客户价的 3 倍。生产已有 2 个内部 B2B 用户、57 个 `auto` token，并存在实际调用；修改倍率会立即影响这些调用。

在 Leo 明确批准生产价格变更和生效时间前，审计器必须保持失败，不能用修改 policy 文件的方式把漂移“改成通过”。
