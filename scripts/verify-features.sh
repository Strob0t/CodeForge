#!/usr/bin/env bash
# CodeForge Feature Verification Reporter
# Runs all test suites and maps results to the 30-feature verification matrix.
#
# Usage:
#   ./scripts/verify-features.sh          Run all suites, print matrix + JSON summary
#   ./scripts/verify-features.sh --quick  Skip test runs, parse existing results only
#   ./scripts/verify-features.sh --trend  Show historical verification trend
#
# Output:
#   stdout  -- Markdown table (same format as docs/feature-verification-matrix.md)
#   /tmp/verification-summary.json -- machine-readable summary
#   data/verification-history/     -- historical results (date_sha.json per run)
#
# Exit code:
#   0 if all critical features (1-10, 22-23) pass
#   1 otherwise
set -euo pipefail

ROOTDIR="$(cd "$(dirname "$0")/.." && pwd)"
QUICK="${1:-}"

GO_RESULTS_FILE="/tmp/go-test-results.json"
PY_RESULTS_FILE="/tmp/pytest-report.json"
CONTRACT_RESULTS_FILE="/tmp/contract-test-results.txt"
SMOKE_RESULTS_FILE="/tmp/smoke-test-results.txt"
SUMMARY_FILE="/tmp/verification-summary.json"
HISTORY_DIR="$ROOTDIR/data/verification-history"

TODAY="$(date +%Y-%m-%d)"
TIMESTAMP="$(date +%Y-%m-%dT%H:%M:%S)"

# ---------------------------------------------------------------------------
# Feature definitions: ID | Name | Go packages (grep patterns) | Py test files (grep patterns)
# ---------------------------------------------------------------------------
declare -a FEAT_ID FEAT_NAME FEAT_GO_PKG FEAT_PY_FILE

add_feature() {
  FEAT_ID+=("$1")
  FEAT_NAME+=("$2")
  FEAT_GO_PKG+=("$3")
  FEAT_PY_FILE+=("$4")
}

add_feature 1  "Project CRUD + Git Ops"           "adapter/postgres|service/project"                ""
add_feature 2  "Auth / JWT / RBAC"                 "middleware|service/auth"                         ""
add_feature 3  "Multi-Tenancy"                     "adapter/postgres"                                ""
add_feature 4  "LLM Model Registry"               "service/llm"                                     "test_llm"
add_feature 5  "LLM Key Management"               "service/llm"                                     "test_llm_key"
add_feature 6  "Conversation (Simple)"             "service/conversation"                            "test_consumer"
add_feature 7  "Conversation (Agentic)"            "service/conversation"                            "test_consumer|test_agent_loop"
add_feature 8  "Agent Tools"                       ""                                                "test_tool_|test_tools"
add_feature 9  "HITL Approval"                     "service/conversation"                            ""
add_feature 10 "Policy Layer"                      "service/policy"                                  ""
add_feature 11 "Modes System"                      "service/mode"                                    ""
add_feature 12 "MCP Server"                        "service/mcp"                                     "test_mcp"
add_feature 13 "LSP"                               "adapter/lsp"                                     ""
add_feature 14 "Retrieval"                         "service/retrieval"                                "test_retrieval"
add_feature 15 "GraphRAG"                          ""                                                "test_graph"
add_feature 16 "RepoMap"                           ""                                                "test_repomap"
add_feature 17 "Context Optimizer"                 "service/context"                                  ""
add_feature 18 "Memory System"                     "domain/memory"                                   "test_memory"
add_feature 19 "Experience Pool"                   "service/experience"                               "test_memory"
add_feature 20 "Microagent"                        "domain/microagent"                                ""
add_feature 21 "Skills"                            "domain/skill"                                     ""
add_feature 22 "Cost Tracking"                     "service/cost"                                     "test_pricing"
add_feature 23 "Benchmarks"                        "service/benchmark"                                "test_benchmark"
add_feature 24 "Evaluation"                        ""                                                "test_eval|test_litellm_judge"
add_feature 25 "Routing"                           ""                                                "test_routing"
add_feature 26 "Orchestration"                     "domain/orchestration"                             ""
add_feature 27 "Trust Annotations"                 "domain/trust"                                    "test_trust"
add_feature 28 "Quarantine"                        "service/quarantine"                               ""
add_feature 29 "A2A Protocol"                      "adapter/a2a"                                     "test_a2a"
add_feature 30 "Handoff"                           "service/handoff"                                  "test_handoff"

