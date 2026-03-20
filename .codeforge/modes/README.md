# Custom Modes

Place `.yaml` files in this directory to define custom agent modes.
They are loaded on server startup and registered alongside the built-in modes.

## File Format

```yaml
id: my_custom_mode
name: My Custom Mode
description: What this mode does
tools:
  - Read
  - Write
  - Edit
  - Glob
  - Grep
llm_scenario: default
autonomy: 3
prompt_prefix: |
  You are a specialist in ...
```

## Fields

| Field | Required | Values |
|---|---|---|
| `id` | yes | snake_case identifier (must not conflict with built-in modes) |
| `name` | yes | Display name |
| `description` | yes | What this mode does |
| `tools` | yes | List of allowed tools: Read, Write, Edit, Bash, Search, Glob, ListDir |
| `llm_scenario` | no | default, background, think, longContext, review, plan |
| `autonomy` | yes | 1-5 (1=supervised, 5=headless) |
| `prompt_prefix` | no | System prompt injected for this mode |

## Built-in Tool Names

Read, Write, Edit, Bash, Search, Glob, ListDir

## Naming Convention

Mode IDs use snake_case: `my_custom_mode`, not `my-custom-mode`.
