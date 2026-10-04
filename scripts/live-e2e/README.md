# Live end-to-end scripts

Scripts that start the full CodeForge stack from a checkout, with a real model, for live end-to-end sessions: you use
the product the way a self-hoster would, through the web UI and the API, and record what you find in
[`docs/testing/live-e2e-findings.md`](../../docs/testing/live-e2e-findings.md). The
[autonomous goal benchmark](../../docs/testing/autonomous-goal-benchmark.md) runs on the same stack.

This is a **development** stack: development mode, tool isolation off (agent tools run as your user, without
Landlock), and well-known development passwords and keys. Never expose it to anyone else.

## Prerequisites

- PostgreSQL 18, NATS with JetStream and an Ollama server, all reachable on `127.0.0.1`. In the dev container they
  run already (`codeforge-test-postgres`, `codeforge-test-nats`); otherwise `docker compose up -d postgres nats`
  from the repository root, with ports published on `127.0.0.1`.
- Docker (for the LiteLLM container), `psql` (to create the session's database; otherwise create it yourself).
- Go 1.25, Python 3.12, the worker's Poetry environment (`cd workers && poetry install`) and the frontend
  packages (`cd frontend && npm ci`).
- A model in Ollama. The default is `qwen3:4b-instruct` (`ollama pull qwen3:4b-instruct`): it calls tools, and
  runs on 4 CPU cores at about 9 tokens/s. Give Ollama a context of at least 16k tokens
  (`OLLAMA_CONTEXT_LENGTH=16384`).

## Starting and stopping

Start the parts in this order; each script waits until its part is healthy:

```bash
scripts/live-e2e/start-litellm.sh    # LiteLLM proxy (container, host network, 127.0.0.1:4000)
scripts/live-e2e/start-core.sh       # builds and starts the Go Core (8080); creates the database
scripts/live-e2e/start-worker.sh     # Python worker (health on 8081); needs the Core: it creates the NATS stream
scripts/live-e2e/start-frontend.sh   # Vite dev server (127.0.0.1:3000)
scripts/live-e2e/stop.sh             # stops frontend, worker, Core and LiteLLM (or name the parts)
```

Open <http://127.0.0.1:3000> and log in as `admin@localhost` / `Changeme123`; the first login asks for a new
password (at least 10 characters, with upper and lower case and a digit). Every Core start sets the admin's password
back to `CODEFORGE_AUTH_ADMIN_PASS`, so after a restart log in with that value again.

Everything a session writes goes below `LIVE_DIR` (default `$TMPDIR/codeforge-live`, else `/tmp/codeforge-live`):
`logs/<part>.log` (the previous log is kept with a timestamp), `run/<part>.pid`, `bin/codeforge`, and `data/`
(workspaces, policy profiles, knowledge content). Local repositories you want to adopt as projects
(`local_path`) go into `$LIVE_DIR/repos`.

## Configuration

[`env.example.sh`](env.example.sh) holds every setting, with defaults for a local session. Override any of them from
the shell, or copy the file, edit the copy and point `LIVE_ENV_FILE` at it:

```bash
LIVE_DIR=$HOME/live2 LIVE_DB_NAME=codeforge_live2 scripts/live-e2e/start-core.sh
```

| Variable | Default | Meaning |
|---|---|---|
| `LIVE_DIR` | `$TMPDIR/codeforge-live` | Logs, PIDs, binary and data of the session |
| `LIVE_DB_NAME` | `codeforge_live` | The session's database (Core and LiteLLM share it; `DATABASE_URL` is always built from `LIVE_PG_URL` and this name, never inherited); use a fresh one per session |
| `LIVE_MODEL` | `ollama/qwen3:4b-instruct` | The model of conversations, agents and the worker (routing off) |
| `OLLAMA_BASE_URL` | `http://127.0.0.1:11434` | Ollama server |
| `LIVE_NATS_URL` | `nats://127.0.0.1:4222` | NATS (exported as `NATS_URL`); a NATS of your own keeps other test runs away from the stream |
| `LIVE_WORKER_PYTHON` | the Poetry environment of `workers/` | The worker's interpreter |
| `LIVE_TOOL_PYTHON` | `python3.12` | Builds `LIVE_TOOL_VENV` (`$LIVE_DIR/toolenv`): the Python with pytest and ruff (`workers/tool-requirements.txt`) that agent tools find first on their PATH, as in the worker image |
| `LIVE_SKIP_BUILD` | `0` | `1` reuses `$LIVE_DIR/bin/codeforge` |

The values marked `DEV ONLY` in `env.example.sh` are public development defaults.

## A cloud model

Provider keys never go into a file. Store the key as a secret of your environment (for a cloud session: the
environment's secrets; locally: your secret manager), so that it reaches the shell as an environment variable, then
choose the model and restart the stack:

```bash
# ANTHROPIC_API_KEY (or another provider's key) comes from the environment's secret store
export LIVE_MODEL=anthropic/<model>
scripts/live-e2e/stop.sh
scripts/live-e2e/start-litellm.sh && scripts/live-e2e/start-core.sh && scripts/live-e2e/start-worker.sh
scripts/live-e2e/start-frontend.sh
```

`start-litellm.sh` hands LiteLLM the key variables that are set (`docker run -e NAME`, by name, so the value is in
no command line), and the Core and the worker learn only which providers have a key
(`CODEFORGE_LITELLM_KEYED_PROVIDERS`). The supported key variables are listed in `LIVE_PROVIDER_KEYS` in
`env.example.sh` and match `litellm/config.yaml`.

## The API client

[`api.py`](api.py) is a small client (standard library only) for the steps a session repeats: log in, projects,
goals, conversations, runs and approvals. It reads the same environment and logs in with
`CODEFORGE_AUTH_ADMIN_PASS` (or `LIVE_ADMIN_PASS`, if set).

```bash
scripts/live-e2e/api.py login
scripts/live-e2e/api.py project-create demo --repo https://github.com/pypa/sampleproject
scripts/live-e2e/api.py call POST /projects/<id>/setup '{}'     # clone, detect the stack, index
scripts/live-e2e/api.py conv-create <project-id>
scripts/live-e2e/api.py send <conversation-id> "Add a multiply function with tests"
scripts/live-e2e/api.py messages <conversation-id>
scripts/live-e2e/api.py approvals                               # approvals requested (from the Core log)
scripts/live-e2e/api.py approve <run-id> <call-id> allow
```

`api.py --help` lists every command; `api.py call METHOD PATH [JSON]` reaches any other endpoint.
