# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