FEATURE_COUNT="${#FEAT_ID[@]}"

# ---------------------------------------------------------------------------
# Arrays to hold per-feature results
# ---------------------------------------------------------------------------
declare -a RES_GO RES_PY RES_CONTRACT RES_SMOKE
for (( i=0; i<FEATURE_COUNT; i++ )); do
  RES_GO+=("NONE")
  RES_PY+=("NONE")
  RES_CONTRACT+=("--")
  RES_SMOKE+=("--")
done

# ---------------------------------------------------------------------------
# Step 1: Run Go tests with JSON output
# ---------------------------------------------------------------------------
run_go_tests() {
  echo ">>> Running Go tests..." >&2
  go test -json -count=1 ./internal/... > "$GO_RESULTS_FILE" 2>/dev/null || true
  echo ">>> Go tests complete." >&2
}

# ---------------------------------------------------------------------------
# Step 2: Run Python tests
# ---------------------------------------------------------------------------
run_python_tests() {
  echo ">>> Running Python tests..." >&2
  cd "$ROOTDIR"
  if poetry run pytest --json-report --json-report-file="$PY_RESULTS_FILE" -q 2>/dev/null; then
    echo ">>> Python tests complete (json-report)." >&2
  else
    # json-report plugin might not be installed; fall back to plain run
    if [ ! -f "$PY_RESULTS_FILE" ]; then
      echo ">>> json-report unavailable, falling back to plain pytest..." >&2
      poetry run pytest -q > /tmp/pytest-plain.txt 2>&1 || true
      # Build a minimal JSON from plain output
      echo '{"summary":{"total":0},"tests":[]}' > "$PY_RESULTS_FILE"
      # Parse pass/fail counts from last line like "168 passed, 2 failed"
      if [ -f /tmp/pytest-plain.txt ]; then
        while IFS= read -r line; do
          # Lines like "workers/tests/test_tool_bash.py::test_name PASSED"
          local nodeid outcome
          nodeid="$(echo "$line" | awk '{print $1}')"
          outcome="$(echo "$line" | awk '{print $NF}')"
          case "$outcome" in
            PASSED|passed) echo "$nodeid PASS" >> /tmp/pytest-parsed.txt ;;
            FAILED|failed) echo "$nodeid FAIL" >> /tmp/pytest-parsed.txt ;;
          esac
        done < /tmp/pytest-plain.txt
      fi
    fi
    echo ">>> Python tests complete." >&2
  fi
}

# ---------------------------------------------------------------------------
# Step 3: Run contract tests
# ---------------------------------------------------------------------------
run_contract_tests() {
  echo ">>> Running contract tests..." >&2
  # Go side: generate fixtures
  go test "$ROOTDIR/internal/port/messagequeue/" -run Contract -v -count=1 > /tmp/contract-go.txt 2>&1 || true
  # Python side: validate fixtures
  cd "$ROOTDIR"
  poetry run pytest workers/tests/test_nats_contracts.py -v > "$CONTRACT_RESULTS_FILE" 2>&1 || true
  echo ">>> Contract tests complete." >&2
}

# ---------------------------------------------------------------------------
# Step 4: Check for smoke test results
# ---------------------------------------------------------------------------
check_smoke_tests() {
  if [ -f "$SMOKE_RESULTS_FILE" ]; then
    echo ">>> Smoke test results found." >&2
  else
    echo ">>> No smoke test results (expected in CI only)." >&2
  fi
}

