#!/usr/bin/env bash
# Blue-green deployment for CodeForge (see docker-compose.blue-green.yml)
#
# Usage: ./scripts/deploy-blue-green.sh [blue|green]
#   Starts the given color (default: the one that is not running), waits
#   until its core and frontend are healthy and then stops the other color.
#   Traefik routes to whichever color runs. If the new color does not become
#   healthy it is stopped and the active one keeps serving.
#
# Environment:
#   ACME_EMAIL, CODEFORGE_DOMAIN  required by the overlay (or set in .env)
#   DRY_RUN=1                     print the plan and run the changing compose
#                                 commands with --dry-run (nothing is pulled,
#                                 created, started or stopped)
#   HEALTH_TIMEOUT (default 120), HEALTH_INTERVAL (default 5) in seconds
set -euo pipefail

cd "$(dirname "$0")/.."

COMPOSE=(docker compose -f docker-compose.prod.yml -f docker-compose.blue-green.yml)
# The colors are compose profiles; naming them lets ps/stop see both.
export COMPOSE_PROFILES=blue,green

HEALTH_TIMEOUT=${HEALTH_TIMEOUT:-120}
HEALTH_INTERVAL=${HEALTH_INTERVAL:-5}
DRY_RUN=${DRY_RUN:-0}

# change runs a compose command that changes the deployment (simulated in a
# dry run).
change() {
    if [ "$DRY_RUN" = "1" ]; then
        echo "+ docker compose --dry-run $*"
        "${COMPOSE[@]}" --dry-run "$@"
    else
        "${COMPOSE[@]}" "$@"
    fi
}

# running_id prints the ID of the service's running container (empty if none).
running_id() {
    "${COMPOSE[@]}" ps -q --status running "$1" 2>/dev/null | head -n 1
}

# detect_active prints the color whose core is running, or "none".
detect_active() {
    local blue green
    blue=$(running_id core-blue)
    green=$(running_id core-green)
    if [ -n "$blue" ] && [ -n "$green" ]; then
        echo "ERROR: both colors are running; stop one before deploying" >&2
        return 1
    elif [ -n "$blue" ]; then
        echo blue
    elif [ -n "$green" ]; then
        echo green
    else
        echo none
    fi
}

# wait_healthy waits until the service's container reports healthy.
wait_healthy() {
    local service=$1 elapsed=0 id health
    echo "Waiting for $service to be healthy..."
    while [ "$elapsed" -lt "$HEALTH_TIMEOUT" ]; do
        id=$(running_id "$service")
        if [ -n "$id" ]; then
            health=$(docker inspect --format '{{.State.Health.Status}}' "$id" 2>/dev/null || echo unknown)
            if [ "$health" = "healthy" ]; then
                echo "$service is healthy"
                return 0
            fi
        fi
        sleep "$HEALTH_INTERVAL"
        elapsed=$((elapsed + HEALTH_INTERVAL))
    done
    echo "ERROR: $service did not become healthy within ${HEALTH_TIMEOUT}s" >&2
    return 1
}

ACTIVE=$(detect_active)
echo "Active color: $ACTIVE"

TARGET=${1:-}
if [ -z "$TARGET" ]; then
    if [ "$ACTIVE" = "blue" ]; then TARGET=green; else TARGET=blue; fi
fi
case "$TARGET" in
    blue | green) ;;
    *)
        echo "Usage: $0 [blue|green]" >&2
        exit 2
        ;;
esac
if [ "$TARGET" = "$ACTIVE" ]; then
    echo "ERROR: $TARGET is the active color; deploy the other one" >&2
    exit 2
fi
echo "Deploying: $TARGET"

if [ "$DRY_RUN" = "1" ]; then
    # A dry-run pull still asks the registry; the plan is shown instead.
    echo "(dry run) would pull core-$TARGET frontend-$TARGET"
else
    change pull "core-$TARGET" "frontend-$TARGET"
fi
# Traefik routes to the colors; it is started here if it is not running yet.
change up -d traefik
change up -d "core-$TARGET" "frontend-$TARGET"

if [ "$DRY_RUN" = "1" ]; then
    echo "(dry run) would wait for core-$TARGET and frontend-$TARGET to be healthy"
    if [ "$ACTIVE" != "none" ]; then
        change stop "core-$ACTIVE" "frontend-$ACTIVE"
    fi
    echo "(dry run) done; nothing was changed"
    exit 0
fi

if ! wait_healthy "core-$TARGET" || ! wait_healthy "frontend-$TARGET"; then
    echo "Deployment of $TARGET failed; stopping it, $ACTIVE keeps serving." >&2
    change stop "core-$TARGET" "frontend-$TARGET"
    exit 1
fi

if [ "$ACTIVE" != "none" ]; then
    echo "Switching traffic from $ACTIVE to $TARGET..."
    change stop "core-$ACTIVE" "frontend-$ACTIVE"
fi

echo "Blue-green deployment complete. Active: $TARGET"
