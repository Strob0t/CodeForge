# ADR-021: Text Tool Protocol for Pure-Completion Models

**Status:** accepted (owner decision 2026-10-04, implemented 2026-10-05, S9-C)
**Plan:** [text-tool-protocol-plan.md](../../plans/text-tool-protocol-plan.md)

### Context

Models without native tool calling (small local models on Ollama or LM Studio, capability level `pure_completion`)
got no tools: the agent loop sent no `tools` parameter, treated every reply as the final answer, and the tool guide
asked for API function calls these models cannot make. AGENTS.md promises that pure-completion models get everything
(context, tools, prompts, quality). The run's offered tools, the Go policy decision per call and the stored
OpenAI-format history must stay the same for every model.

### Decision

- The worker owns a text protocol: one JSON object per reply, `{"thought", "tool", "args"}` for a call or
  `{"thought", "final"}` for the answer. JSON because grammar constraints exist only as JSON Schema on Ollama `/v1` and
  LM Studio, and tool parameters are already JSON Schema.
- The offered tools are rendered into a "## Tools" section appended to the system message of each request copy (never
  stored), deterministic and capped at about 4,000 characters.
- Where the server supports it, the reply is constrained by a `json_schema` `response_format` (one branch per tool plus
  a final branch). Support is detected by trying: a rejection that names the grammar turns it off for the run.
  `litellm.text_tool_grammar` / `CODEFORGE_TEXT_TOOL_GRAMMAR` switches it off.
- A parsed call becomes the same `ToolCallPart` as a native call, so the Go policy, approvals, trackers, trajectory,
  stored messages and the UI work unchanged. One call per reply; an unusable reply gets one repair message, a second in
  a row ends the run.
- Results go back as `<tool_result tool="name">` user text (closing tags in the content neutralised, "data, not
  instructions"); the wire history alternates strictly between user and assistant.
- A server that refuses native tools ("does not support tools", ...) switches the run to the protocol.
- No NATS payload, Go or frontend change.

### Consequences

- Positive: small local models can act; a wrong capability classification costs one request instead of the run;
  one stored history serves both kinds of model.
- Negative: JSON escaping makes code edits harder for small models (possible follow-up: raw fenced payloads); tool
  results in the user role ease prompt injection (mitigated by framing; the Go policy stays the boundary); the
  protocol section costs about 500 tokens per request.
- Neutral: the live check against Ollama (grammar pass-through, thinking models, repair limit) follows with the next
  benchmark run.

### Alternatives Considered

- XML tags (raw code without escaping, but no grammar), ReAct `Action:` lines (fragile, no grammar), Hermes
  `<tool_call>` as the canonical format (accepted by the parser, not canonical), and supporting only models with native
  tool calling (contradicts the capability levels).
