#!/usr/bin/env bash
set -euo pipefail
# Validates required env vars before production deployment.
REQUIRED=(POSTGRES_PASSWORD LITELLM_MASTER_KEY CODEFORGE_INTERNAL_KEY CODEFORGE_JWT_SECRET)
for var in "${REQUIRED[@]}"; do
  val="${!var:-}"
  if [ -z "$val" ]; then
    echo "ERROR: $var is not set" >&2; exit 1
  fi
  if echo "$val" | grep -qE '(codeforge_dev|sk-codeforge-dev|codeforge-internal-dev)'; then
    echo "ERROR: $var contains a default/insecure value" >&2; exit 1
  fi
done
echo "All required env vars validated."
