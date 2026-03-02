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
| `sbcl/` | SBCL compiler source — built from source for multi-platform support (cross-platform builds are non-trivial and error-prone) |
| `sbcl-install/` | SBCL installation directory |

## Build & Development Commands

All commands run from `shout/`:

```bash
make quicklisp   # Set up Quicklisp (uses vendored deps if available, otherwise downloads)
make libs        # Install Lisp dependencies
make shout       # Build executable (requires quicklisp + libs)
make test        # Run test suite (prove framework)
make coverage    # Run coverage analysis
make docker      # Build Docker image (linux/amd64)
make vendor      # Refresh vendored dependencies (run on a connected machine)
make clean       # Remove build artifacts
```

### Go Rewrite (branch `norm/go-shout`)

```bash
make -f Makefile.go build          # Build for current platform
make -f Makefile.go test           # Run tests
make -f Makefile.go all-platforms  # Cross-compile linux/darwin amd64/arm64
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
- Topic matching: literal strings, `*` wildcard, `(is "exact")`, `(matches "regex")` (cl-ppcre)
- Time conditions: `(on weekdays)`, `(from 0800 am to 0500 pm)`, `(after ...)`, `(before ...)`
- Logic: `and`, `or`, `not`, `if`
- Variables: `set`/`value`/`lookup` with map support
- Handlers: `slack` (extensible via `register-plugin`)
- String interpolation: `$topic`, `$status`, `$message`, `$link`, `$[metadata-key]`
- **Deprecated**: `concat` appends a trailing newline after each element; this will change in a future major release

All matching FOR blocks fire (not first-match-wins) — this allows multiple notification channels for one topic. WHEN clauses within a FOR use first-match-wins semantics.

### Notification Plugin System

#### How It Works

Notification backends are registered as named functions in `rules.lisp`:

- **`register-plugin`** — Stores a handler function in the `*plugin-handlers*` alist, keyed by symbol. Called at startup in `api:run`.
- **`dispatch-to-plugin`** — Looks up and calls a handler by name during rule evaluation. Triggered when a plugin name (e.g., `slack`) appears in a WHEN clause body.
- **`registered-plugin?`** — Predicate used during rule parsing to distinguish plugin calls from DSL keywords.

Currently only one plugin is registered: `slack`, which wraps `slack:send` with argument extraction from the rules DSL.

#### Thread Safety Model

**Plugin functions are stateless and thread-safe.** `slack:send` uses only local variables and `drakma:http-request` creates independent socket connections per call. New plugins (email, SMS, pager) do **not** need their own locks.

**However**, plugins currently execute inside both locks:

```
POST /events
  → with-lock-held (*states-lock*)     ← held during entire request
      → set-state → ingest-event → trigger-edge
          → notify-about-state
              → with-lock-held (*rules-lock*)   ← also held
                  → rules:eval/rules
                      → dispatch-to-plugin → slack:send  ← HTTP call inside BOTH locks
```

The background scan loop has the same pattern — reminder notifications fire inside `*states-lock*`.

**Implications:** All notifications are serialized. A slow webhook (e.g., 3s timeout) blocks all other state updates and the scan loop. Adding multiple notification backends on the same event compounds the problem. A future optimization could move plugin dispatch outside the lock scope (queue notifications, release locks, then send), since plugins only need their evaluated arguments, not shared state.

#### Adding a New Notification Backend

1. **Create a new file** (e.g., `email.lisp`) with a package and send function:

```lisp
;;; email.lisp — Email notification plugin for Shout!
(in-package :email)

(defun env (name default)
  (or (sb-unix::posix-getenv name) default))

(defun send (body &key (to      (env "SHOUT_EMAIL_TO" ""))
                       (from    (env "SHOUT_EMAIL_FROM" "shout@example.com"))
                       (subject "Shout! Notification")
                       (smtp    (env "SHOUT_SMTP_HOST" "localhost")))
  (when (equal to "")
    (api:shout-log "email" "no recipient configured, skipping notification")
    (return-from send nil))
  (handler-case
    ;; Replace with actual SMTP implementation (e.g., cl-smtp)
    (progn
      (api:shout-log "email" "sending to ~A via ~A" to smtp)
      ;; ... send email here ...
      )
    (error (e)
      (api:shout-log "email" "failed to send: ~A" e)
      nil)))
```

2. **Add the package** to `packages.lisp`:

```lisp
(defpackage :email
  (:use :cl)
  (:export :send))
```

3. **Add the file** to `shout.asd` components:

```lisp
(:file "email" :depends-on ("packages"))
```

4. **Register the plugin** in `api:run` (in `api.lisp`, alongside the Slack registration):

```lisp
(rules:register-plugin
  'rules::email
  #'(lambda (args)
      (email:send
        (arg args :body)
        :to      (arg args :to)
        :subject (arg args :subject))))
