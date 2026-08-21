#!/usr/bin/env bash
set -euo pipefail

IMAGE_NAME="${1:-artifact-resolver-eval}"
DOCKER_PLATFORM="${2:-linux/amd64}"

docker buildx build \
  --load \
  --platform "${DOCKER_PLATFORM}" \
  -f benzhi.Dockerfile \
  -t "${IMAGE_NAME}:latest" \
  .
