#!/usr/bin/env bash
set -Eeuo pipefail

: "${IMAGE_REF:?missing immutable image reference}"
: "${STAGING_DEPLOY_HOST:?missing staging deployment hostname}"
: "${STAGING_DEPLOY_USER:?missing staging deployment user}"
: "${STAGING_DEPLOY_PATH:?missing staging deployment path}"
: "${STAGING_DEPLOY_SSH_PRIVATE_KEY_FILE:?missing staging SSH key file}"
: "${STAGING_DEPLOY_SSH_KNOWN_HOSTS_FILE:?missing staging known-hosts file}"
: "${STAGING_RUNTIME_ENV_FILE:?missing staging runtime environment file}"
: "${STAGING_LOCAL_HEALTH_URL:?missing staging local health URL}"
: "${STAGING_PUBLIC_HEALTH_URL:?missing staging public health URL}"

export DEPLOY_HOST=$STAGING_DEPLOY_HOST
export DEPLOY_USER=$STAGING_DEPLOY_USER
export DEPLOY_SSH_PRIVATE_KEY_FILE=$STAGING_DEPLOY_SSH_PRIVATE_KEY_FILE
export DEPLOY_SSH_KNOWN_HOSTS_FILE=$STAGING_DEPLOY_SSH_KNOWN_HOSTS_FILE

source ci/gitlab/deploy-lib.sh
prepare_deploy_transport
trap cleanup_deploy_transport EXIT
stage_remote_registry_auth

remote_script="$REMOTE_TMP/deploy-staging.sh"
remote_compose="$REMOTE_TMP/compose.yml"
remote_env="$REMOTE_TMP/runtime.env"
scp -F "$SSH_CONFIG" scripts/deploy-glimo-b2b-staging.sh \
  "new-api-target:$remote_script"
scp -F "$SSH_CONFIG" deploy/glimo-b2b-staging/compose.yml \
  "new-api-target:$remote_compose"
scp -F "$SSH_CONFIG" "$STAGING_RUNTIME_ENV_FILE" \
  "new-api-target:$remote_env"

ssh -F "$SSH_CONFIG" new-api-target \
  "chmod 0700 '$remote_script'; chmod 0600 '$remote_env' '$REMOTE_TMP/docker/config.json'; '$remote_script' '$STAGING_DEPLOY_PATH' '$IMAGE_REF' '$STAGING_LOCAL_HEALTH_URL' '$STAGING_PUBLIC_HEALTH_URL' '$remote_env' '$remote_compose' '$REMOTE_TMP/docker'; status=\$?; rm -rf '$REMOTE_TMP'; exit \$status"

curl --fail --silent --show-error --retry 5 --retry-all-errors \
  "$STAGING_PUBLIC_HEALTH_URL" >/dev/null
echo "Staging deployment verified at $IMAGE_REF"
