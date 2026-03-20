name: error-helper
type: knowledge
trigger: "error|exception|panic|traceback|stack trace"
description: Helps debug errors systematically
---
When encountering an error, follow this approach:
1. Read the full error message and stack trace carefully.
2. Identify the origin file and line number.
3. Check recent changes to that file (git log/diff).
4. Form a hypothesis about the root cause before making changes.
5. Apply a minimal, targeted fix and verify it resolves the error.
Do not suppress errors silently. Fix the root cause.
