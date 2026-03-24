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

## Your Rights (GDPR)

- **Access:** View your data via the dashboard or API
- **Export:** `POST /api/v1/users/{id}/export`
- **Deletion:** `DELETE /api/v1/users/{id}/data`
- **Restriction:** Configure local-only models to prevent external processing