```

5. **Use it in rules**:

```
(for *)
  (when (broken)
    (email :to "oncall@example.com"
           :subject "$topic is $status"
           :body "$message"))
```

6. **Add any new Quicklisp dependency** (e.g., `cl-smtp`) to `shout.asd` `:depends-on` and run `make vendor` to refresh vendored deps for air-gapped builds.

#### Where Configuration Lives

Plugin configuration follows the existing pattern of environment variables:

| Layer | File | Purpose |
|-------|------|---------|
| Defaults | Plugin source (e.g., `slack.lisp`) | `env` calls with fallback values |
| Docker | `Dockerfile` `ENV` directives | Build-time defaults |
| Compose | `docker-compose.yml` `environment:` | Development overrides |
| BOSH | `shout-boshrelease/jobs/shout/spec` | Production deployment properties |
| Rules DSL | Rules file loaded via `/rules` | Per-notification overrides (e.g., `:webhook`) |

Environment variables override defaults; per-notification keyword arguments in the rules DSL override everything.

### Authentication

HTTP Basic Auth with two tiers:
- **Ops** (`SHOUT_OPS_AUTH` env var) — for `/events`, `/state`, `/states`
- **Admin** (`SHOUT_ADMIN_AUTH` env var) — for `/rules`
- Default credentials: `shout:shout`

### Key Environment Variables

`SHOUT_PORT` (default 7109), `SHOUT_DATABASE` (default `/var/db/shout.db`), `SHOUT_PIDFILE`, `SHOUT_IT_OUT_LOUD` (daemon mode), `SHOUT_WEBHOOK`, `SHOUT_BOTNAME`, `SHOUT_BOTICON`.

### Common Lisp Toolchain

- **SBCL** (Steel Bank Common Lisp) — The Common Lisp compiler and runtime. Compiles Lisp to native machine code. `sb-ext:save-lisp-and-die` dumps the entire runtime + application into a single standalone executable (see `compile.lisp`). Requires `--fancy` build for core compression support.
- **ASDF** (Another System Definition Facility) — The Common Lisp build system (analogous to Make for C). `.asd` files (`shout.asd`, `shout-test.asd`) declare system metadata, dependencies, and source file load order. ASDF compiles and loads systems but does not fetch packages from the internet. It finds systems via `asdf:*central-registry*` (list of directories to search for `.asd` files).
- **Quicklisp** — The Common Lisp package manager (analogous to pip or npm). Downloads libraries and their transitive dependencies from the Quicklisp dist server. Integrates with ASDF — once Quicklisp fetches a library, ASDF handles building it. Setup via `(load "build/quicklisp/setup.lisp")`.
- **Roswell** — A Common Lisp implementation manager (analogous to nvm or pyenv). Its `sbcl_bin` project provides pre-built SBCL binaries with `--fancy` for all platforms (linux/darwin, amd64/arm64) at github.com/roswell/sbcl_bin/releases.

### Dependencies (via Quicklisp)

`hunchentoot` (HTTP server), `drakma` (HTTP client), `cl-json` (JSON), `daemon` (daemonization), `prove` (testing).

### Build Scripts

- **`compile.lisp`** — Loads Quicklisp, registers the current directory with ASDF, loads the `:shout` system, and dumps a compressed standalone executable via `save-lisp-and-die`.
- **`test.lisp`** — Same setup, then runs the `:shout-test` system with `prove:run`.
- **`run.lisp`** — Runs Shout! directly without compiling to an executable (development mode).
- **`cover.lisp`** — Runs tests with code coverage instrumentation.

### Vendored Dependencies & Air-Gapped Builds

Quicklisp dependencies are vendored in `vendor/quicklisp/` for air-gapped environments where no internet access is available during build/deployment.

- When `vendor/quicklisp/` exists, `make quicklisp` and `make libs` copy from vendor instead of downloading.
- To refresh vendored dependencies on a connected machine: `make vendor`
- The vendored directory contains all 32 transitive dependencies (~21MB).

For air-gapped deployment, the repo (with `vendor/quicklisp/`) and an SBCL binary are the only external artifacts needed. These can be transferred to internal object storage (Minio, Artifactory) for use in disconnected environments.

### SBCL Requirements

SBCL must be built with `--fancy` or `--with-sb-core-compression` for compressed core images. Homebrew SBCL and Roswell `sbcl_bin` releases include these features. Default SourceForge binaries do not.

## Testing

Tests live in `shout/test/` and use the `prove` framework. Test packages are defined in `shout/test/packages.lisp`. The test system is named `:shout-test` and runs via `prove:run`.
