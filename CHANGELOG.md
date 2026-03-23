# Changelog

All notable changes to CodeForge are documented in this file.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html)

## [0.8.0] - 2026-03-23

### Added
- Visual Design Canvas with SVG tools and multimodal pipeline (Phase 32)
- Contract-First Review/Refactor pipeline (Phase 31)
- Benchmark and Evaluation system (Phase 26+28)
- Security and Trust annotations (Phase 23)
- Real-Time Channels for team collaboration
- Chat enhancements: HITL permission UI, inline diff review, action buttons
- Hybrid routing with ComplexityAnalyzer and MAB model selection
- Agentic conversation loop with 7 built-in tools
- A2A protocol support (v0.3.0)
- MCP server with project/cost resources

### Security
- HSTS header on all responses
- Docker images pinned to specific versions
- cap_drop ALL on production containers
- Pre-deploy environment validation script
- Trust annotations with 4 levels on NATS messages

### Fixed
- Python exception handlers now log error details
- Bare Go error returns wrapped with context
- WebSocket endpoint rate-limited
- PostgreSQL archive command error handling
