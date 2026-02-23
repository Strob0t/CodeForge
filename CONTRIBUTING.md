# Contributing to CodeForge

Thank you for your interest in contributing to CodeForge.

## Development Setup

See [docs/dev-setup.md](docs/dev-setup.md) for environment setup instructions.

## Branch Strategy

- **`main`** — stable release branch, no direct commits
- **`staging`** — active development branch, all work happens here
- Feature branches should be based on `staging`

## Commit Guidelines

- All commit messages must be in **English**
- Use conventional commit style: `feat:`, `fix:`, `chore:`, `docs:`, `refactor:`, `test:`
- Run `pre-commit run --all-files` before committing
- Always push to remote after committing

## Code Standards

### Go (Core Service)

- Format: `gofmt` + `goimports`
- Lint: `golangci-lint`
- Test: `go test -race ./...`
- No `interface{}` / `any` unless unavoidable

### Python (AI Workers)

- Format + Lint: `ruff`
- Test: `pytest`
- Package management: Poetry
- No `Any` type unless unavoidable

### TypeScript (Frontend)

- Format: Prettier
- Lint: ESLint
- Build: Vite + SolidJS
- No `any` type unless unavoidable

## Configuration

- **YAML for all configuration files** (supports comments)
- JSON only for API responses and internal data exchange

## Documentation

Every code change must update relevant documentation:

- `docs/todo.md` — mark completed tasks, add new discoveries
- `docs/architecture.md` — for structural changes
- `docs/features/*.md` — for feature-specific changes
- `docs/dev-setup.md` — for new directories, ports, tooling

See [CLAUDE.md](CLAUDE.md) for the full documentation policy.

## Testing

- Each new feature requires unit tests
- Integration tests for database and NATS interactions
- Frontend E2E tests with Playwright
- Run the full test suite before submitting a PR

## Security

- Read [SECURITY.md](SECURITY.md) for security practices
- Never commit secrets, credentials, or API keys
- Use generic error messages in HTTP responses (log details server-side)
- Validate all user input at system boundaries

## Pull Requests

1. Create a feature branch from `staging`
2. Make your changes with tests
3. Run pre-commit hooks
4. Update documentation
5. Open a PR against `staging`
6. Ensure CI passes
