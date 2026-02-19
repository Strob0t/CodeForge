#!/usr/bin/env bash
# Blue-Green deployment for CodeForge
# Usage: ./scripts/deploy-blue-green.sh [blue|green]
set -euo pipefail

COMPOSE_FILES="-f docker-compose.prod.yml -f docker-compose.blue-green.yml"
HEALTH_TIMEOUT=60
HEALTH_INTERVAL=5

# Detect current active color by checking which core container is running
detect_active() {
    if docker compose $COMPOSE_FILES ps core-blue --status running 2>/dev/null | grep -q "running"; then
        echo "blue"
    elif docker compose $COMPOSE_FILES ps core-green --status running 2>/dev/null | grep -q "running"; then
        echo "green"
    else
        echo "none"
    fi
}

# Wait for a service to be healthy
wait_healthy() {
    local service=$1
    local elapsed=0

    echo "Waiting for $service to be healthy..."
    while [ $elapsed -lt $HEALTH_TIMEOUT ]; do
        if docker compose $COMPOSE_FILES ps "$service" --status running 2>/dev/null | grep -q "running"; then
            local health
            health=$(docker inspect --format='{{.State.Health.Status}}' "$(docker compose $COMPOSE_FILES ps -q "$service")" 2>/dev/null || echo "unknown")
            if [ "$health" = "healthy" ]; then
                echo "$service is healthy"
                return 0
            fi
        fi
        sleep $HEALTH_INTERVAL
        elapsed=$((elapsed + HEALTH_INTERVAL))
    done

    echo "ERROR: $service did not become healthy within ${HEALTH_TIMEOUT}s"
    return 1
}

# Main deployment logic
ACTIVE=$(detect_active)
echo "Current active deployment: $ACTIVE"

if [ "${1:-}" != "" ]; then
    TARGET=$1
else
    # Auto-select: deploy to the inactive color
    if [ "$ACTIVE" = "blue" ]; then
        TARGET="green"
    else
        TARGET="blue"
    fi
fi

echo "Deploying to: $TARGET"

# Pull latest images
echo "Pulling latest images..."
docker compose $COMPOSE_FILES pull "core-${TARGET}" "frontend-${TARGET}"

# Start the target services
echo "Starting $TARGET services..."
docker compose $COMPOSE_FILES up -d "core-${TARGET}" "frontend-${TARGET}"

# Wait for health
if ! wait_healthy "core-${TARGET}"; then
    echo "Deployment failed. Rolling back..."
    docker compose $COMPOSE_FILES stop "core-${TARGET}" "frontend-${TARGET}"
    exit 1
fi

echo "$TARGET deployment is healthy."

# Update Traefik priorities to route traffic to new deployment
# Higher priority = preferred route. We swap priorities by scaling down the old.
if [ "$ACTIVE" != "none" ] && [ "$ACTIVE" != "$TARGET" ]; then
    echo "Switching traffic from $ACTIVE to $TARGET..."
    docker compose $COMPOSE_FILES stop "core-${ACTIVE}" "frontend-${ACTIVE}"
    echo "Old $ACTIVE services stopped."
fi

echo "Blue-green deployment complete. Active: $TARGET"
