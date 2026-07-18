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

The promotional row used above is a staging fixture. Root compliance
confirmation is now complete; creation and redemption of a real staging code
remain an explicit acceptance test rather than being bypassed with another
fixture.

## Provider sandbox configuration

On 2026-07-18 the root operator personally confirmed the payment compliance
statement. Stripe and PayPal sandbox credentials were then stored as separate
secrets in the protected `glimo-gateway-staging` GitHub Environment. PR #7
changed the deployment workflow to append them only to the protected ephemeral
runtime env. Deployment run `29625451246` passed all tests, image build, local
health, public health, deployment, and cleanup steps.

Stripe sandbox configuration:

- Dedicated account name: `Glimo Lab Sandbox`
- Legal entity shown by Stripe: `KL Merchandise Limited`
- Time zone: Hong Kong
- Brand and accent colors: Glimo primary blue and cyan
- Successful-payment and refund receipts enabled
- One-time US$1 prepaid-balance unit price; no subscription price or promotion
  code
- Webhook endpoint listens only for the four Checkout Session events handled by
  the application: completed, expired, async payment succeeded, and async
  payment failed

PayPal sandbox configuration:

- App name: `Glimo AI Gateway Sandbox`
- Removed unused Vault, subscriptions, invoicing, payment links, payouts,
  JavaScript SDK, and mobile SDK permissions
- Enabled customer-dispute and transaction-search permissions
- Webhook endpoint listens only for Checkout order approved, the event handled
  by the application

Runtime provider probes returned HTTP 200 from the Stripe price endpoint and
PayPal sandbox OAuth. Both public webhook endpoints returned HTTP 400 for an
unsigned synthetic request, confirming that the endpoints are enabled and
reject invalid signatures. No provider credential or access token is retained
in this evidence directory.

### Merchant identity boundary

Glimo Lab may use the same Hong Kong legal entity as Baby Pro Asia, but each
brand should have a separate merchant account/profile, customer-facing business
details, statement descriptor, support address, webhook credentials, and
reconciliation stream. Customer terms and checkout must state that Glimo Lab is
operated by `KL Merchandise Limited`.

The currently connected PayPal business account displays `BABY PRO INC.` and a
Philippines sandbox region. It is approved for sandbox integration testing only.
Do not use it for Glimo Lab production collections unless the merchant of record
and intercompany arrangement are separately reviewed and approved. The preferred
production path is a separate PayPal Business account for `KL Merchandise
Limited`, with Glimo Lab as the customer-facing business name.

### Stripe-only scope update

On 2026-07-18 Leo narrowed the Glimo Lab commercial payment scope to Stripe
only. The PayPal sandbox notes above remain as historical evidence of the
earlier evaluation; they are not an active rollout requirement. The staging
Compose and GitHub Actions workflow no longer inject PayPal credentials, so the
optional PayPal endpoints remain disabled. Production must expose only Stripe
Checkout and Stripe refund/reconciliation instructions.

### Stripe-only deployment and provider E2E

PR `#9` removed PayPal credential injection and customer-facing PayPal refund
wording. Deployment run `29638331255` passed the full Go test suite, frontend
type-check/build, ARM64 image build, deployment, local/public health checks,
and ephemeral credential cleanup. Browser verification showed Stripe as the
only payment method; PayPal was absent.

The first post-deployment visual check found that preset cards still displayed
the generic Epay rate (`$10` balance / `Pay 73`) even though Stripe's unit price
and amount API were both `1.0`. PR `#10` made preset cards use
`stripe_unit_price` whenever Stripe is the active/default method. Deployment
run `29638720965` passed all jobs. Browser verification then showed `10/10`,
`25/25`, `50/50`, `100/100`, `250/250`, and `500/500`.

Real Stripe Test Mode provider flows were exercised on 2026-07-18
(Asia/Manila):

| Scenario | Provider/system evidence | Result |
|---|---|---|
| Hosted Checkout success | `$10.00 USD`, test Visa ending `4242`, local order `ref_0fc006502249bc453cd2aa01b8b190958db06552` | Passed; system balance increased from `$200` to `$210` exactly once |
| Webhook delivery | `checkout.session.completed`, destination response `200` | Passed |
| Duplicate webhook | The successful delivery was resent from Stripe Workbench | Passed; destination returned success and system balance did not increase again |
| Customer cancellation | A second hosted Checkout used the Stripe back link before payment | Passed; returned to Wallet and balance remained `$200` after the earlier refund |
| Card decline | Stripe decline test card produced `Your credit card was declined` | Passed; no balance increase |
| Provider refund | Full `$10.00 USD` refund, refund ID `re_3TuUQm2XeJ21Aojq1qKEKxNl`, reason `Requested by customer` | Passed; Stripe showed `Refunded` and completed refund events |
| System refund record | Admin recorded the completed provider refund against the matching local order | Passed; provider refund ID/reason were retained and system balance returned from `$210` to `$200` |

The Stripe test transaction also exposed the expected settlement dimensions:
the `$10.00 USD` test charge converted to `$78.40 HKD`, showed `$6.98 HKD` in
processor fees, and a `$71.42 HKD` net amount. These settlement values are
processor-side reconciliation data and do not change the customer's `1:1` USD
Recharge Balance.

Only card payments are enabled in the current Stripe product, so a delayed
payment-method success/failure event is not applicable to the enabled payment
surface. No Stripe live credential or real payment was used.

## Screenshots

- `customer-wallet-desktop.png`
- `customer-wallet-mobile.png`
- `customer-api-keys-desktop.png`
- `admin-users-desktop.png`
- `admin-order-refund-history.png`
- `admin-refund-form.png`
- `admin-payment-compliance-gate.png`

## Remaining external gates

- Enable Chrome extension access to file URLs, then upload the approved Glimo
  icon and horizontal logo to the Stripe sandbox branding page.
- Create and redeem a real staging redemption code.
- Repeat the successful Checkout once under the controlled `b2btest` customer
  instead of the root operator, and archive the final customer-facing receipt
  view.
- Do not connect Stripe live credentials or process a real payment until public
  terms, final pricing, staging evidence, and canary approval are complete.
- No production merchant account, credential, payment, refund, price, balance,
  user, or DeepSeek channel was changed.
