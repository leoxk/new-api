#!/usr/bin/env bash
set -Eeuo pipefail

if [ "$#" -ne 4 ]; then
  echo "usage: $0 IMAGE PUBLIC_HEALTH_URL ORIGIN_HEALTH_URL DOCKER_CONFIG" >&2
  exit 2
fi

image=$1
public_health_url=$2
origin_health_url=$3
docker_config=$4
current=new-api
candidate=new-api-gitlab-canary
backup=new-api-gitlab-rollback
runtime_env=$(mktemp /dev/shm/new-api-runtime.XXXXXX)
before_state=$(mktemp /dev/shm/new-api-containers-before.XXXXXX)
after_state=$(mktemp /dev/shm/new-api-containers-after.XXXXXX)
switched=false

export DOCKER_CONFIG=$docker_config

snapshot_other_containers() {
  docker ps -a --format '{{.Names}}' |
    grep -Ev "^(${current}|${candidate}|${backup})$" |
    sort |
    while IFS= read -r name; do
      docker inspect --format '{{.Name}} {{.Id}} {{.State.Running}} {{.RestartCount}}' "$name"
    done
}

rollback() {
  status=$?
  if [ "$status" -ne 0 ] && [ "$switched" = true ]; then
    echo "production verification failed; restoring the previous New API container" >&2
    docker rm -f "$current" >/dev/null 2>&1 || true
    if docker inspect "$backup" >/dev/null 2>&1; then
      docker rename "$backup" "$current"
      docker start "$current" >/dev/null
    fi
  fi
  docker rm -f "$candidate" >/dev/null 2>&1 || true
  rm -f "$runtime_env" "$before_state" "$after_state"
  exit "$status"
}
trap rollback EXIT

[[ "$image" =~ @sha256:[a-f0-9]{64}$ ]] || {
  echo "production image must be an immutable digest reference" >&2
  exit 1
}
docker inspect "$current" >/dev/null
if docker inspect "$backup" >/dev/null 2>&1; then
  echo "refusing deployment while a previous rollback container exists" >&2
  exit 1
fi

snapshot_other_containers > "$before_state"
docker pull "$image" >/dev/null
image_arch=$(docker image inspect --format '{{.Architecture}}' "$image")
host_arch=$(uname -m)
[ "$host_arch" != aarch64 ] || host_arch=arm64
if [ "$image_arch" != arm64 ] || [ "$host_arch" != arm64 ]; then
  echo "production requires an ARM64 host and ARM64 image" >&2
  exit 1
fi

docker inspect "$current" | jq -r '.[0].Config.Env[]' > "$runtime_env"
chmod 0600 "$runtime_env"
mapfile -t networks < <(docker inspect "$current" |
  jq -r '.[0].NetworkSettings.Networks | keys[]')
[ "${#networks[@]}" -gt 0 ]

docker rm -f "$candidate" >/dev/null 2>&1 || true
docker run -d --name "$candidate" \
  --network "${networks[0]}" \
  --env-file "$runtime_env" \
  "$image" --log-dir /tmp/logs >/dev/null
for attempt in $(seq 1 18); do
  if docker exec "$candidate" wget --quiet --output-document=- \
    http://127.0.0.1:3000/api/status 2>/dev/null |
    grep -q '"success"[[:space:]]*:[[:space:]]*true'; then
    break
  fi
  if [ "$attempt" -eq 18 ]; then
    echo "migration canary did not become healthy" >&2
    exit 1
  fi
  sleep 5
done
docker rm -f "$candidate" >/dev/null

mount_args=()
while IFS=$'\t' read -r type source destination rw; do
  mount="type=$type,src=$source,dst=$destination"
  [ "$rw" = true ] || mount+=",readonly"
  mount_args+=(--mount "$mount")
