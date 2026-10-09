# Web UI screenshots

Screenshots of the CodeForge web UI for the project README. All images are 1440x900 at device scale factor 1, taken with
Playwright (Chromium) in the default theme (system, light), against a development stack (`APP_ENV=development`) with a
local model, `ollama/qwen3:4b-instruct`, served by Ollama through LiteLLM. No API key was used.

Demo data: two projects cloned from public GitHub repositories, `pypa/sampleproject` (Python) and `sindresorhus/ky`
(TypeScript); a roadmap with two milestones for `sampleproject`; one agent conversation and three agent runs.

| Image | Caption | Page and state |
|---|---|---|
| `dashboard.png` | The projects dashboard: two projects with health, runs and success rate, KPI strip and charts. | `/` after three agent runs (one failed run, two completed). |
| `project.png` | A project: file tree, editor with the file the agent changed, and the agent chat. | `/projects/<id>` of `sampleproject`, Files panel, `src/sample/simple.py` open. |
| `roadmap.png` | The roadmap of a project: milestones and features with status and labels. | `/projects/<id>` of `sampleproject`, "Plan" panel with Goals collapsed, panel split at 62 %. |
| `agent-chat.png` | The agent at work: it fixes a missing import after a failed test run and runs the tests again, which pass. | `/projects/<id>` of `sampleproject`, chat at full width, last `bash` tool call and its result expanded. |
| `approval.png` | A human-in-the-loop approval card: the agent asks before it edits a file (Allow, Allow Always, Deny). | `/projects/<id>` of `sampleproject` during a run with the custom policy profile `review-writes` (edits ask). |
| `models.png` | LLM model settings: the model served through LiteLLM and the form to add another model. | `/ai`, Models tab, "Add Model" form open. |
| `modes.png` | Agent modes: built-in roles with their tools, denied actions, LLM scenario and autonomy level. | `/ai`, Modes tab. |
| `mcp-servers.png` | MCP servers available to agents (stdio and SSE). | `/mcp` with three server definitions (`filesystem`, `fetch`, `docs`). |
