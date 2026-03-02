# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased] — Lisp (develop branch)

### Added
- Hunchentoot read/write timeouts (30s) to prevent slow-client resource exhaustion
- Request body size limit (1MB) to prevent memory exhaustion via oversized payloads
- Constant-time password comparison to prevent timing side-channel attacks
- Database file permissions set to 0600 after every write
- Webhook URL scheme validation (http/https only)
- HTTP client connection timeout (30s) for outbound Slack notifications
- Semver version maintenance via Makefile (git-tag-derived, matching Go approach)
- Expanded `/info` endpoint with build metadata (version, release, build-date, commit)
- SECURITY.md, CONTRIBUTING.md, CHANGELOG.md for OpenSSF compliance

### Changed
- Error responses to API clients now return generic messages; details logged server-side only

### Removed
- Travis CI configuration (replaced by CodeQL and Concourse)

## [Unreleased] — Go rewrite (norm/go-shout branch)

### Added
- Go rewrite of Shout! notification gateway (replaces Common Lisp implementation)
- YAML-based rules engine with expr-lang for condition evaluation
- Built-in notification handlers: Slack, webhook, email
- Plugin architecture for custom notification handlers
- State persistence with dirty-flag optimization
- State expiry with configurable TTL
- Overnight time window support in `between()` expressions
- Graceful shutdown on SIGTERM/SIGINT
- Docker Compose testbed with mock Slack server
- CodeQL security scanning via GitHub Actions
- Security scanning: gosec, govulncheck, trivy
- Unit tests with race detection (78.7% coverage)
- Integration test suite (12 tests)
- Semver versioning via build-time ldflags

### Changed
- Rules format changed from custom Lisp DSL to YAML with expr-lang expressions
- Single static binary replaces SBCL + Quicklisp runtime
- BOSH release simplified (no post-start curl, YAML rules file)

### Removed
- Common Lisp implementation (available on `develop` branch)
- Travis CI configuration (replaced by CodeQL and Concourse)
