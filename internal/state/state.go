package state

import (
	"sync"
	"time"
)

// Event represents an incoming notification event.
type Event struct {
	Topic      string            `json:"topic"`
	OK         bool              `json:"ok"`
	Message    string            `json:"message"`
	Link       string            `json:"link,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	OccurredAt int64             `json:"occurred_at"`
	ReportedAt int64             `json:"reported_at"`
}

// TopicState tracks the break/fix state machine for a single topic.
type TopicState struct {
	Name           string `json:"name"`
	Status         string `json:"state"`
	LastNotifiedAt int64  `json:"notified"`
	RemindEvery    int64  `json:"reminder,omitempty"`
	PreviousEvent  *Event `json:"previous,omitempty"`
	FirstEvent     *Event `json:"first,omitempty"`
	LastEvent      *Event `json:"last,omitempty"`
}

// IsOK returns true if the topic is in a healthy state.
func (ts *TopicState) IsOK() bool {
	if ts == nil || ts.LastEvent == nil {
		return false
	}
	return ts.LastEvent.OK
}

// NeedsReminder returns true if the topic is broken and the reminder
// interval has elapsed since the last notification.
func (ts *TopicState) NeedsReminder() bool {
	if ts == nil || ts.Status != "broken" || ts.RemindEvery <= 0 {
		return false
	}
	return time.Now().Unix()-ts.LastNotifiedAt >= ts.RemindEvery
}

// Manager holds all topic states in memory with thread-safe access.
type Manager struct {
	mu     sync.RWMutex
	states map[string]*TopicState
	store  Store
}

// NewManager creates a new state manager backed by the given store.
func NewManager(store Store) (*Manager, error) {
	states, err := store.Load()
	if err != nil {
		return nil, err
	}
	if states == nil {
		states = make(map[string]*TopicState)
	}
	return &Manager{states: states, store: store}, nil
}

// Ingest processes an event and returns the transition type:
// "broken", "fixed", or "" (no transition).
func (m *Manager) Ingest(evt *Event) (transition string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().Unix()
	if evt.OccurredAt == 0 {
		evt.OccurredAt = now
	}
	evt.ReportedAt = now

	ts, exists := m.states[evt.Topic]
	if !exists {
		ts = &TopicState{Name: evt.Topic}
		m.states[evt.Topic] = ts
	}

	wasOK := ts.IsOK()

	ts.PreviousEvent = ts.LastEvent
	ts.LastEvent = evt

	switch {
	case !wasOK && !evt.OK && exists:
		// Still broken — no transition.
		return ""
	case wasOK && evt.OK:
		// Still working — no transition.
		ts.Status = "working"
		return ""
	case wasOK && !evt.OK:
		// Was working, now broken.
		ts.Status = "broken"
		ts.FirstEvent = evt
		ts.LastNotifiedAt = now
		return "broken"
	case !wasOK && evt.OK:
		// Was broken, now fixed.
		ts.Status = "fixed"
		ts.LastNotifiedAt = now
		return "fixed"
	default:
		// First event for this topic.
		if evt.OK {
			ts.Status = "working"
			return ""
		}
		ts.Status = "broken"
		ts.FirstEvent = evt
		ts.LastNotifiedAt = now
		return "broken"
	}
}

// Get returns the state for a specific topic.
func (m *Manager) Get(topic string) *TopicState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.states[topic]
}

// All returns a copy of all topic states.
func (m *Manager) All() map[string]*TopicState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]*TopicState, len(m.states))
	for k, v := range m.states {
		out[k] = v
	}
	return out
}

// SetReminder sets the reminder interval for a topic.
func (m *Manager) SetReminder(topic string, seconds int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ts, ok := m.states[topic]; ok {
		ts.RemindEvery = seconds
	}
}

// MarkNotified updates the last notification timestamp for a topic.
func (m *Manager) MarkNotified(topic string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ts, ok := m.states[topic]; ok {
		ts.LastNotifiedAt = time.Now().Unix()
	}
}

// BrokenTopics returns all topics in the "broken" state that need reminders.
func (m *Manager) BrokenTopics() []*TopicState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*TopicState
	for _, ts := range m.states {
		if ts.NeedsReminder() {
			result = append(result, ts)
		}
	}
	return result
}

// Save persists the current state to the store.
func (m *Manager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.store.Save(m.states)
}
