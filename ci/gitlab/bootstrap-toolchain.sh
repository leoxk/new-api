#!/usr/bin/env bash
set -Eeuo pipefail

GO_VERSION=1.25.1
NODE_VERSION=24.18.0
BUN_VERSION=1.3.14
BUN_LINUX_X64_SHA256=951ee2aee855f08595aeec6225226a298d3fea83a3dcd6465c09cbccdf7e848f
cache_root=${CI_PROJECT_DIR:-$(pwd)}/.cache/toolchains

case $(uname -m) in
  x86_64) tool_arch=amd64; node_arch=x64; bun_arch=x64 ;;
  aarch64|arm64) tool_arch=arm64; node_arch=arm64; bun_arch=aarch64 ;;
  *) echo "unsupported toolchain architecture: $(uname -m)" >&2; return 1 ;;
esac

install -d -m 0755 "$cache_root"

go_root="$cache_root/go-${GO_VERSION}-${tool_arch}"
if [ ! -x "$go_root/bin/go" ]; then
  archive=$(mktemp)
  unpack=$(mktemp -d)
  curl --fail --silent --show-error --location --retry 3 \
    "https://go.dev/dl/go${GO_VERSION}.linux-${tool_arch}.tar.gz" -o "$archive"
  tar -C "$unpack" -xzf "$archive"
  rm -rf "$go_root"
  mv "$unpack/go" "$go_root"
  rm -f "$archive"
  rmdir "$unpack"
fi

node_root="$cache_root/node-v${NODE_VERSION}-linux-${node_arch}"
if [ ! -x "$node_root/bin/node" ]; then
  archive=$(mktemp)
  curl --fail --silent --show-error --location --retry 3 \
    "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${node_arch}.tar.xz" \
    -o "$archive"
  expected=$(curl --fail --silent --show-error --location --retry 3 \
    "https://nodejs.org/dist/v${NODE_VERSION}/SHASUMS256.txt" |
    awk -v file="node-v${NODE_VERSION}-linux-${node_arch}.tar.xz" '$2 == file { print $1 }')
  printf '%s  %s\n' "$expected" "$archive" | sha256sum --check --status
  tar -C "$cache_root" -xJf "$archive"
  rm -f "$archive"
fi

bun_root="$cache_root/bun-v${BUN_VERSION}-${bun_arch}"
if [ ! -x "$bun_root/bun" ]; then
  archive=$(mktemp)
  unpack=$(mktemp -d)
  curl --fail --silent --show-error --location --retry 3 \
    "https://github.com/oven-sh/bun/releases/download/bun-v${BUN_VERSION}/bun-linux-${bun_arch}.zip" \
    -o "$archive"
  if [ "$bun_arch" = x64 ]; then
    printf '%s  %s\n' "$BUN_LINUX_X64_SHA256" "$archive" |
      sha256sum --check --status
  else
    echo "Bun checksum is pinned only for the required linux-x64 runner" >&2
    return 1
  fi
  unzip -q "$archive" -d "$unpack"
  rm -rf "$bun_root"
  install -d -m 0755 "$bun_root"
  install -m 0755 "$unpack/bun-linux-${bun_arch}/bun" "$bun_root/bun"
  rm -rf "$archive" "$unpack"
fi

export PATH="$go_root/bin:$node_root/bin:$bun_root:$PATH"
go version
node --version
bun --version
