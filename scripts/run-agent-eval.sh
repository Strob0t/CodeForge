#!/usr/bin/env bash
# Automated agent evaluation runner for S1-S4 scenarios.
#
# Usage:
#   ./scripts/run-agent-eval.sh [--scenario S1|S2|S3|S4] [--model MODEL] [--timeout SECONDS]
#
# Prerequisites:
#   - Docker services running (postgres, nats, litellm)
#   - Go backend running on :8080
#   - Python worker running
#
# Output:
#   - PASS / PARTIAL / FAIL per scenario
#   - JSON metrics to stdout
#
# Recommended CI schedule: nightly (too slow for per-PR)

set -euo pipefail

SCENARIO="${1:---scenario}"
MODEL="${MODEL:-lm_studio/qwen/qwen3-30b-a3b}"
TIMEOUT="${TIMEOUT:-3600}"
BASE_URL="${BASE_URL:-http://localhost:8080}"
WORKSPACE_ROOT="${WORKSPACE_ROOT:-/workspaces/CodeForge/data/workspaces}"

# Parse args
while [[ $# -gt 0 ]]; do
    case $1 in
        --scenario) SCENARIO="$2"; shift 2 ;;
        --model) MODEL="$2"; shift 2 ;;
        --timeout) TIMEOUT="$2"; shift 2 ;;
        *) echo "Unknown arg: $1"; exit 1 ;;
    esac
done

if [[ -z "$SCENARIO" || "$SCENARIO" == "--scenario" ]]; then
    SCENARIO="S1"
fi

echo "=== Agent Eval: $SCENARIO ==="
echo "Model: $MODEL"
echo "Timeout: ${TIMEOUT}s"

# Login
TOKEN=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"email":"admin@localhost","password":"Changeme123"}' \
    | python3 -c "import sys,json; print(json.load(sys.stdin).get('access_token',''))" 2>/dev/null)

if [[ -z "$TOKEN" ]]; then
    echo "FAIL: Could not authenticate"
    exit 1
fi

AUTH="Authorization: Bearer $TOKEN"

# Setup workspace
WS_DIR="$WORKSPACE_ROOT/eval-${SCENARIO,,}-$(date +%s)"
mkdir -p "$WS_DIR"

case "$SCENARIO" in
    S1)
        cd "$WS_DIR" && git init -b main
        python3 -c "
text = 'The quick brown fox jumps over the lazy dog. ' * 800
with open('test.txt', 'w') as f:
    for i in range(100):
        f.write(text[i*350:(i+1)*350] + '\n')
"
        wc test.txt > expected_wc_output.txt
        echo "# Build Your Own wc Tool" > README.md
        git add . && git commit -m "initial: test data" -q

        PROMPT='Build a Python clone of the Unix wc tool called ccwc.py.\n\nRequirements:\n1. Count bytes: ccwc -c test.txt\n2. Count lines: ccwc -l test.txt\n3. Count words: ccwc -w test.txt\n4. Default: show lines, words, bytes\n5. Stdin: cat test.txt | python ccwc.py -l\n6. Handle missing file errors\n7. Write pytest tests in test_ccwc.py\n8. Commit to git\n\nTest file test.txt exists in workspace. Compare against real wc.\nWorkspace: '"$WS_DIR"
        ;;
    S2)
        cd "$WS_DIR" && git init -b main
        python3 -c "
lines = ['f0\tf1\tf2\tf3\tf4\tf5','1\tJohn Smith\t35\tM\t90000\tNew York','2\tJane Doe\t28\tF\t85000\tSan Francisco','3\tBob Wilson\t42\tM\t110000\tChicago']
with open('sample.tsv', 'w') as f:
    f.write('\n'.join(lines) + '\n')
csv_lines = ['id,name,age,city','1,Alice,30,New York','2,Bob,25,Los Angeles','3,Charlie,35,Chicago']
with open('sample.csv', 'w') as f:
    f.write('\n'.join(csv_lines) + '\n')
"
        echo "# Build Your Own cut Tool" > README.md
        git add . && git commit -m "initial: test data" -q

        PROMPT='Build a Python clone of the Unix cut tool as a package (cccut/).\n\nRequirements:\n1. Package: cccut/__init__.py, __main__.py, parser.py, cutter.py\n2. Field extraction: cccut -f2 sample.tsv\n3. Custom delimiter: cccut -d, -f1,3 sample.csv\n4. Stdin mode\n5. Error handling\n6. Tests in tests/\n7. Commit to git\n\nTest data exists. Compare against real cut.\nWorkspace: '"$WS_DIR"
        ;;
    *)
        echo "Scenario $SCENARIO not yet automated"
        exit 1
        ;;
esac

# Create project
PID=$(curl -s -X POST "$BASE_URL/api/v1/projects" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"name":"eval-'"$SCENARIO"'","local_path":"'"$WS_DIR"'","config":{"autonomy_level":"4","policy_preset":"trusted-mount-autonomous","execution_mode":"mount"}}' \
    | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null)

