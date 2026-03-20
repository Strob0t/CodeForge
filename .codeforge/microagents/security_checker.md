name: security-checker
type: knowledge
trigger: "auth|password|token|secret|credential|injection|sanitize|encrypt"
description: Security awareness for sensitive operations
---
When working with security-sensitive code:
1. Never hardcode secrets, tokens, or passwords — use environment variables.
2. Validate and sanitize all user input at system boundaries.
3. Use parameterized queries — never string-interpolate SQL.
4. Check for path traversal when handling file paths from user input.
5. Apply the principle of least privilege for permissions and access control.
6. Log security events but never log secrets or credentials.
