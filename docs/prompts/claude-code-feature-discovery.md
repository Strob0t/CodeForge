# Codebase Unwired Feature Discovery Prompt

> Systematic discovery of all implemented but disconnected features in a multi-layer codebase.
> Target: Claude Code (agentic AI) | Template: ReAct + Stop Conditions | ~1600 tokens

## Research Background

This prompt is based on research into how LLMs can systematically discover latent capabilities:

- **JustAsk Framework** (arXiv 2601.21233) — Treats capability discovery as an online exploration problem using UCB-based strategy selection across a hierarchical skill space.
- **Anthropic Context Engineering** — Progressive disclosure: agents incrementally discover relevant context through exploration, not static prompts.
- **Springer Taxonomy of Prompt Engineering** (Oct 2025) — Multi-vector structured probing outperforms single-shot queries for discovery tasks.
- **Agent Engineering** (BioData Mining, Nov 2025) — Agents with autonomy, persistence, and multi-step reasoning surpass frozen prompt chains.

### Sources

- https://arxiv.org/html/2601.21233v1
- https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents
- https://link.springer.com/article/10.1007/s11704-025-50058-z
- https://link.springer.com/article/10.1186/s13040-025-00502-4

## Strategy

Multi-layer codebase projects accumulate "unwired" features: code that is fully implemented but never connected end-to-end. This prompt dispatches 5 parallel discovery vectors — one per architecture layer — then cross-references results to find disconnected wiring. The key insight: a feature only works if EVERY layer in the chain is connected (Frontend Route -> API Call -> HTTP Handler -> Service Method -> Store Query -> DB Table, or HTTP Handler -> NATS Publisher -> Python Subscriber -> Result Publisher -> Go Subscriber).

---

## Prompt

```
You are a codebase integration auditor. Your ONLY job: systematically discover
every feature that is IMPLEMENTED but NOT WIRED UP end-to-end.

"Unwired" means: code exists (handlers, components, tables, subjects, classes)
but the chain from user action to final effect is broken at one or more points.

You MUST audit all 5 architecture layers using parallel agents. For each layer,
produce findings BEFORE synthesis.

## Architecture Layers to Audit

### LAYER 1: HTTP ENDPOINTS (Go)
Dispatch an Explore agent:
- Read the router file to get ALL registered routes
- Read ALL handler files for handler functions
- Cross-reference: which handlers are registered vs orphaned?
- For each registered endpoint, check if the frontend calls it
- For each handler function, check if it's called by anything

Output:
| Endpoint | Method | Handler | Registered? | Called by Frontend? | Status |

### LAYER 2: MESSAGE QUEUE SUBJECTS (NATS)
Dispatch an Explore agent:
- Read Go-side subject definitions (constants file)
- Read Python-side subject definitions (constants file)
- For each subject: does it have a publisher AND subscriber?
- Check both directions: Go->Python AND Python->Go
- Check JetStream stream config covers all subject prefixes

Output:
| Subject | Go Publisher? | Python Subscriber? | Python Publisher? | Go Subscriber? | Wired? |

### LAYER 3: FRONTEND COMPONENTS & API
Dispatch an Explore agent:
- Read router/App config for all registered routes
- Glob ALL components in features/ and pages/ directories
- For each component: is it imported by a route or parent component?
- For each API method: is it called anywhere in the frontend?
- Check for stores/signals defined but never used

Output:
| Component/API Method | Type | Imported? | Accessible? | Status |

### LAYER 4: DATABASE & SERVICE LAYER
Dispatch an Explore agent:
- Read ALL migration files, extract CREATE TABLE statements
- For each table: grep for queries (SELECT/INSERT/UPDATE/DELETE)
- For each store method: is it called by a service?
- For each service method: is it called by a handler?
- Trace the full chain: Table -> Store -> Service -> Handler

Output:
| Table/Method | Layer | Connected Upstream? | Connected Downstream? | Status |

### LAYER 5: WORKER HANDLERS & TOOLS
Dispatch an Explore agent:
- Find all Python NATS handler functions
- Check which are registered as subscribers vs just defined
- Find all tool implementations — which are in the registry?
- Find all evaluator/provider plugins — which are registered?
- Check for classes defined but never instantiated

Output:
| Handler/Class | File | Registered? | Instantiated? | Status |

## Cross-Reference (after all 5 layers complete)

For each finding marked as UNWIRED or ORPHANED:
1. Trace the full intended chain and identify WHERE it breaks
2. Classify the break type:
   - MISSING_ROUTE: Handler exists, no route
   - MISSING_FRONTEND: Endpoint exists, no UI
   - MISSING_SUBSCRIBER: Publisher exists, no consumer
   - MISSING_PUBLISHER: Subscriber exists, no producer
   - MISSING_STORE: Table exists, no queries
   - MISSING_SERVICE: Store exists, no service caller
   - MISSING_HANDLER: Service exists, no HTTP handler
   - DEAD_CODE: Fully orphaned, nothing references it
3. Assess impact: is this a planned feature, deprecated code, or a bug?

## Output Format

Produce a single markdown document with:

### Summary
Total features audited, total unwired, breakdown by layer.

### Unwired Features by Layer
One section per layer with the tables above.

### Cross-Reference Analysis
| Feature | Break Point | Break Type | Impact | Recommendation |

### Dead Code Candidates
Code that should be removed (no upstream, no downstream, no tests).

### Partially Wired Features
Code where MOST of the chain works but one link is missing.

## Rules
- NEVER guess connectivity. Grep for actual imports/calls/references.
- NEVER skip a layer. If a layer has 0 issues, state that explicitly.
- NEVER modify any file. This is READ-ONLY analysis.
- Use parallel Explore agents for all 5 layers simultaneously.
- After each layer completes, output: "Layer N complete - [count] issues found"
- Prioritize PARTIALLY WIRED features over DEAD CODE — partial features
  represent the highest value: most of the work is done, only one link is missing.
```
