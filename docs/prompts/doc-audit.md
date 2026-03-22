# Documentation Audit Prompt for Claude Code

Copy the prompt block below and paste it into a Claude Code session opened at the CodeForge project root.

---

## Prompt

```
You are a documentation auditor for this codebase. Your job: make every document in this repository accurately reflect the current state of the code. The code is the source of truth — never change code to match docs.

## Starting state
- A codebase with documentation spread across: docs/, README.md, CLAUDE.md, inline code comments, API docs, feature specs, architecture docs, and any other .md files
- Docs may be outdated, incomplete, or contradicting the actual implementation

## Task
Audit ALL documentation files against the actual codebase. For every doc, verify that:
- Described features, APIs, endpoints, configs, and behaviors match the implementation
- File paths, function names, class names, and module references are correct and exist
- Architecture descriptions match the actual code structure
- Status claims (implemented, planned, TODO) match reality
- Code examples and snippets compile/run against the current codebase
- Environment variables, ports, CLI flags, and defaults are accurate
- Dependency lists and version numbers match package manifests

## Execution rules
1. Read each documentation file completely before auditing
2. For every claim in a doc, locate the corresponding code — grep, glob, read the source
3. Collect ALL mismatches before making any edits — do not fix one file at a time blindly
4. After the full audit: output a summary table of all findings (file, line, issue, status)
5. Then fix each documentation file to match the code. NEVER modify source code.
6. After all fixes: re-read each changed file to verify your edits are correct

## Output after each step
After auditing each file: [file audited] — N issues found
After all fixes: output the full summary table with columns: File | Issue | Fix Applied

## Constraints
- MUST NOT change any source code, configs, or non-documentation files
- MUST NOT delete documentation sections — update them or mark as deprecated with date
- MUST NOT invent features that don't exist in code — if a doc describes something unimplemented, mark it as `<!-- NOT IMPLEMENTED as of YYYY-MM-DD -->`
- MUST preserve the original document structure and formatting style
- If a file has zero issues, report it as clean — do not skip it silently
- Stop and ask before: deleting any file, restructuring doc directories, or changing CLAUDE.md
```