# ---------------------------------------------------------------------------
# Parse Go test JSON results
# ---------------------------------------------------------------------------
parse_go_results() {
  if [ ! -f "$GO_RESULTS_FILE" ]; then
    return
  fi

  for (( i=0; i<FEATURE_COUNT; i++ )); do
    local pkg_pattern="${FEAT_GO_PKG[$i]}"
    if [ -z "$pkg_pattern" ]; then
      RES_GO[$i]="N/A"
      continue
    fi

    # Find matching test results -- look for Action=pass or Action=fail with matching Package
    local has_pass=false has_fail=false has_any=false

    while IFS= read -r line; do
      local action pkg
      # Extract Action and Package from JSON lines using bash string ops
      # Lines look like: {"Time":"...","Action":"pass","Package":"...","Test":"...","Elapsed":0.1}
      action="$(echo "$line" | grep -o '"Action":"[^"]*"' | head -1 | cut -d'"' -f4)" || continue
      pkg="$(echo "$line" | grep -o '"Package":"[^"]*"' | head -1 | cut -d'"' -f4)" || continue

      if [ -z "$action" ] || [ -z "$pkg" ]; then
        continue
      fi

      # Check if package matches any of the patterns (pipe-separated)
      local match=false
      IFS='|' read -ra patterns <<< "$pkg_pattern"
      for pat in "${patterns[@]}"; do
        if echo "$pkg" | grep -q "$pat"; then
          match=true
          break
        fi
      done

      if [ "$match" = true ]; then
        # Only count lines with Test field (individual test results, not package summaries without Test)
        local has_test
        has_test="$(echo "$line" | grep -o '"Test"' || true)"

        case "$action" in
          pass)
            if [ -n "$has_test" ]; then
              has_any=true
              has_pass=true
            fi
            ;;
          fail)
            if [ -n "$has_test" ]; then
              has_any=true
              has_fail=true
            fi
            ;;
        esac
      fi
    done < "$GO_RESULTS_FILE"

    if [ "$has_fail" = true ]; then
      RES_GO[$i]="FAIL"
    elif [ "$has_pass" = true ]; then
      RES_GO[$i]="PASS"
    elif [ "$has_any" = false ] && [ -n "$pkg_pattern" ]; then
      # Package pattern specified but no tests found
      RES_GO[$i]="NONE"
    fi
  done
}

# ---------------------------------------------------------------------------
# Parse Python test results
# ---------------------------------------------------------------------------
parse_python_results() {
  # Try JSON report first
  if [ -f "$PY_RESULTS_FILE" ] && grep -q '"tests"' "$PY_RESULTS_FILE" 2>/dev/null; then
    parse_python_json
    return
  fi

  # Fall back to parsed plain output
  if [ -f /tmp/pytest-parsed.txt ]; then
    parse_python_plain
    return
  fi
}

