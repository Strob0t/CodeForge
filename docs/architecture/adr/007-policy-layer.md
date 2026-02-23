# ADR-007: Policy Layer -- Permission Rules, Quality Gates, and Termination Conditions

> **Status:** accepted
> **Date:** 2026-02-17
> **Deciders:** Project lead + Claude Code analysis

### Context

CodeForge executes AI coding agents at varying autonomy levels (supervised to headless). At higher autonomy levels, safety rules replace the human approver. The system needs a configurable mechanism to:

- Decide per tool call whether an agent may execute it (allow/deny/ask)
- Enforce quality gates before delivering results (tests pass, lint clean)
- Terminate runs that exceed resource limits (steps, cost, time, stall)
- Apply different rules to different contexts (plan-only agents vs. trusted autonomous agents)

The policy system must be declarative (YAML-configurable), evaluable at runtime (sub-millisecond per tool call), and extensible (custom policies without code changes).

### Decision

**First-match-wins permission rules** with quality gates and termination conditions, organized as named PolicyProfile presets.

#### Domain Model (`internal/domain/policy/`)

```go
type PolicyProfile struct {
    Name          string
    Description   string
    Mode          PermissionMode        // default, acceptEdits, plan, delegate
    Rules         []PermissionRule      // ordered, first-match-wins
    QualityGate   QualityGate           // tests, lint, rollback
    Termination   TerminationCondition  // steps, timeout, cost, stall
    ResourceLimits *resource.Limits     // memory, CPU, PIDs (optional)
}

type PermissionRule struct {
    Specifier    ToolSpecifier         // tool name + optional sub-pattern
    Decision     Decision              // allow, deny, ask
    PathAllow    []string              // glob patterns (path must match one)
    PathDeny     []string              // glob patterns (path must not match any)
    CommandAllow []string              // prefix patterns (command must match one)
    CommandDeny  []string              // prefix patterns (command must not match any)
}
```

#### Evaluation Algorithm

```text
For each rule in profile.Rules (ordered):
    1. Does rule.Specifier match the ToolCall? (tool name + sub-pattern glob)
    2. Does the path pass PathDeny/PathAllow constraints?
    3. Does the command pass CommandDeny/CommandAllow constraints?
    If all match -> return rule.Decision
    If any fails -> skip to next rule

No rule matched -> return defaultDecisionForMode(profile.Mode)
    plan      -> deny
    default   -> ask
    acceptEdits/delegate -> allow
```

Deny lists take precedence over allow lists within a single rule. This means a path that matches both `PathAllow` and `PathDeny` is denied (safe default).

#### Built-in Presets (4)

| Preset | Mode | Use Case | Key Rules |
|---|---|---|---|
| `plan-readonly` | plan | Read-only agents, architecture analysis | Allow Read/Search/List, deny Edit/Write/Bash; 30 steps, 300s, $1 |
| `headless-safe-sandbox` | default | Safe autonomous execution | Allow Read/Edit/Write (deny .env/secrets), Bash limited to git/test; 50 steps, 600s, $5 |
| `headless-permissive-sandbox` | default | Broader autonomous execution | Allow most tools, deny network commands (curl/wget/ssh); 100 steps, 1800s, $20 |
| `trusted-mount-autonomous` | acceptEdits | Trusted agents on mounted repos | Allow all tools, deny only secrets paths; 200 steps, 3600s, $50 |

#### Custom Policies

Users create YAML files in the policy directory (`CODEFORGE_POLICY_DIR`):

```yaml
name: my-custom-policy
description: "Custom policy for internal tools"
mode: default
rules:
  - specifier: { tool: "Read" }
    decision: allow
  - specifier: { tool: "Edit" }
    decision: allow
    path_deny: ["**/.env", "**/secrets/**"]
  - specifier: { tool: "Bash", sub_pattern: "git *" }
    decision: allow
  - specifier: { tool: "Bash" }
    decision: deny
quality_gate:
  require_tests_pass: true
  require_lint_pass: true
  rollback_on_gate_fail: true
termination:
  max_steps: 75
  timeout_seconds: 900
  max_cost: 10.0
  stall_detection: true
  stall_threshold: 5
```

Custom profiles are loaded on startup from YAML files and can be created/deleted at runtime via the REST API.

#### Integration Points

- Runtime (per tool call): `PolicyService.Evaluate(profile, toolCall)` returns a Decision
- Runtime (termination): `checkTermination()` reads TerminationCondition from active policy
- Checkpoint (rollback): QualityGate.RollbackOnGateFail triggers `CheckpointService.RewindToLast()`
- Sandbox (resources): ResourceLimits merged into Docker container flags
- Frontend: PolicyPanel with list/detail/editor views and evaluate tester

### Consequences

#### Positive

- Declarative: Policies are data (YAML), not code, so operators configure without programming
- Fast: First-match-wins is O(n) with small n (typically 5-15 rules); sub-microsecond evaluation
- Composable: Presets cover common cases; custom policies handle edge cases
- Safe defaults: Mode-based fallback ensures unmatched tools get the safest decision for the context
- Auditable: Policy evaluation is deterministic, meaning same input always produces same output

#### Negative

- No scope hierarchy yet: Policies are flat (per-run), not layered (global to project to run override). Mitigation: deferred to future work; single-level policies are sufficient for current use cases.
- No "why matched" explanation: The evaluate endpoint returns the decision but not which rule matched. Mitigation: deferred; useful for debugging but not critical for operation.
- Rule ordering matters: First-match-wins means rule order is semantically significant, so reordering rules can change behavior. Mitigation: presets are carefully ordered; custom policies must be reviewed by the user.

#### Neutral

- Glob patterns (`**`, `*`, `?`) reuse Go's `filepath.Match` with a custom `**` extension, avoiding regex overhead
- Command matching uses prefix-based comparison (`git` matches `git status`, `git push`), keeping it simple and predictable
- Built-in presets cannot be deleted via API (protection against accidental removal)

### Alternatives Considered

| Alternative | Pros | Cons | Why Not |
|---|---|---|---|
| OPA (Open Policy Agent) | Industry standard, Rego language, powerful | External service, learning curve for Rego, overkill for our rule set | Too heavy for per-tool-call evaluation in a dev tool; our rules are simple enough for a Go function |
| Role-Based Access Control (RBAC) | Well-understood pattern | Maps poorly to tool-call decisions; roles don't capture path/command constraints | Policies are about what tools can do, not who can use them |
| Allow-all with blocklist only | Simpler mental model | Cannot express "only these tools are allowed" (whitelist scenarios) | Plan-readonly mode requires explicit allow-listing of read-only tools |
| Regex-based matching | More powerful patterns | Slower evaluation, harder to write correctly, regex injection risk | Glob patterns are sufficient for file paths and command prefixes |

### References

- `internal/domain/policy/policy.go` -- Domain model (PolicyProfile, PermissionRule, ToolCall, Decision)
- `internal/domain/policy/presets.go` -- 4 built-in presets
- `internal/domain/policy/validate.go` -- Profile validation
- `internal/domain/policy/loader.go` -- YAML loading + SaveToFile
- `internal/service/policy.go` -- PolicyService with first-match-wins evaluation
- `internal/service/policy_test.go` -- 25+ test functions
- `internal/adapter/http/handlers.go` -- REST API handlers (list, get, evaluate, create, delete)
- `frontend/src/features/project/PolicyPanel.tsx` -- Frontend UI component
- `docs/features/04-agent-orchestration.md` -- Policy System section
