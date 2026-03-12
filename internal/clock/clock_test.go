package clock

import (
	"testing"
	"time"
)

func TestRealClock(t *testing.T) {
	c := RealClock{}
	before := time.Now()
	now := c.Now()
	after := time.Now()

	if now.Before(before) || now.After(after) {
		t.Errorf("RealClock.Now() = %v, expected between %v and %v", now, before, after)
	}
}

func TestMockClock(t *testing.T) {
	fixed := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	c := NewMockClock(fixed)

	if !c.Now().Equal(fixed) {
		t.Errorf("expected %v, got %v", fixed, c.Now())
	}
}

func TestMockClockSet(t *testing.T) {
	c := NewMockClock(time.Now())
	target := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	c.Set(target)

	if !c.Now().Equal(target) {
		t.Errorf("expected %v after Set, got %v", target, c.Now())
	}
}

func TestMockClockAdvance(t *testing.T) {
	start := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	c := NewMockClock(start)
	c.Advance(5 * time.Minute)

	expected := start.Add(5 * time.Minute)
	if !c.Now().Equal(expected) {
		t.Errorf("expected %v after Advance, got %v", expected, c.Now())
	}
}
