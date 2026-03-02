package clock

import (
	"sync"
	"time"
)

// Clock abstracts time for testability.
type Clock interface {
	Now() time.Time
}

// RealClock returns the actual system time.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

// MockClock returns a controllable time for testing.
type MockClock struct {
	mu  sync.Mutex
	now time.Time
}

func NewMockClock(t time.Time) *MockClock {
	return &MockClock{now: t}
}

func (c *MockClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *MockClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

func (c *MockClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
