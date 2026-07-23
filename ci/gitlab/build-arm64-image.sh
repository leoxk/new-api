#!/usr/bin/env bash
set -Eeuo pipefail

: "${CI_REGISTRY_IMAGE:?missing GitLab registry image path}"
: "${CI_COMMIT_SHA:?missing commit SHA}"
: "${IMAGE_TAG:?missing immutable image tag}"
: "${BUILDX_BUILDER:?missing Buildx builder name}"

if ! docker buildx inspect "$BUILDX_BUILDER" >/dev/null 2>&1; then
  docker buildx create --name "$BUILDX_BUILDER" --driver docker-container --use
else
  docker buildx use "$BUILDX_BUILDER"
fi
# Keep the per-runner BuildKit store and also share reusable layers when a job
# lands on the other isolated build runner.
docker buildx inspect --bootstrap | grep -Eq 'Platforms:.*linux/arm64'

printf '%s\n' "$CI_COMMIT_SHA" > VERSION
cache_ref="$CI_REGISTRY_IMAGE:buildcache"
docker buildx build \
  --platform linux/arm64 \
  --cache-from "type=registry,ref=$cache_ref" \
  --cache-to "type=registry,ref=$cache_ref,mode=max,compression=zstd" \
  --push \
  --tag "$IMAGE_TAG" \
  --provenance mode=max \
  --sbom true \
  --metadata-file image-metadata.json \
  .

digest=$(jq -er '.["containerimage.digest"]' image-metadata.json)
[[ "$digest" =~ ^sha256:[a-f0-9]{64}$ ]]
image_ref="${CI_REGISTRY_IMAGE}@${digest}"
manifest=$(docker buildx imagetools inspect "$image_ref")
grep -Eq 'Platform:[[:space:]]+linux/arm64' <<<"$manifest"
if grep -Eq 'Platform:[[:space:]]+linux/amd64' <<<"$manifest"; then
  echo "refusing unexpected amd64 platform in the production ARM64 image" >&2
  exit 1
fi

printf 'IMAGE_DIGEST=%s\nIMAGE_REF=%s\n' "$digest" "$image_ref" > build.env
echo "Built and verified linux/arm64 image $image_ref"
