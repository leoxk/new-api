#!/usr/bin/env bash
set -Eeuo pipefail

if [ "$#" -ne 5 ]; then
  echo "usage: $0 DEPLOY_PATH IMAGE LOCAL_HEALTH_URL PUBLIC_HEALTH_URL RUNTIME_ENV" >&2
  exit 2
fi

deploy_path=$1
image=$2
local_health_url=$3
public_health_url=$4
runtime_env=$5

cleanup() {
  rm -f "$runtime_env"
}
trap cleanup EXIT

cd "$deploy_path"

old_image=$(docker inspect --format '{{.Config.Image}}' new-api 2>/dev/null || true)
rollback() {
  if [ -z "$old_image" ]; then
    echo "staging deployment failed and no prior image was available for automatic rollback" >&2
    return
  fi
  echo "staging deployment failed; rolling back to $old_image" >&2
  NEW_API_IMAGE="$old_image" docker compose --env-file "$runtime_env" up -d --no-deps new-api
}
trap rollback ERR

NEW_API_IMAGE="$image" docker compose --env-file "$runtime_env" pull new-api
NEW_API_IMAGE="$image" docker compose --env-file "$runtime_env" up -d --no-deps new-api

for attempt in $(seq 1 12); do
  if curl --fail --silent --show-error "$local_health_url" >/dev/null; then
    break
  fi
  if [ "$attempt" -eq 12 ]; then
    echo "local staging health check failed" >&2
    exit 1
  fi
  sleep 5
done

curl --fail --silent --show-error "$public_health_url" >/dev/null
running_image=$(docker inspect --format '{{.Config.Image}}' new-api)
if [ "$running_image" != "$image" ]; then
  echo "running image mismatch: expected $image, got $running_image" >&2
  exit 1
fi

trap - ERR
echo "staging deployment verified: $running_image"
