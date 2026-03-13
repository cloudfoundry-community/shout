# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Layout

This is a multi-project workspace for **Shout!**, a notifications gateway server for Concourse CI/CD pipelines. The repository contains two implementations: a Go rewrite (at the repo root) and the original Common Lisp version (in `lisp/`).

### Workspace Level

| Directory | Purpose |
|---|---|
| `shout/` | Main Shout! server (this repo — Go + Lisp) |
| `shout-resource/` | Concourse CI resource type plugin (shell scripts) |
| `shout-boshrelease/` | BOSH release for production deployment |
| `shout-docker-image/` | Base Docker image for Common Lisp |
| `concourse-dockerfiles/` | Concourse CI base images (ubuntu-noble, concourse-cl) |

### This Repository

```
├── cmd/shout/             Go entry point (main.go)
├── internal/              Go internal packages
│   ├── api/               HTTP server (net/http)
│   ├── clock/             Clock interface for testable time
│   ├── engine/            Rules engine (YAML + expr-lang)
│   ├── notify/            Notification backends (Slack, email)
│   └── state/             Per-topic state tracking
├── pkg/version/           Go semver version package
├── lisp/                  Common Lisp implementation
│   ├── *.lisp             Source files (api, rules, slack, shout, packages)
│   ├── test/              Lisp test suite (prove framework)
│   ├── vendor/quicklisp/  Vendored CL dependencies for air-gapped builds
│   ├── Makefile           Standalone Lisp build system
│   └── Dockerfile         Lisp Docker image
├── mock-slack/            Mock Slack webhook receiver (for integration tests)
├── test-client/           Integration test runner (shell scripts)
├── docs/                  Documentation (Slack app setup, etc.)
├── ci/                    CI scripts (version)
├── Makefile               Unified build system (platform selector)
├── version.mk             Shared semver logic (included by both Makefiles)
├── Dockerfile             Go Docker image
├── docker-compose.yml     Go integration test environment
├── rules.example.yml      Go rules file example
└── rules.test.yml         Go integration test rules
```

## Build & Development Commands

A unified Makefile supports both implementations via a platform selector:

```bash
make build                # build both Go and Lisp
make build go             # build Go only
make build lisp           # build Lisp only
```

The `go` / `lisp` selector works with all targets:

| Command | Description |
|---|---|
| `make build [go\|lisp]` | Build binaries |
| `make test [go\|lisp]` | Run test suites |
| `make check [go\|lisp]` | Static analysis (Go: fmt + vet, Lisp: sblint) |
| `make clean [go\|lisp]` | Remove build artifacts |
| `make coverage [go\|lisp]` | Coverage gate (default 50%, COVERAGE_MIN=N, COVERAGE_REPORT=full for HTML) |
| `make release [go\|lisp]` | Build release binaries (Go: cross-compile all platforms, Lisp: no -dev tag) |
| `make security [go\|lisp]` | Security scanning (Go: gosec + govulncheck + trivy, Lisp: sblint + trivy) |
| `make docker [go\|lisp]` | Build Docker image |
| `make help [go\|lisp]` | Show available targets |

### Go-only shortcuts (no platform selector needed)

```bash
make debug-version        # Print resolved version variables
```

### Coverage variables

```bash
make coverage go                        # gate at 50% (default)
make coverage go COVERAGE_MIN=30        # gate at 30%
make coverage go COVERAGE_MIN=0         # show percentage, no gate
make coverage go COVERAGE_REPORT=full   # full HTML report instead of gate
```

### Lisp standalone (from lisp/ directory)

```bash
cd lisp
make build                # Build standalone executable
make test                 # Run prove test suite
make check                # Run sblint static analysis
sbcl --script run.lisp    # Run in development without compiling
```

### Versioning

Both implementations share `version.mk` for semver resolution from git tags. Patch is auto-incremented from the latest tag. Override with `VERSION=x.y.z`. Dev builds get a `-dev` prerelease suffix automatically.

- Go: version injected via `-ldflags -X` at build time
- Lisp: `version.lisp` generated from git tags (auto-generated, gitignored)

## Architecture

### Go Implementation

