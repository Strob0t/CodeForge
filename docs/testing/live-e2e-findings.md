# Live end-to-end findings

A running log of live end-to-end sessions: the full stack, a real model and the web UI, used the way a user would. Each
session records what was tried, what worked, what broke (each bug also becomes a Known Issue in
[docs/todo.md](../todo.md#known-issues)), where the experience rubbed, and what it suggests for the product. The last
part is meant for discussion with the project owner.

Since 2026-10-03 every milestone ends with a live session. The benchmark for a fully autonomous programming goal is
[autonomous-goal-benchmark.md](autonomous-goal-benchmark.md).

## Session template

```
## Session N - YYYY-MM-DD - <focus>
Stack: <branch/commit>, <mode>, <model and hardware>, <isolation on/off>
### Journeys
### What worked
### What broke (-> KI-x)
### Friction
### Product and vision notes (for discussion)
```

---

## Session 1 - 2026-10-03 - README screenshots with a local model

**Stack:**
- Code: branch `claude/busy-dijkstra-q0oxi9` at `aa3ed945`, development mode, tool isolation off.
- Model: Ollama on 4 CPU cores, with `qwen3:4b-instruct` (about 9 tokens/s with 3 threads). `qwen2.5:1.5b` and
  `qwen2.5:3b` were tried first.
- No API key.

### Journeys
1. Log in as admin and change the password.
2. Add two projects from public GitHub repositories (`pypa/sampleproject`, `sindresorhus/ky`) and let them clone and
   index.
3. Build a roadmap with two milestones and seven features.
4. In a project conversation, ask the agent to add a function and a unit test and run the tests, under a custom
   policy profile in which edits need approval.
5. Start three agent runs through the API.
6. Look at the dashboard, the AI configuration (models, modes), MCP servers, costs and routing.

### What worked
- Cloning, stack detection, goals, the roadmap with drag and drop, the file tree and editor.
- The agent loop with a 4B local model, once it could call tools: it read both files, edited them, ran the tests, saw
  a failure, fixed the missing import and ran the tests again until they passed. That took 3 approvals and about
  18 minutes.
- The live approval card (Allow, Allow Always, Deny), driven by a custom policy profile.
- Modes, MCP server definitions and the model list all behaved as expected.

### What broke
- **KI-125 (high):** a local model gets no tools unless its name contains "instruct", "coder" or "qwen3". An agent
  run without a model in its config gets no tools. Parallel tool calls from Ollama are glued together. The shipped
  `ollama/*` route streams tool calls as text.
- **KI-126:** a failed `bash` command returns only "exit code 1", so the model never sees the failing test's output.
- **KI-127:** token counts stay at 0 for OpenAI-compatible backends.
- **KI-128:** `PYTHONPATH=src python -m unittest` is refused under a profile with a deny list.
- **KI-129:**
  - Conversation runs count neither on the dashboard nor on the cost page, and the onboarding never completes.
  - A new project shows as critical.
  - The approval countdown is always 60 s.
  - The preset list is incomplete.
  - Long model IDs overflow.
  - Feature titles are cut.
  - Tool cards appear only after the run.
- **KI-130:** the default embedding model is OpenAI's, so indexing fails on every clone without a key; the frontend
  install downloads icons from the network.

### Friction
- Getting a local model to call tools took several configuration changes that a self-hoster would not find alone.
- During a run, the chat shows only streamed text. You cannot see which tool is running until the run ends.
- Dashboard numbers (cost $0.00, 0 tokens, no activity) contradict what just happened in the chat.

### Product and vision notes (for discussion)
1. **The local-model story is a differentiator, and right now it is broken.** "Runs fully offline with your own
   model" is one of the strongest self-hosting arguments, and the README makes it. KI-125 should be fixed before
   anything is published.
2. **Pure-completion models should still be able to act.** AGENTS.md says pure-completion models get "everything":
   context, tools, prompts, quality. Today they get no tools at all. A text tool-call protocol (ReAct style, parsed by
   the worker) for models without native tool calling would make the capability levels real. Small local models
   would then be usable for simple tasks.
3. **One view of all agent work.** The control-plane promise is "everything is recorded and visible". Conversation
   runs and API runs live in two separate worlds (dashboard, costs, activity). A single "agent work" view across
   both would make the core message visible in the product.
4. **Live observability.** Seeing the tool call while it runs (not after) matters for trust and for approvals. It is
   the moment where a user decides whether to intervene.
5. **Small models need more help from the platform.** The 4B model reached green tests although a failed command
   returned only its exit code (KI-126); it guessed the cause. Feeding errors back (KI-126), tighter task splitting
   from the roadmap, and automatic test runs after edits are platform features that raise the floor for weak
   models. That fits "any model" better than requiring a strong one.

## Session 2 - 2026-10-04 - fixes of S8/S7, roadmap, agent chat, benchmark run 1

**Stack:** branch `claude/busy-dijkstra-q0oxi9` at `6c25472d`, development mode, tool isolation off, Ollama
`qwen3:4b-instruct` on 4 CPU cores (`num_ctx` 16384), no API key. Started with the new `scripts/live-e2e/`.

### Journeys
1. Fresh start: login, password change, the model list and the AI page.
2. A project from a public repository: clone, setup, index, search.
3. Roadmap: AI proposals, approving milestones, the "Sync with PM" preview.
4. Agent chat with tools under a deny-list profile: a code change plus tests.
5. Settings > Privacy and the MCP page.
6. Benchmark run 1 (see [autonomous-goal-benchmark.md](autonomous-goal-benchmark.md)).

### What worked
- The Ollama model is listed first and keyless cloud routes collapse to one row each (S8-A confirmed).
- `POST /projects/{id}/setup` clones, detects the stack, creates a vision goal and roadmap, and builds the repo map
  and code graph.
- Roadmap proposal cards, the PM sync form's validation and its "token is never stored" note.
- Policy: `PYTHONPATH=src python -m pytest` allowed under the deny-list profile and denied by the safe preset, as
  documented (KI-128); a failed command's output reached the model (KI-126), which installed pytest and reran
  (4 passed). Write approvals with a live countdown.
- The data export (no secrets) and the MCP page's redaction and link-local refusal.

### What broke (-> KI)
- Conversation messages of one turn share a timestamp, so tool results come back before their calls and the next
  turn's history is out of order (KI-147, high).
- A reload loses the running turn and its pending approval, which then times out (KI-148, high).
- Validation and not-found errors answer 500: short password, bad PM reference, unknown consent purpose (KI-149).
- Without an embedding key the index ends in `error`, not BM25-only, and `/search` gives no hint (KI-150).
- `propose_roadmap` has no allow rule in any preset and runs into the 60 s approval timeout (KI-151).
- The github-issues PM preview fails without `gh` (KI-117).
- The auto-agent marks a feature done whenever its run ends (KI-152); implementation turns are offered the planning
  tools (KI-153).
- Smaller: context window mismatch (KI-154), the admin password reset on every start (KI-155), dead settings
  (KI-156), roadmap card approval 404 without a roadmap (KI-157), `CODEFORGE_TOOL_PATH` ignored with isolation off
  (KI-158), the chat stays "running" (KI-159), no cacheable prompt prefix (KI-160). Tool cards still appear only after
  a reload and the dashboard still shows 0 tokens (KI-129).

### Friction
- The first turn of every conversation spends 2.5 to 4.5 minutes on prompt prefill on CPU.
- Each roadmap proposal costs an approval plus a card click; approving a card starts a new chat turn.
- The AI page does not mark the default model, shows the model card's 262,144-token context instead of the server's
  16,384, and offers Delete on config routes. A VCS onboarding modal covers it.
- The agent chat run took 16.5 minutes (81k tokens in, 1k out); the model ignored "the test must fail first" and once
  replaced a whole file through `write_file`; the approval card showed no diff.

### Product and vision notes (for discussion)
1. **Verification is the platform's job.** The auto-agent believes the model. Running the project's test and lint
   commands and a change check after every feature, and feeding failures back, would turn "the model says done" into
   "done" (KI-152). This is the strongest lever for weak models.
2. **Tool hygiene per phase.** Planning tools in implementation turns pull small models back into planning
   (KI-153); a "continue" nudge when a model announces an action without calling a tool would help too.
3. **Deterministic scaffolding.** Plans should start with a scaffold step (package, CLI entry, test layout) and carry
   acceptance tests per step; the planner left both out.
4. **Speed on local hardware.** A stable, cacheable prompt prefix and a compact prompt for small models cut minutes of
   prefill per conversation (KI-160); the context should follow the server (KI-154).
5. **Reviewable changes.** Edits instead of whole-file writes for existing files, and diffs in approval cards.
