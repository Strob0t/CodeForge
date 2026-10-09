#!/usr/bin/env bash
# (Re)starts the LiteLLM proxy container with the repository's
# litellm/config.yaml. It uses the host network and listens on 127.0.0.1 only,
# so PostgreSQL and Ollama on 127.0.0.1 are reachable without container IPs.
# Provider keys are passed by name from the environment, never as values.
set -euo pipefail
# shellcheck source=scripts/live-e2e/lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

key_args=()
for entry in $LIVE_PROVIDER_KEYS; do
  key=${entry%%:*}
  if [ -n "${!key:-}" ]; then
    key_args+=(-e "$key")
  fi
done

live_ensure_db
docker rm -f "$LIVE_LITELLM_CONTAINER" >/dev/null 2>&1 || true
OLLAMA_OPENAI_API_BASE="$OLLAMA_BASE_URL/v1" OLLAMA_API_BASE="$OLLAMA_BASE_URL" \
  docker run -d --name "$LIVE_LITELLM_CONTAINER" --network host --memory 2g \
  --cap-drop ALL --security-opt no-new-privileges:true \
  -e LITELLM_MASTER_KEY -e DATABASE_URL -e OLLAMA_OPENAI_API_BASE -e OLLAMA_API_BASE \
  "${key_args[@]}" \
  -v "$LIVE_REPO_ROOT/litellm/config.yaml:/app/data/config.yaml:ro" \
  "$LIVE_LITELLM_IMAGE" \
  --config /app/data/config.yaml --host 127.0.0.1 --port "$LIVE_LITELLM_PORT" >/dev/null
live_log "LiteLLM container $LIVE_LITELLM_CONTAINER started (keys: ${CODEFORGE_LITELLM_KEYED_PROVIDERS:-none})"

deadline=$((SECONDS + 180))
until curl -sf -o /dev/null "$LITELLM_BASE_URL/health/liveliness"; do
  if [ "$(docker inspect -f '{{.State.Running}}' "$LIVE_LITELLM_CONTAINER" 2>/dev/null)" != true ]; then
    docker logs --tail 30 "$LIVE_LITELLM_CONTAINER" >&2 || true
    live_die "LiteLLM exited"
  fi
  [ "$SECONDS" -lt "$deadline" ] || live_die "LiteLLM not ready after 180 s"
  sleep 3
done
live_log "LiteLLM ready: $LITELLM_BASE_URL"