parse_python_json() {
  for (( i=0; i<FEATURE_COUNT; i++ )); do
    local py_pattern="${FEAT_PY_FILE[$i]}"
    if [ -z "$py_pattern" ]; then
      RES_PY[$i]="N/A"
      continue
    fi

    local has_pass=false has_fail=false

    # Read test entries from JSON -- each test has "nodeid" and "outcome"
    # Use grep to find matching tests
    IFS='|' read -ra patterns <<< "$py_pattern"
    for pat in "${patterns[@]}"; do
      # Look for lines containing the pattern and "passed" or "failed"
      if grep -q "\"nodeid\".*${pat}.*\"outcome\":.*\"passed\"" "$PY_RESULTS_FILE" 2>/dev/null; then
        has_pass=true
      fi
      if grep -q "\"nodeid\".*${pat}.*\"outcome\":.*\"failed\"" "$PY_RESULTS_FILE" 2>/dev/null; then
        has_fail=true
      fi
      # Also handle format where nodeid and outcome are on separate lines (pretty-printed)
      # Extract nodeids matching the pattern and check their outcomes
      local matching_tests
      matching_tests="$(grep -o "\"nodeid\": *\"[^\"]*${pat}[^\"]*\"" "$PY_RESULTS_FILE" 2>/dev/null || true)"
      if [ -n "$matching_tests" ]; then
        if grep -A5 "\"nodeid\": *\"[^\"]*${pat}" "$PY_RESULTS_FILE" 2>/dev/null | grep -q '"passed"'; then
          has_pass=true
        fi
        if grep -A5 "\"nodeid\": *\"[^\"]*${pat}" "$PY_RESULTS_FILE" 2>/dev/null | grep -q '"failed"'; then
          has_fail=true
        fi
      fi
    done

    if [ "$has_fail" = true ]; then
      RES_PY[$i]="FAIL"
    elif [ "$has_pass" = true ]; then
      RES_PY[$i]="PASS"
    fi
  done
}

parse_python_plain() {
  for (( i=0; i<FEATURE_COUNT; i++ )); do
    local py_pattern="${FEAT_PY_FILE[$i]}"
    if [ -z "$py_pattern" ]; then
      RES_PY[$i]="N/A"
      continue
    fi

    local has_pass=false has_fail=false

    IFS='|' read -ra patterns <<< "$py_pattern"
    for pat in "${patterns[@]}"; do
      if grep -q "${pat}.*PASS" /tmp/pytest-parsed.txt 2>/dev/null; then
        has_pass=true
      fi
      if grep -q "${pat}.*FAIL" /tmp/pytest-parsed.txt 2>/dev/null; then
        has_fail=true
      fi
    done

    if [ "$has_fail" = true ]; then
      RES_PY[$i]="FAIL"
    elif [ "$has_pass" = true ]; then
      RES_PY[$i]="PASS"
    fi
  done
}

# ---------------------------------------------------------------------------
# Parse contract test results
# ---------------------------------------------------------------------------
parse_contract_results() {
  if [ ! -f "$CONTRACT_RESULTS_FILE" ]; then
    return
  fi

  # Contract tests cover features that use NATS: 6,7,14,15,16,18,23,24,29,30
  # Map: if contract tests pass, mark those features
  local contract_pass=false contract_fail=false
  if grep -q "passed" "$CONTRACT_RESULTS_FILE" 2>/dev/null; then
    contract_pass=true
  fi
  if grep -qE "FAILED|ERROR|failed" "$CONTRACT_RESULTS_FILE" 2>/dev/null; then
    contract_fail=true
  fi

  # Features that use NATS contracts
  local nats_features=(5 6 13 14 15 17 22 23 28 29)
  for idx in "${nats_features[@]}"; do
    local arr_idx=$((idx - 1))
    if [ "$contract_fail" = true ]; then
      RES_CONTRACT[$arr_idx]="FAIL"
    elif [ "$contract_pass" = true ]; then
      RES_CONTRACT[$arr_idx]="PASS"
    fi
  done
}

# ---------------------------------------------------------------------------
# Parse smoke test results
# ---------------------------------------------------------------------------
parse_smoke_results() {
  if [ ! -f "$SMOKE_RESULTS_FILE" ]; then
    return
  fi

  # If smoke tests ran and passed, mark features covered by smoke flows
  if grep -q "PASS" "$SMOKE_RESULTS_FILE" 2>/dev/null; then
    # Flow 1 covers features 1,2,3
    for idx in 0 1 2; do
      RES_SMOKE[$idx]="PASS"
    done
  fi
  if grep -q "FAIL" "$SMOKE_RESULTS_FILE" 2>/dev/null; then
    for idx in 0 1 2; do
      RES_SMOKE[$idx]="FAIL"
    done
  fi
}

