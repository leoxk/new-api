# Glimo B2B payment staging

This Compose project is intentionally limited to payment and wallet testing on
the existing Singapore VPS. It must never connect to the production New API
database, Redis instance, Docker network, or named volumes.

## Isolation contract

- Compose project: `glimo-b2b-staging`
- Local bind: `127.0.0.1:3100`
- Network: `glimo-b2b-staging-network`
- Volumes:
  - `glimo-b2b-staging-new-api-data`
  - `glimo-b2b-staging-new-api-logs`
  - `glimo-b2b-staging-postgres-data`
  - `glimo-b2b-staging-redis-data`
- Database and Redis are private to the staging network and publish no host ports.
- No production sidecars or production volumes are mounted.
- Payment credentials must be Stripe Test Mode only. Glimo Lab does not inject
  PayPal credentials, so the existing optional PayPal routes remain disabled.

The public staging hostname routes through the existing VPS Cloudflare Tunnel
to `http://localhost:3100`. GitHub Actions deploys directly to the public VPS
SSH hostname with a project-specific SSH key, a pinned host key, and strict host
key checking. A Cloudflare Access Service Token is not required for deployment.

After each deployment, the workflow uses the protected staging admin token to
clear `general_setting.docs_link`. Signed-in customers are therefore routed to
the built-in authenticated `/docs` guide instead of the upstream New API site.
The token is used ephemerally by GitHub Actions and is not copied to the host.

## Runtime variables

The protected GitHub Environment secret `STAGING_RUNTIME_ENV` supplies:

```dotenv
POSTGRES_DB=glimo_b2b_staging
POSTGRES_USER=glimo_b2b_staging
POSTGRES_PASSWORD=...
REDIS_PASSWORD=...
SESSION_SECRET=...
CRYPTO_SECRET=...
STRIPE_API_SECRET=...
STRIPE_WEBHOOK_SECRET=...
STRIPE_PRICE_ID=...
```

The workflow copies this file to `/dev/shm` only for the duration of deployment
and removes it on exit. No runtime secret belongs in this directory or Git.

## Controlled customer credential sync

The manual `sync_test_customer` workflow option resets only staging user ID 3
after first verifying that it is the expected `b2btest` user in group `b2b`.
The password comes from the protected `STAGING_CUSTOMER_PASSWORD` Environment
Secret, is verified through the public login endpoint, and is never printed or
written to the repository or staging host. This option does not deploy an image
and cannot target production.
