# Shout! Plugin Architecture

Shout! uses a simple handler-based plugin architecture for notification backends. Each handler implements a small interface and is registered at startup.

## Handler Interface

```go
package notify

import "context"

type Handler interface {
    // Name returns the handler's registered name (e.g., "slack", "email").
    // This name is used in YAML rules to route notifications.
    Name() string

    // Send delivers a notification with the given arguments.
    // The args map contains string key-value pairs produced by
    // template evaluation from the rules engine.
    Send(ctx context.Context, args map[string]string) error
}
```

## Registry

Handlers are managed by a `Registry`:

```go
registry := notify.NewRegistry()
registry.Register(myHandler)

h, ok := registry.Get("myhandler")  // lookup by name
names := registry.List()            // all registered names
```

Handlers are registered once at startup in `cmd/shout/main.go`. The registry is read-only after initialization, so no mutex is needed.

## Thread Safety

Handlers **must be safe for concurrent `Send()` calls**. The rules engine fires notifications from HTTP request goroutines and the background scan loop concurrently. Stateless handlers (like the built-in slack and webhook handlers) are inherently safe. If your handler has mutable state, protect it with a mutex.

## Argument Contract

The `args map[string]string` passed to `Send()` comes from Go template evaluation defined in the YAML rules. Template context fields available:

| Field | Type | Description |
|-------|------|-------------|
| `.Topic` | string | Event topic name |
| `.Status` | string | Transition status ("now broken", "now fixed", etc.) |
| `.Message` | string | Event message |
| `.Link` | string | Event link URL |
| `.OK` | bool | Whether the event is healthy |
| `.Meta` | map[string]string | Event metadata key-value pairs |
| `.Vars` | map[string]any | Variables defined in YAML rules |

Template functions:
- `lookup .Vars.mapname "key1" "key2"` — looks up keys in a map with fallback

## Adding a Handler: Step-by-Step

### 1. Create the handler file

Create `internal/notify/myhandler.go`:

```go
package notify

import (
    "context"
    "fmt"
    "os"
)

type MyHandler struct {
    apiKey string
}

func NewMyHandler() *MyHandler {
    return &MyHandler{
        apiKey: os.Getenv("SHOUT_MYHANDLER_API_KEY"),
    }
}

func (h *MyHandler) Name() string { return "myhandler" }

func (h *MyHandler) Send(ctx context.Context, args map[string]string) error {
    target := args["target"]
    if target == "" {
        return fmt.Errorf("myhandler: missing 'target' argument")
    }
    message := args["message"]

    // ... send notification using h.apiKey ...

    return nil
}
```

### 2. Register in main.go

```go
handlers.Register(notify.NewMyHandler())
```

### 3. Use in YAML rules

```yaml
rules:
  - for: "*"
    when:
      - match: "*"
        do:
          - myhandler:
              target: "ops-channel"
              message: "{{ .Topic }} is {{ .Status }}: {{ .Message }}"
```

## Configuration Pattern

- **Environment variables** for infrastructure config (API keys, hostnames, ports) — set once per deployment
- **YAML rule arguments** for per-notification overrides (recipients, message text, colors) — vary per rule

Built-in handlers follow this pattern:
- `slack`: `SHOUT_WEBHOOK` (env, optional) vs `webhook`, `text`, `color` (YAML args)
- `email`: `SHOUT_SMTP_HOST`, `SHOUT_SMTP_PORT`, `SHOUT_EMAIL_FROM` (env) vs `to`, `subject`, `body` (YAML args)
- `webhook`: no env config — `url` and payload fields all in YAML args

## Built-in Handlers

| Handler | Required Args | Optional Args |
|---------|--------------|---------------|
| `slack` | `webhook` | `text`, `color`, `username`, `icon_url`, `attach` |
| `webhook` | `url` | any key-value pairs (sent as JSON body) |
| `email` | `to` | `subject`, `body` |
