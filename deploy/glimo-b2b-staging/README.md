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
- Payment credentials must be Stripe Test Mode and PayPal Sandbox only.

The public staging hostname routes through the existing VPS Cloudflare Tunnel
to `http://localhost:3100`. GitHub Actions deploys directly to the public VPS
SSH hostname with a project-specific SSH key, a pinned host key, and strict host
key checking. A Cloudflare Access Service Token is not required for deployment.

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
PAYPAL_MODE=sandbox
PAYPAL_CLIENT_ID=...
PAYPAL_CLIENT_SECRET=...
PAYPAL_WEBHOOK_ID=...
```

The workflow copies this file to `/dev/shm` only for the duration of deployment
and removes it on exit. No runtime secret belongs in this directory or Git.