done < <(docker inspect "$current" | jq -r '
  .[0].Mounts[] | [.Type, (.Name // .Source), .Destination, .RW] | @tsv
')

label_args=()
while IFS=$'\t' read -r key value; do
  label_args+=(--label "$key=$value")
done < <(docker inspect "$current" | jq -r '
  .[0].Config.Labels // {} | to_entries[] | [.key, .value] | @tsv
')

port_args=()
while IFS=$'\t' read -r container_port host_ip host_port; do
  port_args+=(--publish "${host_ip}:${host_port}:${container_port}")
done < <(docker inspect "$current" | jq -r '
  .[0].HostConfig.PortBindings // {} | to_entries[] as $port |
  $port.value[] | [$port.key, .HostIp, .HostPort] | @tsv
')

memory=$(docker inspect --format '{{.HostConfig.Memory}}' "$current")
memory_reservation=$(docker inspect --format '{{.HostConfig.MemoryReservation}}' "$current")
nano_cpus=$(docker inspect --format '{{.HostConfig.NanoCpus}}' "$current")
restart=$(docker inspect --format '{{.HostConfig.RestartPolicy.Name}}' "$current")
run_args=(--name "$current" --network "${networks[0]}" --env-file "$runtime_env")
[ "$memory" -le 0 ] || run_args+=(--memory "$memory")
[ "$memory_reservation" -le 0 ] || run_args+=(--memory-reservation "$memory_reservation")
if [ "$nano_cpus" -gt 0 ]; then
  cpus=$(awk -v value="$nano_cpus" 'BEGIN { printf "%.9f", value / 1000000000 }')
  run_args+=(--cpus "$cpus")
fi
[ -z "$restart" ] || run_args+=(--restart "$restart")
run_args+=("${mount_args[@]}")
run_args+=("${label_args[@]}")
run_args+=("${port_args[@]}")

health_test=$(docker inspect "$current" | jq -r '
  .[0].Config.Healthcheck.Test // [] |
  if .[0] == "CMD-SHELL" then .[1] // "" else "" end
')
if [ -n "$health_test" ]; then
  health_interval=$(docker inspect "$current" | jq -r '.[0].Config.Healthcheck.Interval // 0')
  health_timeout=$(docker inspect "$current" | jq -r '.[0].Config.Healthcheck.Timeout // 0')
  health_retries=$(docker inspect "$current" | jq -r '.[0].Config.Healthcheck.Retries // 0')
  health_start_period=$(docker inspect "$current" | jq -r '.[0].Config.Healthcheck.StartPeriod // 0')
  run_args+=(--health-cmd "$health_test")
  [ "$health_interval" -le 0 ] || run_args+=(--health-interval "${health_interval}ns")
  [ "$health_timeout" -le 0 ] || run_args+=(--health-timeout "${health_timeout}ns")
  [ "$health_retries" -le 0 ] || run_args+=(--health-retries "$health_retries")
  [ "$health_start_period" -le 0 ] || run_args+=(--health-start-period "${health_start_period}ns")
fi

docker stop --time 30 "$current" >/dev/null
docker rename "$current" "$backup"
switched=true
docker run -d "${run_args[@]}" "$image" --log-dir /app/logs >/dev/null
for network in "${networks[@]:1}"; do
  docker network connect "$network" "$current"
done

for attempt in $(seq 1 18); do
  if docker exec "$current" wget --quiet --output-document=- \
    http://127.0.0.1:3000/api/status 2>/dev/null |
    grep -q '"success"[[:space:]]*:[[:space:]]*true'; then
    break
  fi
  if [ "$attempt" -eq 18 ]; then
    echo "replacement New API container did not become healthy" >&2
    exit 1
  fi
  sleep 5
done

running_image=$(docker inspect --format '{{.Config.Image}}' "$current")
[ "$running_image" = "$image" ] || {
  echo "running image does not match the requested digest" >&2
  exit 1
}
if docker inspect new-api-origin-proxy >/dev/null 2>&1; then
  docker exec new-api-origin-proxy nginx -t
  docker exec new-api-origin-proxy nginx -s reload
fi
curl --fail --silent --show-error --retry 5 --retry-all-errors \
  "$public_health_url" >/dev/null
curl --fail --silent --show-error --retry 5 --retry-all-errors \
  "$origin_health_url" >/dev/null

snapshot_other_containers > "$after_state"
if ! cmp --silent "$before_state" "$after_state"; then
  echo "a non-New-API container changed during deployment" >&2
  diff --unified "$before_state" "$after_state" >&2 || true
  exit 1
fi

docker rm "$backup" >/dev/null
switched=false
echo "production New API deployment verified: $running_image"
