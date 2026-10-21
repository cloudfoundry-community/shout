# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased] — Lisp

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
- `slack-app` handler for Slack Web API (chat.postMessage with Bearer token auth)

### Changed
- Error responses to API clients now return generic messages; details logged server-side only
- Slack payload omits empty fields (aligned with Go behavior)

### Removed
- Travis CI configuration (replaced by CodeQL and Concourse)

## [Unreleased] — Go rewrite

### Added
- Go rewrite of Shout! notification gateway (replaces Common Lisp implementation)
- YAML-based rules engine with expr-lang for condition evaluation
- Built-in notification handlers: Slack, webhook, email
- `slack-app` handler for Slack Web API (chat.postMessage with Bearer token auth)
- Plugin architecture for custom notification handlers
- State persistence with dirty-flag optimization
- State expiry with configurable TTL
- Overnight time window support in `between()` expressions
- Graceful shutdown on SIGTERM/SIGINT
- Docker Compose testbed with mock Slack server
- CodeQL security scanning via GitHub Actions
- Security scanning: gosec, govulncheck, trivy
- Unit tests with race detection (78.7% coverage)
- Integration test suite (12 tests + slack-app tests)
- Semver versioning via build-time ldflags
- The server converts a Lisp Shout! state file when it starts. Before this, the BOSH job moved such a file aside, so topics that were broken at the upgrade lost their reminders and never sent a "fixed" notification. When the state file holds the JSON list the Lisp server wrote, or the bare `null` it wrote with no topics, the server keeps the original beside it as `<file>.lisp-<unix seconds>`, writes the converted state the same way a normal save does, and logs one line with the number of topics it imported. Imported topics get the configured expiry, and a last event whose timestamps the Lisp server lost counts as reported at the import. A file in neither format still stops the server with an error that names the file, and the file is left untouched.
- A `-check-state` flag reports whether the state file can be loaded and exits without changing anything. It prints one line and exits 0 when the file is missing, in the Go format, or a Lisp file that the server will convert. It exits 3 when the file is in neither format, and 1 when the file cannot be read.

### Changed
- Rules format changed from custom Lisp DSL to YAML with expr-lang expressions
- Single static binary replaces SBCL + Quicklisp runtime
- BOSH release simplified (no post-start curl, YAML rules file)

### Fixed
- SIGTERM and SIGINT now save the state file and exit 0. Before this, the server exited with status 1 as soon as the listener closed, and that exit raced the final save, so the last state changes could be lost. Shutdown now drains in-flight requests for up to 20 seconds, cancels any notification still sending, waits up to 5 more seconds, and then writes the state file. A second signal exits at once.

### Removed
- Travis CI configuration (replaced by CodeQL and Concourse)

### Security
- Log lines and API error responses no longer reveal webhook secrets. When a delivery failed, the `slack`, `webhook`, and `slack-app` handlers returned the error from `net/http`, which quotes the full target URL, and the server logged it. For a Slack incoming webhook that URL holds the secret path, so a DNS outage or an egress block wrote every webhook secret to `shout.log`. The handlers now mask the URL in their errors, and the server masks URLs again in every log line and in the error that `POST /rules` returns. The mask hides everything after `/services/` in a path, and it hides the user information, the query, and the fragment of any URL.
