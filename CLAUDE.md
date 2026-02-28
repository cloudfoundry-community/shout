# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Layout

This is a multi-project workspace for **Shout!**, a notifications gateway server for Concourse CI/CD pipelines, written in Common Lisp (SBCL).

| Directory | Purpose |
|---|---|
| `shout/` | Main Shout! server application |
| `shout-resource/` | Concourse CI resource type plugin (shell scripts) |
| `shout-boshrelease/` | BOSH release for production deployment |
| `shout-docker-image/` | Base Docker image for Common Lisp |
| `sbcl/` | Steel Bank Common Lisp compiler source |
| `sbcl-install/` | SBCL installation directory |

## Build & Development Commands

All commands run from `shout/`:

```bash
make quicklisp   # Install Quicklisp package manager (first-time setup)
make libs        # Download Lisp dependencies
make shout       # Build executable (requires quicklisp + libs)
make test        # Run test suite (prove framework)
make coverage    # Run coverage analysis
make docker      # Build Docker image (linux/amd64)
make clean       # Remove build artifacts
```

Run in development without compiling:
```bash
sbcl --script run.lisp
```

## Architecture

### Packages (shout/packages.lisp)

- **`api`** — HTTP server (Hunchentoot). Endpoints: `/info`, `/events`, `/announcements`, `/rules`, `/state`, `/states`. Entry point: `api:run`.
- **`rules`** — Custom DSL parser and evaluator for notification routing rules. Key exports: `rules:load/rules`, `rules:eval/rules`, `rules:register-plugin`.
- **`slack`** — Slack webhook notification handler. Exports: `slack:send`, `slack:attach`.
- **`shout`** — Main entry point and daemon management. Export: `shout:shout`.

### Event Model

Events carry a `topic`, `ok` status, `message`, `link`, and optional metadata. The server tracks per-topic state transitions (working/broken/fixed) and only sends notifications on transitions or reminder intervals.

### Rules DSL

The rules engine (`shout/rules.lisp`) implements a Lisp-like DSL with:
- Topic matching: literal strings, `*` wildcard, `(~ "glob*")`, `(re "regex")`
- Time conditions: `(on weekdays)`, `(from 0800 am to 0500 pm)`, `(after ...)`, `(before ...)`
- Logic: `and`, `or`, `not`, `if`
- Variables: `set`/`value`/`lookup` with map support
- Handlers: `slack` (extensible via `register-plugin`)
- String interpolation: `$topic`, `$status`, `$message`, `$link`, `$[metadata-key]`

FOR clauses match topics; WHEN clauses match time conditions. Both use first-match-wins semantics.

### Authentication

HTTP Basic Auth with two tiers:
- **Ops** (`SHOUT_OPS_AUTH` env var) — for `/events`, `/state`, `/states`
- **Admin** (`SHOUT_ADMIN_AUTH` env var) — for `/rules`
- Default credentials: `shout:shout`

### Key Environment Variables

`SHOUT_PORT` (default 7109), `SHOUT_DATABASE` (default `/var/db/shout.db`), `SHOUT_PIDFILE`, `SHOUT_IT_OUT_LOUD` (daemon mode), `SHOUT_WEBHOOK`, `SHOUT_BOTNAME`, `SHOUT_BOTICON`.

### Dependencies (Quicklisp)

`hunchentoot` (HTTP server), `drakma` (HTTP client), `cl-json` (JSON), `daemon` (daemonization), `prove` (testing).

## Testing

Tests live in `shout/test/` and use the `prove` framework. Test packages are defined in `shout/test/packages.lisp`. The test system is named `:shout-test` and runs via `prove:run`.
