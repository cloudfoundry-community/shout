package notify

import "context"

// Handler is the interface that notification backends implement.
type Handler interface {
	// Name returns the handler's registered name (e.g., "slack").
	Name() string

	// Send delivers a notification with the given arguments.
	// The args map contains evaluated string values (after interpolation).
	Send(ctx context.Context, args map[string]string) error
}

// Registry manages handler registration and lookup.
type Registry struct {
	handlers map[string]Handler
}

// NewRegistry creates a new handler registry.
func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]Handler)}
}

// Register adds a handler to the registry.
func (r *Registry) Register(h Handler) {
	r.handlers[h.Name()] = h
}

// Get retrieves a handler by name.
func (r *Registry) Get(name string) (Handler, bool) {
	h, ok := r.handlers[name]
	return h, ok
}

// List returns all registered handler names.
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		names = append(names, name)
	}
	return names
}
