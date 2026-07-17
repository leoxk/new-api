# Glimo B2B payment staging evidence

Date: 2026-07-17 (Asia/Manila)

## Deployment

- Repository: `leoxk/new-api`
- Initial feature PR: `#1`
- First-deploy fix PR: `#2`
- Successful deployment run: `29590873241`
- Deployed image: `ghcr.io/leoxk/new-api:staging-2da2193cef36f625534b5136b169c241469372e1`
- Final image build/deployment run: `29595606051`
- Current restore run: `29596336428`
- Current image: `ghcr.io/leoxk/new-api:staging-dccda5cbff4e193aabbc6c7db2ad66e7c375acf3`
- Image platform: `linux/arm64`
- Local health: `http://127.0.0.1:3100/api/status` returned HTTP 200.
- Public health: `https://staging-llm.glimolab.com/api/status` returned HTTP 200.

The first deployment exposed two deployment-only defects: the container health
check sent `HEAD` to a GET-only endpoint, and the final Compose inspection did
not retain `NEW_API_IMAGE`. Both were fixed through PR #2 and the complete
workflow was rerun successfully.

### Secure-session recovery and rollback exercise

Deployment run `29593744121` failed after `SESSION_COOKIE_SECURE=true` was
enabled without the required `SESSION_COOKIE_TRUSTED_URL`. Container logs
showed the exact startup error, while PostgreSQL and Redis remained healthy and
their volumes retained all staging data. PR #4 added the single trusted HTTPS
origin `https://staging-llm.glimolab.com`; recovery run `29594711437` then
passed the complete test, image, deployment, local-health, and public-health
jobs.

The failure also proved that an explicit `exit 1` in the deployment script did
not invoke its Bash `ERR` trap. PR #5 added an explicit abort-and-rollback path
for architecture, local-health, and running-image validation failures. Final
deployment run `29595606051` passed.

The GitHub Actions `rollback_image` path was then exercised in both directions:

| Action | Workflow run | Result |
|---|---:|---|
| Roll back to `staging-2da2193cef36f625534b5136b169c241469372e1` | `29596142681` | Passed; local/public health 200 |
| Restore `staging-dccda5cbff4e193aabbc6c7db2ad66e7c375acf3` | `29596336428` | Passed; local/public health 200 |

After both transitions, the staging database still contained three controlled
users and two top-up orders. The `b2btest` wallet remained at total 17,500,000
quota, recharge 17,500,000 quota, and promotional zero; the recorded US$40
refund and provider refund ID `stg-test-refund-001` remained intact. The four
staging volumes and the isolated `glimo-b2b-staging-network` were unchanged,
and no `glimo-b2b-*` deployment files remained in `/dev/shm` or `/tmp`.

## Isolation

- Compose project: `glimo-b2b-staging`
- Containers: New API, PostgreSQL 15, Redis 7 only
- Network: `glimo-b2b-staging-network` only
- Host bind: `127.0.0.1:3100`
- PostgreSQL and Redis expose no host ports.
- Volumes:
  - `glimo-b2b-staging-new-api-data`
  - `glimo-b2b-staging-new-api-logs`
  - `glimo-b2b-staging-postgres-data`
  - `glimo-b2b-staging-redis-data`
- Production New API, PostgreSQL, and Redis remained on
  `vps-oci-sgp-glimolab-llm_llm-network`; no network or volume overlap was
  observed.
- Deployment runtime files were absent from `/dev/shm` and `/tmp` after the
  workflow completed.

## Controlled accounts

- Root operator: `stgadmin`
- Admin operator: `stgoperator`
- Customer: `b2btest`, group `b2b`, initial quota zero
- Customer API key: `B2B staging auto`, group `auto`

Passwords, the root management token, and the customer API key are stored only
in the protected `glimo-gateway-staging` GitHub Environment Secrets. No secret
is included in this evidence directory.

## Wallet and refund results

Quota conversion during this test was `500000 quota = US$1`.

| Stage | Total | Recharge | Promotional | Result |
|---|---:|---:|---:|---|
| Initial | $0 | $0 | $0 | Passed |
| $100 cash completion | $100 | $100 | $0 | Passed; duplicate completion remained idempotent |
| Add $25 promotional fixture | $125 | $100 | $25 | Passed |
| Simulate $30 usage | $95 | $95 | $0 | Passed; promotion was consumed before cash |
| Record $40 completed refund | $55 | $55 | $0 | Passed |
| Repeat same refund | unchanged | unchanged | unchanged | Rejected as already recorded |
| Attempt $50 refund with only $35 refundable | $35 | $35 | $0 | Rejected; balance and order remained unchanged |

The successful refund retained provider refund ID
`stg-test-refund-001`, amount `40.00`, reason, operator, request path, and audit
records. The refund UI explicitly states that it records a refund already
completed in Stripe or PayPal and does not send money.

The promotional row used above is a staging fixture. The actual redemption-code
creation and redemption endpoints remain locked by New API's root compliance
confirmation and have not been bypassed.

## Screenshots

- `customer-wallet-desktop.png`
- `customer-wallet-mobile.png`
- `customer-api-keys-desktop.png`
- `admin-users-desktop.png`
- `admin-order-refund-history.png`
- `admin-refund-form.png`
- `admin-payment-compliance-gate.png`

## Remaining external gates

- A root administrator must personally review and confirm the New API payment
  compliance statements in the staging dashboard. This unlocks redemption-code
  and payment testing.
- Stripe Test Mode is currently at the account login page.
- PayPal Developer is currently at the account password page.
- Sandbox credentials must be stored in the GitHub staging Environment and
  deployed through GitHub Actions before provider end-to-end testing.
- No production merchant account, credential, payment, refund, price, balance,
  user, or DeepSeek channel was changed.
