# GitLab main repository and CI/CD migration

This repository is the source of truth candidate for the Glimo Lab New API
service at `gitlab.glimolab.com/glimolab/new-api`. The migration intentionally
covers only New API source, validation, image publication, staging operations,
deployment, and acceptance. No unrelated service source is copied into this
repository or built by this pipeline.

The existing GitHub production workflow must remain enabled until a GitLab
merge request, protected pipeline, registry image, staging deployment,
production deployment, and production acceptance have all succeeded.

## Pipeline contract

The root `.gitlab-ci.yml` provides these gates:

1. `verify:new-api` installs pinned Go, Node, and Bun toolchains in the Build
   Runner cache, runs the frontend type-check/build, the Glimo B2B policy audit,
   and all Go tests.
2. `build:arm64` uses a stable, per-Runner Buildx `docker-container` builder.
   Its BuildKit content store is the persistent local build cache; cache layers
   are never imported from or exported to the Registry. The job explicitly
   builds `linux/arm64`, pushes the release image to
   `registry.glimolab.com`, verifies the remote manifest, and exports an
   immutable `IMAGE_REF` containing the registry digest.
3. `scan:container` blocks on unfixed critical vulnerabilities.
4. `deploy:staging` is a manual protected-main job. It deploys the digest to
   the isolated payment staging Compose project and verifies local and public
   health.
5. `deploy:production` is a separate manual protected-main job. It starts the
   candidate image against the existing database first, waits for migration
   and health completion, replaces only the `new-api` container, verifies that
   every other container kept the same ID/running/restart state, and rolls back
   automatically if local, origin, public, or immutable-image verification
   fails.
6. The acceptance jobs independently repeat the public staging check and both
   production checks after their corresponding deploy job.

Build jobs use only runner tags:

```text
solstice-build, build-only, linux-x64
```

Deploy and acceptance jobs use only:

```text
solstice-deploy, deploy-only, linux-x64
```

The production runner must be configured to accept only protected branches and
protected tags. Protect `main`, require merge requests, and protect the
`production` environment with Leo as an allowed deployer before the first
production pipeline.

## Required GitLab variables

Set variables at project scope. Every credential must be `Protected`; secrets
that meet GitLab's masking rules must also be `Masked`. Variables identified as
File must use GitLab's File type so the job receives only a temporary pathname.
Do not copy any value into Git, pipeline YAML, runner configuration, or a
persistent target-host file.

| Variable | Type | Environment scope | Protection |
|---|---|---|---|
| `CF_ACCESS_CLIENT_ID` | Variable | `staging` and `production` | Protected + Masked |
| `CF_ACCESS_CLIENT_SECRET` | Variable | `staging` and `production` | Protected + Masked |
| `STAGING_DEPLOY_HOST` | Variable | `staging` | Protected |
| `STAGING_DEPLOY_USER` | Variable | `staging` | Protected |
| `STAGING_DEPLOY_PATH` | Variable | `staging` | Protected |
| `STAGING_DEPLOY_SSH_PRIVATE_KEY_FILE` | File | `staging` | Protected |
| `STAGING_DEPLOY_SSH_KNOWN_HOSTS_FILE` | File | `staging` | Protected |
| `STAGING_RUNTIME_ENV_FILE` | File | `staging` | Protected |
| `STAGING_LOCAL_HEALTH_URL` | Variable | `staging` | Protected |
| `STAGING_PUBLIC_HEALTH_URL` | Variable | `staging` | Protected |
| `STAGING_ADMIN_ACCESS_TOKEN` | Variable | `staging` | Protected + Masked |
| `STAGING_CUSTOMER_PASSWORD` | Variable | `staging` | Protected + Masked |
| `STAGING_B2B_TEST_API_KEY` | Variable | `staging` | Protected + Masked |
| `PRODUCTION_DEPLOY_HOST` | Variable | `production` | Protected |
| `PRODUCTION_DEPLOY_USER` | Variable | `production` | Protected |
| `PRODUCTION_DEPLOY_SSH_PRIVATE_KEY_FILE` | File | `production` | Protected |
| `PRODUCTION_DEPLOY_SSH_KNOWN_HOSTS_FILE` | File | `production` | Protected |
| `PRODUCTION_PUBLIC_HEALTH_URL` | Variable | `production` | Protected |
| `PRODUCTION_ORIGIN_HEALTH_URL` | Variable | `production` | Protected |

`CF_ACCESS_CLIENT_ID` must retain its `.access` suffix. The deployment hostname
must be a dedicated `deploy-*` Cloudflare Tunnel hostname protected by a
Service Auth policy. The known-hosts File variable must contain the pinned SSH
host key; jobs never run `ssh-keyscan`.

The staging runtime File variable contains only the isolated staging database,
Redis, session/crypto, and Stripe Test Mode settings described in
`deploy/glimo-b2b-staging/README.md`. The deployment copies it to `/dev/shm`,
and cleanup removes it even after failure. Production does not receive a
runtime-env variable: the replacement inherits the current New API container's
environment through a protected `/dev/shm` file that exists only for the
deployment process.

GitLab automatically supplies `CI_REGISTRY`, `CI_REGISTRY_IMAGE`,
`CI_REGISTRY_USER`, and `CI_REGISTRY_PASSWORD`. The job writes Docker auth only
under its temporary directory and the remote `/dev/shm` handoff directory.

## Manual staging operations

Start a `main` pipeline from the GitLab UI, set `STAGING_OPERATION`, and then
run `staging:operation`. Allowed values are:

- `sync_test_customer`
- `record_completed_stripe_refund`
- `sync_b2b_catalog_policy`
- `verify_b2b_catalog_gate`

The refund operation additionally requires the one-run pipeline variables
`STAGING_TRADE_NO`, `STAGING_PROVIDER_REFUND_ID`, `STAGING_REFUNDED_MONEY`, and
`STAGING_REFUND_REASON`. It validates the controlled customer and successful
unrefunded Stripe Sandbox order before recording the completed external refund.

## Cutover and rollback

1. Push the complete Git history to `glimolab/new-api` without changing the
   GitHub remote.
2. Protect `main`, `production`, and the Deploy Runner; add the variables above.
3. Open a GitLab merge request and require `verify`, ARM64 build, manifest
   verification, and critical scan to pass.
4. Merge through GitLab, run `deploy:staging`, then capture the pipeline URL,
   runner names, registry digest, local/public staging health, and visual B2B
   payment acceptance.
5. With Leo's explicit production approval, run `deploy:production`. Capture
   the digest, migration-canary result, unchanged-container check, local New API
   health, origin health, and public health.
6. Keep the GitHub production workflow enabled during an observation window.
   Only after the GitLab evidence is accepted may it be disabled in a separate,
   explicitly approved change.

Production rollback is automatic while the job is running: the old container
is retained under a temporary name until all checks pass. A failed check removes
the replacement, restores the old name, and starts the previous container. For
a later rollback, rerun a protected pipeline with the previously accepted Git
commit so the registry digest remains immutable and auditable.

Komodo remains monitoring-only and is not a pipeline trigger or deployer.
