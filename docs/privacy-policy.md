# Privacy & LLM Data Processing Notice

## What Data is Processed

CodeForge processes the following data categories:
- **User accounts:** Email, name, role, password hash
- **Code and prompts:** Source code, natural language instructions
- **Agent activity:** Tool calls, conversation history, cost records
- **Infrastructure:** IP addresses (in audit logs), session tokens

## LLM Provider Data Processing

When you use CodeForge, your code and prompts may be sent to external
LLM providers for processing. The specific provider depends on your
configuration:

| Provider   | Data Sent          | Data Retention by Provider |
|------------|--------------------|-----------------------------|
| OpenAI     | Prompts, code      | See OpenAI data policy      |
| Anthropic  | Prompts, code      | See Anthropic data policy   |
| Ollama     | Prompts, code      | Local only -- no external   |
| LM Studio  | Prompts, code      | Local only -- no external   |

### Opting Out of External LLM Processing

Configure CodeForge to use local models only (Ollama, LM Studio)
to ensure no data leaves your infrastructure.

## Browser Storage

### Session Cookie

| Name | Type | Purpose | Duration | Scope |
|------|------|---------|----------|-------|
| `codeforge_refresh` | HttpOnly cookie | Authentication refresh token | 7 days | `/api/v1/auth` |

This cookie is strictly necessary for authentication. No consent required under ePrivacy Directive Art. 5(3).

### localStorage (UI Preferences)

The following keys are stored in browser localStorage for UI state persistence.
They contain no personal data and are never transmitted to the server.

| Key Pattern | Purpose |
|-------------|---------|
| `codeforge-theme` | UI theme selection |
| `codeforge-sidebar-collapsed` | Sidebar layout state |
| `codeforge-i18n-locale` | Language preference |
| `codeforge-shortcuts` | Custom keyboard shortcuts |
| `codeforge-notification-settings` | Notification preferences |
| `codeforge-onboarding-completed` | Onboarding wizard state |
| `codeforge-frequency-tracker-*` | Autocomplete frequency data |
| `codeforge-split-ratio-*` | Panel layout per project |

localStorage data can be cleared via browser developer tools.

## Your Rights (GDPR)

- **Access:** View your data via the dashboard or API
- **Export:** `POST /api/v1/users/{id}/export`
- **Deletion:** `DELETE /api/v1/users/{id}/data`
- **Restriction:** Configure local-only models to prevent external processing
