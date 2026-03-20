name: commit-helper
type: knowledge
trigger: "commit|git add|staged"
description: Commit message guidance
---
When creating commits:
1. Use conventional commit format: `type(scope): description`
2. Types: feat, fix, refactor, docs, test, chore, perf, ci
3. Keep the subject line under 72 characters.
4. Describe the "why" not the "what" — the diff shows what changed.
5. Reference issue IDs when applicable.
6. Never commit secrets, .env files, or generated artifacts.
