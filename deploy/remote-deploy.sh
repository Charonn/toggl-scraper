#!/usr/bin/env bash
set -euo pipefail

# Usage: remote-deploy.sh <image> <env_file> [container_name]
# Optionally expects GHCR_USERNAME and GHCR_TOKEN env vars to be set for private images.
# Optional env vars:
#   HTTP_PORT (default: 8085)
#   APP_ARGS (default: "--http=:${HTTP_PORT}")

IMAGE=${1:?image ref required}
ENV_FILE=${2:?env file path required}
NAME=${3:-toggl-scraper}
HTTP_PORT=${HTTP_PORT:-8085}
APP_ARGS=${APP_ARGS:---http=:$HTTP_PORT}

if [[ -n "${GHCR_USERNAME:-}" && -n "${GHCR_TOKEN:-}" ]]; then
  echo "$GHCR_TOKEN" | docker login ghcr.io -u "$GHCR_USERNAME" --password-stdin
fi

docker pull "$IMAGE"
docker rm -f "$NAME" || true
docker run -d --restart unless-stopped \
  --name "$NAME" \
  --env-file "$ENV_FILE" \
  -p ${HTTP_PORT}:${HTTP_PORT} \
  "$IMAGE" ${APP_ARGS}

echo "Deployed $IMAGE as container $NAME using env $ENV_FILE"
