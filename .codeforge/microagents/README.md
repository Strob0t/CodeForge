# Microagents

Place `.md` files in this directory to define project-specific microagents.
They are automatically loaded on server startup and injected into the system
prompt when their trigger pattern matches user input.

## File Format

```yaml
name: my-agent
type: knowledge
trigger: "pattern"
description: Optional description
---
Prompt content injected when the trigger matches.
```

## Fields

| Field | Required | Values |
|---|---|---|
| `name` | yes | Unique identifier |
| `type` | yes | `knowledge`, `repo`, or `task` |
| `trigger` | yes | Substring (case-insensitive) or regex (prefix with `^` or `(`) |
| `description` | no | Human-readable purpose |

Everything after the `---` separator becomes the prompt text.
