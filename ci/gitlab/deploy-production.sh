#!/usr/bin/env bash
set -Eeuo pipefail

: "${IMAGE_REF:?missing immutable image reference}"
: "${PRODUCTION_DEPLOY_HOST:?missing production deployment hostname}"
: "${PRODUCTION_DEPLOY_USER:?missing production deployment user}"
: "${PRODUCTION_DEPLOY_SSH_PRIVATE_KEY_FILE:?missing production SSH key file}"
: "${PRODUCTION_DEPLOY_SSH_KNOWN_HOSTS_FILE:?missing production known-hosts file}"
: "${PRODUCTION_PUBLIC_HEALTH_URL:?missing production public health URL}"
: "${PRODUCTION_ORIGIN_HEALTH_URL:?missing production origin health URL}"

export DEPLOY_HOST=$PRODUCTION_DEPLOY_HOST
export DEPLOY_USER=$PRODUCTION_DEPLOY_USER
export DEPLOY_SSH_PRIVATE_KEY_FILE=$PRODUCTION_DEPLOY_SSH_PRIVATE_KEY_FILE
export DEPLOY_SSH_KNOWN_HOSTS_FILE=$PRODUCTION_DEPLOY_SSH_KNOWN_HOSTS_FILE

source ci/gitlab/deploy-lib.sh
prepare_deploy_transport
trap cleanup_deploy_transport EXIT
stage_remote_registry_auth

remote_script="$REMOTE_TMP/deploy-production.sh"
scp -F "$SSH_CONFIG" scripts/deploy-glimo-new-api-production.sh \
  "new-api-target:$remote_script"
ssh -F "$SSH_CONFIG" new-api-target \
  "chmod 0700 '$remote_script'; chmod 0600 '$REMOTE_TMP/docker/config.json'; '$remote_script' '$IMAGE_REF' '$PRODUCTION_PUBLIC_HEALTH_URL' '$PRODUCTION_ORIGIN_HEALTH_URL' '$REMOTE_TMP/docker'; status=\$?; rm -rf '$REMOTE_TMP'; exit \$status"

curl --fail --silent --show-error --retry 5 --retry-all-errors \
  "$PRODUCTION_PUBLIC_HEALTH_URL" >/dev/null
curl --fail --silent --show-error --retry 5 --retry-all-errors \
  "$PRODUCTION_ORIGIN_HEALTH_URL" >/dev/null
echo "Production deployment verified at $IMAGE_REF"
