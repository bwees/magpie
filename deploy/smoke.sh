#!/usr/bin/env bash
# Starts compose.smoke.yaml from the images `mise run release:snapshot` loaded, runs the smoke test
# against it and tears it down. Snapshot images are tagged per platform: latest-<arch>.
set -euo pipefail

deploy_dir="$(cd "$(dirname "$0")" && pwd)"
compose_file="$deploy_dir/compose.smoke.yaml"
server_arch="$(docker version --format '{{.Server.Arch}}')"

export MAGPIE_IMAGE="${MAGPIE_IMAGE:-ghcr.io/bwees/magpie:latest-$server_arch}"
export MAGPIE_AGENT_IMAGE="${MAGPIE_AGENT_IMAGE:-ghcr.io/bwees/magpie-agent:latest-amd64}"
export MAGPIE_SMOKE_PORT="${MAGPIE_SMOKE_PORT:-3931}"

cleanup() {
  local status=$?
  if [ "$status" -ne 0 ]; then
    docker compose -f "$compose_file" logs --no-color || true
  fi
  docker compose -f "$compose_file" down --volumes --remove-orphans
}
trap cleanup EXIT

docker compose -f "$compose_file" up --detach --wait

cd "$deploy_dir/../server"
MAGPIE_SMOKE_URL="http://127.0.0.1:$MAGPIE_SMOKE_PORT" go test -tags smoke -count=1 -v ./internal/smoke/
