#!/usr/bin/env bash
set -Eeuo pipefail

if [ "$#" -ne 6 ]; then
  echo "usage: $0 DEPLOY_PATH IMAGE LOCAL_HEALTH_URL PUBLIC_HEALTH_URL RUNTIME_ENV COMPOSE_SOURCE" >&2
  exit 2
fi

deploy_path=$1
image=$2
local_health_url=$3
public_health_url=$4
runtime_env=$5
compose_source=$6
compose_file="$deploy_path/compose.yml"

# The compose file requires NEW_API_IMAGE during every command, including
# inspection commands after startup. Export the requested image as the default;
# rollback can still override it for a single compose invocation.
export NEW_API_IMAGE="$image"

cleanup() {
  rm -f "$runtime_env"
}
trap cleanup EXIT

install -d -m 0755 "$deploy_path"
install -m 0644 "$compose_source" "$compose_file"
cd "$deploy_path"

compose() {
  docker compose --project-name glimo-b2b-staging --file "$compose_file" --env-file "$runtime_env" "$@"
}

old_container=$(compose ps --quiet new-api 2>/dev/null || true)
old_image=""
if [ -n "$old_container" ]; then
  old_image=$(docker inspect --format '{{.Config.Image}}' "$old_container" 2>/dev/null || true)
fi
rollback() {
  if [ -z "$old_image" ]; then
    echo "staging deployment failed and no prior image was available for automatic rollback" >&2
    compose stop new-api 2>/dev/null || true
    return
  fi
  echo "staging deployment failed; rolling back to $old_image" >&2
  NEW_API_IMAGE="$old_image" compose up -d --no-deps new-api
}
trap rollback ERR

NEW_API_IMAGE="$image" compose pull
image_arch=$(docker image inspect --format '{{.Architecture}}' "$image")
host_arch=$(uname -m)
if [ "$host_arch" = "aarch64" ]; then
  host_arch="arm64"
fi
if [ "$image_arch" != "$host_arch" ]; then
  echo "image architecture mismatch: image=$image_arch host=$host_arch" >&2
  exit 1
fi
NEW_API_IMAGE="$image" compose up -d

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
running_container=$(compose ps --quiet new-api)
running_image=$(docker inspect --format '{{.Config.Image}}' "$running_container")
if [ "$running_image" != "$image" ]; then
  echo "running image mismatch: expected $image, got $running_image" >&2
  exit 1
fi

trap - ERR
echo "staging deployment verified: $running_image"