# ---------------------------------------------------------------------------
# Determine overall status per feature
# ---------------------------------------------------------------------------
compute_verified() {
  local idx="$1"
  local go="${RES_GO[$idx]}"
  local py="${RES_PY[$idx]}"
  local contract="${RES_CONTRACT[$idx]}"
  local smoke="${RES_SMOKE[$idx]}"

  # Check for any FAIL
  if [ "$go" = "FAIL" ] || [ "$py" = "FAIL" ] || [ "$contract" = "FAIL" ] || [ "$smoke" = "FAIL" ]; then
    echo "NO"
    return
  fi

  # Count applicable layers that are PASS
  local applicable=0 passed=0

  if [ "$go" != "N/A" ]; then
    applicable=$((applicable + 1))
    if [ "$go" = "PASS" ]; then
      passed=$((passed + 1))
    fi
  fi

  if [ "$py" != "N/A" ]; then
    applicable=$((applicable + 1))
    if [ "$py" = "PASS" ]; then
      passed=$((passed + 1))
    fi
  fi

  if [ "$applicable" -eq 0 ]; then
    echo "NO"
    return
  fi

  if [ "$passed" -eq "$applicable" ]; then
    echo "YES"
  elif [ "$passed" -gt 0 ]; then
    echo "partial"
  else
    echo "NO"
  fi
}

# ---------------------------------------------------------------------------
# Generate Markdown table
# ---------------------------------------------------------------------------
generate_markdown() {
  echo ""
  echo "# Feature Verification Report"
  echo ""
  echo "> Date: $TODAY"
  echo "> Generated by: scripts/verify-features.sh"
  echo ""
  echo "| # | Feature | Go Unit | Py Unit | Contract | Smoke | Verified |"
  echo "|---|---------|---------|---------|----------|-------|----------|"

  for (( i=0; i<FEATURE_COUNT; i++ )); do
    local verified
    verified="$(compute_verified "$i")"
    printf "| %d | %s | %s | %s | %s | %s | %s |\n" \
      "${FEAT_ID[$i]}" \
      "${FEAT_NAME[$i]}" \
      "${RES_GO[$i]}" \
      "${RES_PY[$i]}" \
      "${RES_CONTRACT[$i]}" \
      "${RES_SMOKE[$i]}" \
      "$verified"
  done

  echo ""

  # Summary counts
  local verified_count=0 partial_count=0 not_verified_count=0
  for (( i=0; i<FEATURE_COUNT; i++ )); do
    local v
    v="$(compute_verified "$i")"
    case "$v" in
      YES)     verified_count=$((verified_count + 1)) ;;
      partial) partial_count=$((partial_count + 1)) ;;
      NO)      not_verified_count=$((not_verified_count + 1)) ;;
    esac
  done

  echo "## Summary"
  echo ""
  echo "| Status | Count |"
  echo "|--------|-------|"
  echo "| Verified | $verified_count |"
  echo "| Partial | $partial_count |"
  echo "| Not verified | $not_verified_count |"
  echo ""
}

