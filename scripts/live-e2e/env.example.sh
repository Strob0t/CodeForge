# shellcheck shell=bash
# Environment of a live end-to-end session, sourced by the start scripts.
#
# Every value can be set from the calling shell, for example
#   LIVE_DIR=$HOME/live LIVE_DB_NAME=codeforge_live2 scripts/live-e2e/start-core.sh
# or copy this file, edit the copy and point LIVE_ENV_FILE at it.
#
# DEV ONLY: the passwords, keys and secrets below are public development
# defaults. Never use them for a stack that anyone else can reach. Provider
# API keys never go into this file: export them from your secret store (see
# README.md).

# --- Paths -------------------------------------------------------------------
# LIVE_REPO_ROOT is set by lib.sh (the checkout these scripts belong to).
: "${LIVE_DIR:=${TMPDIR:-/tmp}/codeforge-live}"  # logs, PIDs, binary, data
export LIVE_DIR
export CODEFORGE_WORKSPACE_ROOT=${CODEFORGE_WORKSPACE_ROOT:-$LIVE_DIR/data/workspaces}
export CODEFORGE_POLICY_DIR=${CODEFORGE_POLICY_DIR:-$LIVE_DIR/data/policies}
export CODEFORGE_KNOWLEDGE_CONTENT_ROOT=${CODEFORGE_KNOWLEDGE_CONTENT_ROOT:-$LIVE_DIR/data/knowledge}
# Local repositories a platform admin may adopt as projects (local_path).
export CODEFORGE_WORKSPACE_ADOPT_ROOTS=${CODEFORGE_WORKSPACE_ADOPT_ROOTS:-$LIVE_DIR/repos}
export CODEFORGE_WORKSPACE=${CODEFORGE_WORKSPACE:-$LIVE_REPO_ROOT}

# --- Mode and accounts (DEV ONLY) --------------------------------------------
export APP_ENV=${APP_ENV:-development}
export CODEFORGE_AUTH_ADMIN_EMAIL=${CODEFORGE_AUTH_ADMIN_EMAIL:-admin@localhost}
export CODEFORGE_AUTH_ADMIN_PASS=${CODEFORGE_AUTH_ADMIN_PASS:-Changeme123}       # DEV ONLY
export CODEFORGE_AUTH_JWT_SECRET=${CODEFORGE_AUTH_JWT_SECRET:-live-e2e-dev-only-jwt-secret-0123456789abcdef}  # DEV ONLY
export CODEFORGE_INTERNAL_KEY=${CODEFORGE_INTERNAL_KEY:-live-e2e-dev-only-internal-key-0123456789abcdef}     # DEV ONLY

# --- Infrastructure (started separately, see README.md) ----------------------
# DATABASE_URL and NATS_URL are always derived from the LIVE_* values: the dev
# container exports both for the test suites, and a session must not share
# their database or stream.
: "${LIVE_PG_URL:=postgres://codeforge:codeforge_dev@127.0.0.1:5432}"  # DEV ONLY password
: "${LIVE_DB_NAME:=codeforge_live}"
: "${LIVE_NATS_URL:=nats://127.0.0.1:4222}"
export LIVE_PG_URL LIVE_DB_NAME LIVE_NATS_URL
export DATABASE_URL="$LIVE_PG_URL/$LIVE_DB_NAME?sslmode=disable"
export NATS_URL=$LIVE_NATS_URL
# A small JetStream stream suits a development disk (error 10047 otherwise).
export CODEFORGE_NATS_STREAM_MAX_BYTES=${CODEFORGE_NATS_STREAM_MAX_BYTES:-536870912}

# --- LLM ---------------------------------------------------------------------
: "${LIVE_LITELLM_CONTAINER:=codeforge-live-litellm}"
# The release production runs (docker-compose.prod.yml).
: "${LIVE_LITELLM_IMAGE:=ghcr.io/berriai/litellm:v1.103.1}"
: "${LIVE_LITELLM_PORT:=4000}"
export LIVE_LITELLM_CONTAINER LIVE_LITELLM_IMAGE LIVE_LITELLM_PORT
export LITELLM_BASE_URL=${LITELLM_BASE_URL:-http://127.0.0.1:$LIVE_LITELLM_PORT}
export LITELLM_MASTER_KEY=${LITELLM_MASTER_KEY:-sk-codeforge-dev}  # DEV ONLY
export OLLAMA_BASE_URL=${OLLAMA_BASE_URL:-http://127.0.0.1:11434}
# The model every conversation, agent and worker call uses (no routing).
: "${LIVE_MODEL:=ollama/qwen3:4b-instruct}"
export LIVE_MODEL
export CODEFORGE_CONVERSATION_MODEL=${CODEFORGE_CONVERSATION_MODEL:-$LIVE_MODEL}
export CODEFORGE_AGENT_DEFAULT_MODEL=${CODEFORGE_AGENT_DEFAULT_MODEL:-$LIVE_MODEL}
export CODEFORGE_DEFAULT_MODEL=${CODEFORGE_DEFAULT_MODEL:-$LIVE_MODEL}
export CODEFORGE_ROUTING_ENABLED=${CODEFORGE_ROUTING_ENABLED:-false}
# Cloud provider keys LiteLLM receives when they are set in the environment;
# the Core and the worker get only the names of the keyed providers.
LIVE_PROVIDER_KEYS="OPENAI_API_KEY:openai ANTHROPIC_API_KEY:anthropic GEMINI_API_KEY:gemini GROQ_API_KEY:groq MISTRAL_API_KEY:mistral OPENROUTER_API_KEY:openrouter CEREBRAS_API_KEY:cerebras CHUTES_API_KEY:chutes AIHUBMIX_API_KEY:aihubmix"
if [ -z "${CODEFORGE_LITELLM_KEYED_PROVIDERS+set}" ]; then
  CODEFORGE_LITELLM_KEYED_PROVIDERS=
  for _entry in $LIVE_PROVIDER_KEYS; do
    _key=${_entry%%:*}
    if [ -n "${!_key:-}" ]; then
      CODEFORGE_LITELLM_KEYED_PROVIDERS+="${_entry#*:},"
    fi
  done
  unset _entry _key
fi
export CODEFORGE_LITELLM_KEYED_PROVIDERS

# --- Ports and the worker ----------------------------------------------------
# DEV ONLY credentials and isolation off: the Core listens on loopback only,
# like LiteLLM and the frontend (KI-213).
export CODEFORGE_HOST=${CODEFORGE_HOST:-127.0.0.1}
export CODEFORGE_PORT=${CODEFORGE_PORT:-8080}
export CODEFORGE_CORE_URL=${CODEFORGE_CORE_URL:-http://127.0.0.1:$CODEFORGE_PORT}
export CODEFORGE_WORKER_HEALTH_PORT=${CODEFORGE_WORKER_HEALTH_PORT:-8081}
: "${LIVE_FRONTEND_PORT:=3000}"
export LIVE_FRONTEND_PORT
# DEV ONLY: tool processes run as the worker's user, without Landlock.
export CODEFORGE_TOOL_ISOLATION=${CODEFORGE_TOOL_ISOLATION:-off}
# The interpreter of agent tool processes (pytest, ruff), like the worker image's.
: "${LIVE_TOOL_PYTHON:=python3.12}"
: "${LIVE_TOOL_VENV:=$LIVE_DIR/toolenv}"
export LIVE_TOOL_PYTHON LIVE_TOOL_VENV