if [[ -z "$PID" ]]; then
    echo "FAIL: Could not create project"
    exit 1
fi

# Create conversation + bypass
CONV_ID=$(curl -s -X POST "$BASE_URL/api/v1/projects/$PID/conversations" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"title":"eval-'"$SCENARIO"'"}' \
    | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null)

curl -s -X POST "$BASE_URL/api/v1/conversations/$CONV_ID/bypass-approvals" -H "$AUTH" > /dev/null

# Dispatch agentic message
curl -s -X POST "$BASE_URL/api/v1/conversations/$CONV_ID/messages" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"content":"'"$(echo -e "$PROMPT")"'","role":"user","model":"'"$MODEL"'","agentic":true}' > /dev/null

echo "Dispatched. Polling until completion (timeout: ${TIMEOUT}s)..."

# Poll until done
START_TIME=$(date +%s)
while true; do
    sleep 30
    ELAPSED=$(( $(date +%s) - START_TIME ))
    if [[ $ELAPSED -gt $TIMEOUT ]]; then
        echo "TIMEOUT after ${ELAPSED}s"
        break
    fi

    RESP=$(curl -s "$BASE_URL/api/v1/conversations/$CONV_ID/messages" -H "$AUTH")
    MSG_COUNT=$(echo "$RESP" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null)
    TOOL_COUNT=$(echo "$RESP" | python3 -c "import sys,json; print(sum(len(m.get('tool_calls',[]) or []) for m in json.load(sys.stdin)))" 2>/dev/null)

    DONE=$(echo "$RESP" | python3 -c "
import sys,json
msgs=json.load(sys.stdin)
for m in msgs:
    if m.get('role')=='assistant' and not m.get('tool_calls') and len(m.get('content',''))>50:
        print('DONE')
        break
" 2>/dev/null)

    echo "  [${ELAPSED}s] msgs=$MSG_COUNT tools=$TOOL_COUNT"

    if [[ "$DONE" == "DONE" ]]; then
        echo "Agent completed after ${ELAPSED}s"
        break
    fi
done

# Validation
echo ""
echo "=== Validation ==="
cd "$WS_DIR"

PASS=0
FAIL=0
TOTAL=0

check() {
    TOTAL=$((TOTAL + 1))
    if eval "$1" > /dev/null 2>&1; then
        echo "PASS: $2"
        PASS=$((PASS + 1))
    else
        echo "FAIL: $2"
        FAIL=$((FAIL + 1))
    fi
}

case "$SCENARIO" in
    S1)
        check "test -f ccwc.py" "ccwc.py exists"
        check "test -f test_ccwc.py" "test_ccwc.py exists"
        check "python3 -m py_compile ccwc.py" "syntax valid"
        EXPECTED_L=$(wc -l < test.txt | tr -d ' ')
        ACTUAL_L=$(python3 ccwc.py -l test.txt 2>/dev/null | grep -oP '\d+' | head -1)
        check "[ '$EXPECTED_L' = '$ACTUAL_L' ]" "-l line count"
        EXPECTED_W=$(wc -w < test.txt | tr -d ' ')
        ACTUAL_W=$(python3 ccwc.py -w test.txt 2>/dev/null | grep -oP '\d+' | head -1)
        check "[ '$EXPECTED_W' = '$ACTUAL_W' ]" "-w word count"
        EXPECTED_C=$(wc -c < test.txt | tr -d ' ')
        ACTUAL_C=$(python3 ccwc.py -c test.txt 2>/dev/null | grep -oP '\d+' | head -1)
        check "[ '$EXPECTED_C' = '$ACTUAL_C' ]" "-c byte count"
        check "python3 ccwc.py test.txt 2>/dev/null | grep -qP '\d+\s+\d+\s+\d+'" "default output"
        ;;
    S2)
        check "test -d cccut" "package dir exists"
        check "test -f cccut/__main__.py" "__main__.py exists"
        check "python3 -m py_compile cccut/__init__.py" "syntax valid"
        check "test -d tests" "tests dir exists"
        check "test -f pyproject.toml" "pyproject.toml exists"
        ;;
esac

echo ""
RESULT="FAIL"
if [[ $PASS -eq $TOTAL ]]; then
    RESULT="PASS"
elif [[ $PASS -gt $(($TOTAL / 2)) ]]; then
    RESULT="PARTIAL"
fi

echo "=== Result: $RESULT ($PASS/$TOTAL) ==="
echo ""

# JSON output
python3 -c "
import json
print(json.dumps({
    'scenario': '$SCENARIO',
    'model': '$MODEL',
    'result': '$RESULT',
    'passed': $PASS,
    'total': $TOTAL,
    'messages': ${MSG_COUNT:-0},
    'tool_calls': ${TOOL_COUNT:-0},
    'duration_s': ${ELAPSED:-0},
    'workspace': '$WS_DIR',
}, indent=2))
"