# ---------------------------------------------------------------------------
# Generate JSON summary
# ---------------------------------------------------------------------------
generate_json() {
  local verified_count=0 partial_count=0 not_verified_count=0

  local features_json="["
  for (( i=0; i<FEATURE_COUNT; i++ )); do
    local verified
    verified="$(compute_verified "$i")"
    case "$verified" in
      YES)     verified_count=$((verified_count + 1)) ;;
      partial) partial_count=$((partial_count + 1)) ;;
      NO)      not_verified_count=$((not_verified_count + 1)) ;;
    esac

    if [ "$i" -gt 0 ]; then
      features_json+=","
    fi
    features_json+="{\"id\":${FEAT_ID[$i]},\"name\":\"${FEAT_NAME[$i]}\",\"go_unit\":\"${RES_GO[$i]}\",\"py_unit\":\"${RES_PY[$i]}\",\"contract\":\"${RES_CONTRACT[$i]}\",\"smoke\":\"${RES_SMOKE[$i]}\",\"verified\":\"${verified}\"}"
  done
  features_json+="]"

  # Check critical features (1-10, 22-23)
  local critical_pass=true
  for crit_id in 1 2 3 4 5 6 7 8 9 10 22 23; do
    local idx=$((crit_id - 1))
    local v
    v="$(compute_verified "$idx")"
    if [ "$v" = "NO" ]; then
      # Check if there's an actual FAIL vs just missing tests
      if [ "${RES_GO[$idx]}" = "FAIL" ] || [ "${RES_PY[$idx]}" = "FAIL" ]; then
        critical_pass=false
      fi
    fi
  done

  cat > "$SUMMARY_FILE" <<ENDJSON
{
  "date": "$TODAY",
  "total_features": $FEATURE_COUNT,
  "verified": $verified_count,
  "partial": $partial_count,
  "not_verified": $not_verified_count,
  "critical_pass": $critical_pass,
  "features": $features_json
}
ENDJSON

  echo ">>> JSON summary written to $SUMMARY_FILE" >&2
}

# ---------------------------------------------------------------------------
# Store historical results
# ---------------------------------------------------------------------------
store_history() {
  mkdir -p "$HISTORY_DIR"
  local git_sha
  git_sha="$(git -C "$ROOTDIR" rev-parse --short HEAD 2>/dev/null || echo "unknown")"
  local git_branch
  git_branch="$(git -C "$ROOTDIR" branch --show-current 2>/dev/null || echo "unknown")"

  # Add metadata to the summary and save to history
  local history_file="$HISTORY_DIR/${TODAY}_${git_sha}.json"
  if [ -f "$SUMMARY_FILE" ]; then
    # Inject timestamp, git_sha, git_branch into the JSON
    python3 -c "
import json, sys
with open('$SUMMARY_FILE') as f:
    data = json.load(f)
data['timestamp'] = '$TIMESTAMP'
data['git_sha'] = '$git_sha'
data['git_branch'] = '$git_branch'
with open('$history_file', 'w') as f:
    json.dump(data, f, indent=2)
" 2>/dev/null || cp "$SUMMARY_FILE" "$history_file"
    echo ">>> History saved to $history_file" >&2
  fi
}

