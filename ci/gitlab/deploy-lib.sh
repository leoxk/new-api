#!/usr/bin/env bash

prepare_deploy_transport() {
  : "${DEPLOY_HOST:?missing deployment hostname}"
  : "${DEPLOY_USER:?missing deployment user}"
  : "${DEPLOY_SSH_PRIVATE_KEY_FILE:?missing SSH private-key file variable}"
  : "${DEPLOY_SSH_KNOWN_HOSTS_FILE:?missing pinned known-hosts file variable}"
  : "${CF_ACCESS_CLIENT_ID:?missing Cloudflare Access client ID}"
  : "${CF_ACCESS_CLIENT_SECRET:?missing Cloudflare Access client secret}"
  : "${CLOUDFLARED_VERSION:?missing pinned cloudflared version}"
  : "${CLOUDFLARED_LINUX_AMD64_SHA256:?missing cloudflared checksum}"

  DEPLOY_TMP=${CI_PROJECT_DIR:?}/.tmp/deploy-${CI_JOB_ID:?}
  install -d -m 0700 "$DEPLOY_TMP/bin" "$DEPLOY_TMP/ssh" "$DEPLOY_TMP/docker"

  cloudflared="$DEPLOY_TMP/bin/cloudflared"
  curl --fail --silent --show-error --location --retry 3 \
    "https://github.com/cloudflare/cloudflared/releases/download/${CLOUDFLARED_VERSION}/cloudflared-linux-amd64" \
    -o "$cloudflared"
  printf '%s  %s\n' "$CLOUDFLARED_LINUX_AMD64_SHA256" "$cloudflared" |
    sha256sum --check --status
  chmod 0755 "$cloudflared"

  install -m 0600 "$DEPLOY_SSH_PRIVATE_KEY_FILE" "$DEPLOY_TMP/ssh/key"
  install -m 0600 "$DEPLOY_SSH_KNOWN_HOSTS_FILE" "$DEPLOY_TMP/ssh/known_hosts"
  cat > "$DEPLOY_TMP/ssh/config" <<EOF
Host new-api-target
  HostName $DEPLOY_HOST
  User $DEPLOY_USER
  IdentityFile $DEPLOY_TMP/ssh/key
  UserKnownHostsFile $DEPLOY_TMP/ssh/known_hosts
  ProxyCommand $cloudflared access ssh --hostname %h
  BatchMode yes
  ConnectTimeout 20
  ConnectionAttempts 3
  ServerAliveInterval 15
  ServerAliveCountMax 3
  StrictHostKeyChecking yes
EOF
  chmod 0600 "$DEPLOY_TMP/ssh/config"

  export TUNNEL_SERVICE_TOKEN_ID=$CF_ACCESS_CLIENT_ID
  export TUNNEL_SERVICE_TOKEN_SECRET=$CF_ACCESS_CLIENT_SECRET
  export SSH_CONFIG=$DEPLOY_TMP/ssh/config

  printf '%s' "$CI_REGISTRY_PASSWORD" |
    docker --config "$DEPLOY_TMP/docker" login "$CI_REGISTRY" \
      -u "$CI_REGISTRY_USER" --password-stdin >/dev/null
}

cleanup_deploy_transport() {
  status=$?
  if [[ -n ${REMOTE_TMP:-} && -n ${SSH_CONFIG:-} ]]; then
    ssh -F "$SSH_CONFIG" new-api-target "rm -rf '$REMOTE_TMP'" >/dev/null 2>&1 || true
  fi
  rm -rf "${DEPLOY_TMP:-/nonexistent}"
  return "$status"
}

stage_remote_registry_auth() {
  REMOTE_TMP="/dev/shm/new-api-gitlab-${CI_PIPELINE_ID}-${CI_JOB_ID}"
  ssh -F "$SSH_CONFIG" new-api-target \
    "install -d -m 0700 '$REMOTE_TMP/docker'"
  scp -F "$SSH_CONFIG" "$DEPLOY_TMP/docker/config.json" \
    "new-api-target:$REMOTE_TMP/docker/config.json"
}
