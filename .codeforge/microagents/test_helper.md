name: test-helper
type: knowledge
trigger: "test|spec|assert|expect|pytest|jest"
description: Test writing guidance
---
When writing or modifying tests:
1. Use table-driven tests (Go) or parametrize (Python) for multiple cases.
2. Test happy path, error paths, and edge cases (nil, empty, boundary values).
3. Use clear test names that describe the expected behavior.
4. Prefer testing behavior over implementation details.
5. Keep tests independent — no shared mutable state between test cases.
6. Assert specific error types and messages, not just "error occurred".