# ---------------------------------------------------------------------------
# Show trend from historical results
# ---------------------------------------------------------------------------
show_trend() {
  if [ ! -d "$HISTORY_DIR" ] || [ -z "$(ls -A "$HISTORY_DIR" 2>/dev/null)" ]; then
    echo "No historical verification data found in $HISTORY_DIR" >&2
    exit 0
  fi

  echo ""
  echo "# Verification Trend"
  echo ""
  echo "| Date | SHA | Branch | Verified | Partial | Failing | Critical |"
  echo "|------|-----|--------|----------|---------|---------|----------|"

  # Process last 20 entries sorted by filename (date_sha)
  for f in $(ls -1 "$HISTORY_DIR"/*.json 2>/dev/null | sort -r | head -20); do
    python3 -c "
import json, sys
with open('$f') as fh:
    d = json.load(fh)
date = d.get('date', '?')
sha = d.get('git_sha', '?')
branch = d.get('git_branch', '?')
verified = d.get('verified', 0)
partial = d.get('partial', 0)
not_v = d.get('not_verified', 0)
crit = 'PASS' if d.get('critical_pass', False) else 'FAIL'
print(f'| {date} | {sha} | {branch} | {verified} | {partial} | {not_v} | {crit} |')
" 2>/dev/null || true
  done

  echo ""

  # Show per-feature trend for the last 5 runs
  echo "## Per-Feature Trend (last 5 runs)"
  echo ""

  local files
  files="$(ls -1 "$HISTORY_DIR"/*.json 2>/dev/null | sort -r | head -5 | tac)"
  if [ -z "$files" ]; then
    echo "Not enough data for per-feature trend."
    return
  fi

  # Header
  local header="| Feature |"
  for f in $files; do
    local sha
    sha="$(python3 -c "import json; print(json.load(open('$f')).get('git_sha','?'))" 2>/dev/null || echo "?")"
    header+=" $sha |"
  done
  echo "$header"

  local separator="| --- |"
  for f in $files; do
    separator+=" --- |"
  done
  echo "$separator"

  # One row per feature (use first file to get feature list)
  local first_file
  first_file="$(echo "$files" | head -1)"
  local feat_count
  feat_count="$(python3 -c "import json; print(len(json.load(open('$first_file')).get('features',[])))" 2>/dev/null || echo 0)"

  for (( i=0; i<feat_count; i++ )); do
    local row=""
    local feat_name
    feat_name="$(python3 -c "import json; print(json.load(open('$first_file'))['features'][$i]['name'])" 2>/dev/null || echo "?")"
    row="| $feat_name |"
    for f in $files; do
      local status
      status="$(python3 -c "import json; print(json.load(open('$f'))['features'][$i]['verified'])" 2>/dev/null || echo "?")"
      row+=" $status |"
    done
    echo "$row"
  done

  echo ""
  exit 0
}

# ---------------------------------------------------------------------------
# Check critical features exit code
# ---------------------------------------------------------------------------
check_critical() {
  for crit_id in 1 2 3 4 5 6 7 8 9 10 22 23; do
    local idx=$((crit_id - 1))
    if [ "${RES_GO[$idx]}" = "FAIL" ] || [ "${RES_PY[$idx]}" = "FAIL" ]; then
      echo ">>> CRITICAL: Feature ${crit_id} (${FEAT_NAME[$idx]}) has FAIL results." >&2
      return 1
    fi
  done
  return 0
}

# ---------------------------------------------------------------------------
# Warn about non-critical feature regressions (GitHub Actions annotations)
# ---------------------------------------------------------------------------
warn_non_critical() {
  local non_critical_failures=0
  for feat_id in 11 12 13 14 15 16 17 18 19 20 21 24 25 26 27 28 29 30; do
    local idx=$((feat_id - 1))
    if [ "${RES_GO[$idx]}" = "FAIL" ] || [ "${RES_PY[$idx]}" = "FAIL" ]; then
      echo "::warning::Non-critical feature ${feat_id} (${FEAT_NAME[$idx]}) has regressions"
      non_critical_failures=$((non_critical_failures + 1))
    fi
  done
  if [ "$non_critical_failures" -gt 0 ]; then
    echo ">>> $non_critical_failures non-critical feature(s) have regressions (warnings only)." >&2
  fi
  # Output for GitHub Actions
  if [ -n "${GITHUB_OUTPUT:-}" ]; then
    echo "non_critical_failures=$non_critical_failures" >> "$GITHUB_OUTPUT"
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
  # Handle --trend flag: show historical trend and exit
  if [ "$QUICK" = "--trend" ]; then
    show_trend
    exit 0
  fi

  echo "=== CodeForge Feature Verification Reporter ===" >&2
  echo "Date: $TODAY" >&2
  echo "" >&2

  if [ "$QUICK" != "--quick" ]; then
    run_go_tests
    run_python_tests
    run_contract_tests
    check_smoke_tests
  else
    echo ">>> Quick mode: skipping test runs, parsing existing results." >&2
  fi

  parse_go_results
  parse_python_results
  parse_contract_results
  parse_smoke_results

  generate_markdown
  generate_json
  store_history

  warn_non_critical

  if check_critical; then
    echo ">>> All critical features OK (no FAIL results)." >&2
    exit 0
  else
    echo ">>> Some critical features FAILING -- see report above." >&2
    exit 1
  fi
}

main