- **Rules engine**: YAML rules files with [expr-lang/expr](https://github.com/expr-lang/expr) for condition evaluation
- **API**: Same REST endpoints as Lisp version — Concourse resource works unchanged
- **FOR blocks**: All-fire semantics (multiple FORs can match); first-match-wins within each FOR's WHENs
- **Thread safety**: `engineMu sync.RWMutex` protects engine reads/writes
- **State expiry**: `--expiry` flag / `SHOUT_EXPIRY` env (default 86400s, 0=disabled)
- **Notifications**: Slack (webhook), email (net/smtp) — plugin architecture for adding more
- **HTTP client**: `http.Client{Timeout: 30s}` (never uses `http.DefaultClient`)
- **Dependencies**: `expr-lang/expr`, `gopkg.in/yaml.v3` (vendored via `go mod vendor`)

### Lisp Implementation (lisp/)

#### Packages (lisp/packages.lisp)

- **`api`** — HTTP server (Hunchentoot). Endpoints: `/info`, `/events`, `/announcements`, `/rules`, `/state`, `/states`. Entry point: `api:run`.
- **`rules`** — Custom DSL parser and evaluator for notification routing rules. Key exports: `rules:load/rules`, `rules:eval/rules`, `rules:register-plugin`.
- **`slack`** — Slack webhook notification handler. Exports: `slack:send`, `slack:attach`.
- **`shout`** — Main entry point and daemon management. Export: `shout:shout`.

#### Rules DSL

The rules engine (`lisp/rules.lisp`) implements a Lisp-like DSL with:
- Topic matching: literal strings, `*` wildcard, `(is "exact")`, `(matches "regex")` (cl-ppcre)
- Time conditions: `(on weekdays)`, `(from 0800 am to 0500 pm)`, `(after ...)`, `(before ...)`
- Logic: `and`, `or`, `not`, `if`
- Variables: `set`/`value`/`lookup` with map support
- Handlers: `slack` (extensible via `register-plugin`)
- String interpolation: `$topic`, `$status`, `$message`, `$link`, `$[metadata-key]`
- **Deprecated**: `concat` appends a trailing newline after each element

All matching FOR blocks fire (not first-match-wins). WHEN clauses within a FOR use first-match-wins semantics.

#### Build Scripts (lisp/)

- **`compile.lisp`** — Loads Quicklisp, registers with ASDF, loads `:shout`, dumps compressed standalone executable via `save-lisp-and-die`.
- **`test.lisp`** — Same setup, then runs `:shout-test` with `prove:run`.
- **`run.lisp`** — Runs Shout! directly without compiling (development mode).
- **`cover.lisp`** — Runs tests with code coverage instrumentation.

#### Vendored Dependencies & Air-Gapped Builds

Quicklisp dependencies are vendored in `lisp/vendor/quicklisp/` for air-gapped environments.

- When `vendor/quicklisp/` exists, `make quicklisp` and `make libs` copy from vendor instead of downloading.
- To refresh vendored dependencies on a connected machine: `make vendor`
- The vendored directory contains all 32 transitive dependencies (~21MB).

#### Common Lisp Toolchain

- **SBCL** — Compiler and runtime. Must be built with `--fancy` for core compression. Homebrew and Roswell `sbcl_bin` include this.
- **ASDF** — Build system. `.asd` files declare dependencies and source file load order.
- **Quicklisp** — Package manager. Downloads libraries from the Quicklisp dist server.
- **Roswell** — CL implementation manager. Provides pre-built SBCL binaries at github.com/roswell/sbcl_bin/releases.
- **sblint** — Static analysis tool wrapping SBCL compiler diagnostics. Install via `ros install cxxxr/sblint`. Binary at `~/.roswell/bin/sblint`.

#### Dependencies (via Quicklisp)

`hunchentoot` (HTTP server), `drakma` (HTTP client), `cl-json` (JSON), `cl-ppcre` (regex), `daemon` (daemonization), `prove` (testing).

### Event Model

Events carry a `topic`, `ok` status, `message`, `link`, and optional metadata. The server tracks per-topic state transitions (working/broken/fixed) and only sends notifications on transitions or reminder intervals.

### Authentication

HTTP Basic Auth with two tiers:
- **Ops** (`SHOUT_OPS_AUTH` env var) — for `/events`, `/state`, `/states`
- **Admin** (`SHOUT_ADMIN_AUTH` env var) — for `/rules`
- Default credentials: `shout:shout`

### Key Environment Variables

`SHOUT_PORT` (default 7109), `SHOUT_DATABASE` (default `/var/db/shout.db`), `SHOUT_PIDFILE`, `SHOUT_IT_OUT_LOUD` (daemon mode), `SHOUT_WEBHOOK`, `SHOUT_BOTNAME`, `SHOUT_BOTICON`.

### Notification Plugin System (Lisp)

#### How It Works

Notification backends are registered as named functions in `rules.lisp`:

- **`register-plugin`** — Stores a handler function in the `*plugin-handlers*` alist, keyed by symbol.
- **`dispatch-to-plugin`** — Looks up and calls a handler by name during rule evaluation.
- **`registered-plugin?`** — Predicate used during rule parsing to distinguish plugin calls from DSL keywords.

Currently only one plugin is registered: `slack`, which wraps `slack:send` with argument extraction from the rules DSL.

#### Thread Safety Model

Plugin functions are stateless and thread-safe. However, plugins execute inside both `*states-lock*` and `*rules-lock*`, so all notifications are serialized. A slow webhook blocks other state updates and the scan loop. A future optimization could queue notifications and send after releasing locks.

#### Adding a New Notification Backend (Lisp)

1. Create a new file (e.g., `email.lisp`) with a package and send function
2. Add the package to `packages.lisp`
3. Add the file to `shout.asd` components
4. Register the plugin in `api:run` (alongside the Slack registration)
5. Use it in rules: `(email :to "oncall@example.com" :subject "$topic is $status" :body "$message")`
6. Add any new Quicklisp dependency to `shout.asd` `:depends-on` and run `make vendor`

#### Where Configuration Lives

| Layer | File | Purpose |
|-------|------|---------|
| Defaults | Plugin source (e.g., `slack.lisp`) | `env` calls with fallback values |
| Docker | `Dockerfile` `ENV` directives | Build-time defaults |
| Compose | `docker-compose.yml` `environment:` | Development overrides |
| BOSH | `shout-boshrelease/jobs/shout/spec` | Production deployment properties |
| Rules DSL | Rules file loaded via `/rules` | Per-notification overrides (e.g., `:webhook`) |

Environment variables override defaults; per-notification keyword arguments in the rules DSL override everything.

## Testing

### Go

Tests use the standard `testing` package with race detection. 78.7% coverage (engine 95.7%, notify 92%, state 87.8%, clock 100%, api 69%).

```bash
make test go              # fmt + vet + tests with -race
make coverage go          # gate at COVERAGE_MIN (default 50%)
```

### Lisp

Tests live in `lisp/test/` and use the `prove` framework. Test system is `:shout-test`.

```bash
make test lisp            # run prove test suite
make coverage lisp        # gate at COVERAGE_MIN (default 50%)
```

### Integration Tests

Integration tests run against a live Shout! instance and mock-slack (no Docker required):

```bash
# Terminal 1: start mock-slack
python3 mock-slack/server.py

# Terminal 2: start Shout! (Go or Lisp)
./shout --rules rules.test.yml           # Go
cd lisp && sbcl --script run.lisp        # Lisp

# Terminal 3: run tests
./test-shout-slack.sh                    # all tests (webhook + slack-app)
./test-shout-slack.sh --test webhook     # webhook handler only
./test-shout-slack.sh --test slack-app   # slack-app handler only
```

The test script auto-detects Go vs Lisp via `/info` and uses the appropriate rules format.

For testing against real Slack (no mock):
```bash
./test-shout-slack.sh --webhook https://hooks.slack.com/...
./test-shout-slack.sh --token xoxb-... --channel '#test'
```

Docker Compose testbeds also exist (`docker-compose.yml` for Go, `lisp/docker-compose.yml` for Lisp) but are optional.

### mock-slack

A lightweight Python HTTP server in `mock-slack/` that captures webhook payloads:
- `POST /` — Receives Slack webhook payloads
- `POST /api/chat.postMessage` — Receives Slack Web API calls
- `GET /messages` — Returns all captured payloads as JSON
- `DELETE /messages` — Clears captured payloads

### Test State Machine

Shout! tracks per-topic state. The first event on a topic establishes a baseline without notifying. Only state transitions (working->broken, broken->fixed) trigger notifications. Duplicate events in the same state are suppressed.
