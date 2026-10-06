#!/usr/bin/env bash
# Blue-green deployment for CodeForge (see docker-compose.blue-green.yml)
#
# Usage: ./scripts/deploy-blue-green.sh [blue|green]
#   Starts the given color (default: the one that is not running), waits
#   until its core and frontend are healthy, routes Traefik to it and then
#   stops the other color. If the new color does not become healthy it is
#   stopped and the active one keeps serving.
#
#   Traefik has no Docker access (KI-214): its file provider reads the routes
#   this script writes to traefik/dynamic/active-color.yaml (replaced by a
#   rename; Traefik reloads it within about two seconds).
#
#   The shared services (postgres, nats, litellm) must already run and be
#   healthy (`docker compose -f docker-compose.prod.yml -f
#   docker-compose.blue-green.yml up -d`). The color is started with
#   --no-deps, so the deployment never recreates them.
#
#   This script switches only the core and the frontend. The worker is not
#   colored: an upgrade to per-tenant tool users (KI-96, ADR-018) stops every
#   worker by hand first (`docker compose ... stop worker`, all replicas),
#   deploys a color with this script, then starts the new worker image
#   (`docker compose ... up -d worker`). An older worker must never run next
#   to a new one or after the upgrade: it runs every tenant's tools as one
#   user in the workspace group and takes no tenant lock. Run
#   ./scripts/check-host.sh with the new worker image before.
#
# Environment:
#   ACME_EMAIL, CODEFORGE_DOMAIN  required by the overlay (or set in .env)
#   DRY_RUN=1                     print the plan and run the changing compose
#                                 commands with --dry-run (nothing is pulled,
#                                 created, started or stopped)
#   HEALTH_TIMEOUT (default 120), HEALTH_INTERVAL (default 5) in seconds
#   ROUTE_SWITCH_WAIT (default 3) seconds Traefik gets to load new routes
#                                 before the old color stops
set -euo pipefail

cd "$(dirname "$0")/.."

COMPOSE=(docker compose -f docker-compose.prod.yml -f docker-compose.blue-green.yml)
# The colors are compose profiles; naming them lets ps/stop see both.
export COMPOSE_PROFILES=blue,green

# The services the colors depend on (core's depends_on in the prod file).
SHARED_SERVICES=(postgres nats litellm)

HEALTH_TIMEOUT=${HEALTH_TIMEOUT:-120}
HEALTH_INTERVAL=${HEALTH_INTERVAL:-5}
ROUTE_SWITCH_WAIT=${ROUTE_SWITCH_WAIT:-3}
DRY_RUN=${DRY_RUN:-0}

# Traefik's file provider watches this directory (docker-compose.blue-green.yml).
ROUTES=traefik/dynamic/active-color.yaml

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

# health prints the health of a container: healthy, starting, unhealthy, or
# none when it has no health check.
health() {
    docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$1" 2>/dev/null || echo unknown
}

# require_shared_services fails unless every shared service runs and, when it
# has a health check, is healthy: the colors are started with --no-deps, so
# nothing else starts them (and an `up` with dependencies could recreate them).
require_shared_services() {
    local service id state missing=()
    for service in "${SHARED_SERVICES[@]}"; do
        id=$(running_id "$service")
        state=$([ -n "$id" ] && health "$id" || echo "not running")
        if [ "$state" != "healthy" ] && [ "$state" != "none" ]; then
            missing+=("$service ($state)")
        fi
    done
    if [ "${#missing[@]}" -gt 0 ]; then
        echo "ERROR: shared services are not running and healthy: ${missing[*]}" >&2
        echo "Start them first: ${COMPOSE[*]} up -d" >&2
        return 1
    fi
    echo "Shared services running: ${SHARED_SERVICES[*]}"
}

