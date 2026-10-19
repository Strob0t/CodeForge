# shellcheck shell=bash
# Sourced by setup.sh and by ~/.bashrc in the dev container (KI-124): the Go
# Core and the Python worker run in its shells and must share
# CODEFORGE_INTERNAL_KEY (worker calls to the Core get 401 otherwise). The key
# is generated once per container and kept outside the repository; a key the
# user exported first is kept. APP_ENV, the database URL and the LiteLLM
# master key come from devcontainer.json (remoteEnv).

if [ -z "${CODEFORGE_INTERNAL_KEY:-}" ]; then
    _cf_key_dir="${XDG_CONFIG_HOME:-$HOME/.config}/codeforge"
    _cf_key_file="$_cf_key_dir/internal_key"
    if [ ! -s "$_cf_key_file" ]; then
        mkdir -p "$_cf_key_dir" && chmod 700 "$_cf_key_dir"
        (umask 077 && od -An -N32 -tx1 /dev/urandom | tr -d ' \n' >"$_cf_key_file")
    fi
    CODEFORGE_INTERNAL_KEY="$(cat "$_cf_key_file")"
    export CODEFORGE_INTERNAL_KEY
    unset _cf_key_dir _cf_key_file
fi
