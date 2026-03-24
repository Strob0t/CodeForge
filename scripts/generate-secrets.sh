#!/usr/bin/env bash
# scripts/generate-secrets.sh -- Generate random secrets for production
#
# Usage: ./scripts/generate-secrets.sh [secrets_dir]
# Default dir: ./secrets
set -euo pipefail

SECRETS_DIR="${1:-./secrets}"
mkdir -p "$SECRETS_DIR"

generate() {
    local name="$1"
    local file="$SECRETS_DIR/$name"
    if [ -f "$file" ]; then
        echo "Secret $name already exists, skipping"
        return
    fi
    openssl rand -base64 32 | tr -d '\n' > "$file"
    chmod 600 "$file"
    echo "Generated $name"
}

generate litellm-master-key
generate postgres-password
generate nats-user
generate nats-pass

echo ""
echo "Secrets generated in $SECRETS_DIR"
echo "Add $SECRETS_DIR to .gitignore (if not already) to prevent committing secrets."
