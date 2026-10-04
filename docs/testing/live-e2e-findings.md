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