# wait_healthy waits until the service's container reports healthy.
wait_healthy() {
    local service=$1 elapsed=0 id
    echo "Waiting for $service to be healthy..."
    while [ "$elapsed" -lt "$HEALTH_TIMEOUT" ]; do
        id=$(running_id "$service")
        if [ -n "$id" ] && [ "$(health "$id")" = "healthy" ]; then
            echo "$service is healthy"
            return 0
        fi
        sleep "$HEALTH_INTERVAL"
        elapsed=$((elapsed + HEALTH_INTERVAL))
    done
    echo "ERROR: $service did not become healthy within ${HEALTH_TIMEOUT}s" >&2
    return 1
}

# write_routes points Traefik at a color: the core gets /api, /health, /ws,
# /.well-known and /a2a, the frontend everything else. Traefik fills in the
# domain from its environment (Go template). The file is replaced by a
# rename, so Traefik never reads half of it; it skips the .tmp name.
write_routes() {
    local template
    template=$(cat <<'ROUTES_EOF'
# Written by scripts/deploy-blue-green.sh: Traefik routes to the @COLOR@ color.
http:
  routers:
    core:
      rule: 'Host(`{{ env "CODEFORGE_DOMAIN" }}`) && (PathPrefix(`/api`) || PathPrefix(`/health`) || PathPrefix(`/ws`) || PathPrefix(`/.well-known`) || PathPrefix(`/a2a`))'
      entryPoints: [websecure]
      tls:
        certResolver: letsencrypt
      middlewares: [rate-limit, security-headers]
      service: core
      priority: 20
    frontend:
      rule: 'Host(`{{ env "CODEFORGE_DOMAIN" }}`)'
      entryPoints: [websecure]
      tls:
        certResolver: letsencrypt
      middlewares: [security-headers]
      service: frontend
      priority: 1
  services:
    core:
      loadBalancer:
        servers:
          - url: http://core-@COLOR@:8080
    frontend:
      loadBalancer:
        servers:
          - url: http://frontend-@COLOR@:8080
ROUTES_EOF
)
    if [ "$DRY_RUN" = "1" ]; then
        echo "(dry run) would route Traefik to $1 ($ROUTES)"
        return 0
    fi
    printf '%s\n' "${template//@COLOR@/$1}" > "$ROUTES.tmp"
    mv -f "$ROUTES.tmp" "$ROUTES"
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
require_shared_services

if [ "$DRY_RUN" = "1" ]; then
    # A dry-run pull still asks the registry; the plan is shown instead.
    echo "(dry run) would pull core-$TARGET frontend-$TARGET"
else
    change pull "core-$TARGET" "frontend-$TARGET"
fi

# start_healthy starts one service of the color without its dependencies and
# waits until it is healthy; on failure the color is stopped again.
start_healthy() {
    change up -d --no-deps "$1"
    if [ "$DRY_RUN" = "1" ]; then
        echo "(dry run) would wait for $1 to be healthy"
        return 0
    fi
    if ! wait_healthy "$1"; then
        echo "Deployment of $TARGET failed; stopping it, $ACTIVE keeps serving." >&2
        change stop "core-$TARGET" "frontend-$TARGET"
        exit 1
    fi
}

# Traefik keeps routing to the active color while the new one starts; the
# routes are (re)written first, so a Traefik started or recreated here (the
# first deployment, an upgrade from the Docker provider) serves it at once.
if [ "$ACTIVE" != "none" ]; then
    write_routes "$ACTIVE"
fi
change up -d --no-deps traefik
# The frontend proxies to its color's core: the core first.
start_healthy "core-$TARGET"
start_healthy "frontend-$TARGET"

echo "Switching traffic from $ACTIVE to $TARGET..."
write_routes "$TARGET"

if [ "$DRY_RUN" = "1" ]; then
    if [ "$ACTIVE" != "none" ]; then
        change stop "core-$ACTIVE" "frontend-$ACTIVE"
    fi
    echo "(dry run) done; nothing was changed"
    exit 0
fi

if [ "$ACTIVE" != "none" ]; then
    sleep "$ROUTE_SWITCH_WAIT"
    change stop "core-$ACTIVE" "frontend-$ACTIVE"
fi

echo "Blue-green deployment complete. Active: $TARGET"
