package state

import (
	"sync"
	"testing"
	"time"
)

func newTestManager(t *testing.T, opts ...ManagerOption) *Manager {
	t.Helper()
	store := &memStore{states: make(map[string]*TopicState)}
	mgr, err := NewManager(store, opts...)
	if err != nil {
		t.Fatalf("NewManager() error: %v", err)
	}
	return mgr
}

type memStore struct {
	states map[string]*TopicState
	saves  int
}

func (s *memStore) Load() (map[string]*TopicState, error) {
	return s.states, nil
}

func (s *memStore) Save(states map[string]*TopicState) error {
	s.saves++
	return nil
}

func TestFirstEventWorkingNoTransition(t *testing.T) {
	mgr := newTestManager(t)
	transition := mgr.Ingest(&Event{Topic: "test", OK: true, Message: "ok"})
	if transition != "" {
		t.Errorf("first working event should not transition, got %q", transition)
	}
	ts := mgr.Get("test")
	if ts.Status != "working" {
		t.Errorf("expected status=working, got %q", ts.Status)
	}
}

func TestFirstEventBrokenTransitions(t *testing.T) {
	mgr := newTestManager(t)
	transition := mgr.Ingest(&Event{Topic: "test", OK: false, Message: "fail"})
	if transition != "broken" {
		t.Errorf("first broken event should transition to broken, got %q", transition)
	}
	ts := mgr.Get("test")
	if ts.Status != "broken" {
		t.Errorf("expected status=broken, got %q", ts.Status)
	}
}

func TestWorkingToBrokenTransition(t *testing.T) {
	mgr := newTestManager(t)
	mgr.Ingest(&Event{Topic: "test", OK: true, Message: "ok"})

	transition := mgr.Ingest(&Event{Topic: "test", OK: false, Message: "fail"})
	if transition != "broken" {
		t.Errorf("expected broken transition, got %q", transition)
	}
}

func TestBrokenToFixedTransition(t *testing.T) {
	mgr := newTestManager(t)
	mgr.Ingest(&Event{Topic: "test", OK: true, Message: "ok"})
	mgr.Ingest(&Event{Topic: "test", OK: false, Message: "fail"})

	transition := mgr.Ingest(&Event{Topic: "test", OK: true, Message: "fixed"})
	if transition != "fixed" {
		t.Errorf("expected fixed transition, got %q", transition)
	}
}

func TestStillBrokenNoTransition(t *testing.T) {
	mgr := newTestManager(t)
	mgr.Ingest(&Event{Topic: "test", OK: true, Message: "ok"})
	mgr.Ingest(&Event{Topic: "test", OK: false, Message: "fail"})

	transition := mgr.Ingest(&Event{Topic: "test", OK: false, Message: "still fail"})
	if transition != "" {
		t.Errorf("still broken should not transition, got %q", transition)
	}
}

func TestStillWorkingNoTransition(t *testing.T) {
	mgr := newTestManager(t)
	mgr.Ingest(&Event{Topic: "test", OK: true, Message: "ok"})

	transition := mgr.Ingest(&Event{Topic: "test", OK: true, Message: "still ok"})
	if transition != "" {
		t.Errorf("still working should not transition, got %q", transition)
	}
}

func TestNeedsReminder(t *testing.T) {
	mgr := newTestManager(t)
	mgr.Ingest(&Event{Topic: "test", OK: true, Message: "ok"})
	mgr.Ingest(&Event{Topic: "test", OK: false, Message: "fail"})
	mgr.SetReminder("test", 1) // 1 second

	ts := mgr.Get("test")
	// Set last notified to the past so NeedsReminder fires
	ts.LastNotifiedAt = time.Now().Unix() - 2

	if !ts.NeedsReminder() {
		t.Error("expected NeedsReminder() = true")
	}
}

func TestNeedsReminderNotBroken(t *testing.T) {
	mgr := newTestManager(t)
	mgr.Ingest(&Event{Topic: "test", OK: true, Message: "ok"})
	mgr.SetReminder("test", 1)

	ts := mgr.Get("test")
	if ts.NeedsReminder() {
		t.Error("working topic should not need reminder")
	}
}

func TestNeedsReminderNoInterval(t *testing.T) {
	mgr := newTestManager(t)
	mgr.Ingest(&Event{Topic: "test", OK: true, Message: "ok"})
	mgr.Ingest(&Event{Topic: "test", OK: false, Message: "fail"})

	ts := mgr.Get("test")
	if ts.NeedsReminder() {
		t.Error("no remind interval should not need reminder")
	}
}

func TestExpiry(t *testing.T) {
	mgr := newTestManager(t, WithExpiry(1))
	mgr.Ingest(&Event{Topic: "test", OK: true, Message: "ok"})

	// Manually backdate the event
	ts := mgr.Get("test")
	ts.LastEvent.ReportedAt = time.Now().Unix() - 10

	mgr.Expire()

	if mgr.Get("test") != nil {
		t.Error("expected topic to be expired")
	}
}

func TestExpiryNotExpired(t *testing.T) {
	mgr := newTestManager(t, WithExpiry(3600))
	mgr.Ingest(&Event{Topic: "test", OK: true, Message: "ok"})

	mgr.Expire()

	if mgr.Get("test") == nil {
		t.Error("topic should not be expired yet")
	}
}

func TestDirtyFlag(t *testing.T) {
	store := &memStore{states: make(map[string]*TopicState)}
	mgr, _ := NewManager(store)

	// Clean state — Save should be a no-op
	mgr.Save()
	if store.saves != 0 {
		t.Errorf("expected 0 saves on clean state, got %d", store.saves)
	}

	// Ingest sets dirty
	mgr.Ingest(&Event{Topic: "test", OK: true, Message: "ok"})
	mgr.Save()
	if store.saves != 1 {
		t.Errorf("expected 1 save after ingest, got %d", store.saves)
	}

	// Save again — should be clean
	mgr.Save()
	if store.saves != 1 {
		t.Errorf("expected still 1 save (clean), got %d", store.saves)
	}
}

func TestConcurrentIngest(t *testing.T) {
	mgr := newTestManager(t)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ok := n%2 == 0
			mgr.Ingest(&Event{Topic: "concurrent", OK: ok, Message: "msg"})
		}(i)
	}
	wg.Wait()

	ts := mgr.Get("concurrent")
	if ts == nil {
		t.Fatal("expected topic state to exist")
	}
	if ts.Status == "" {
		t.Error("expected a status after concurrent ingests")
	}
}

func TestAllTopics(t *testing.T) {
	mgr := newTestManager(t)
	mgr.Ingest(&Event{Topic: "a", OK: true, Message: "ok"})
	mgr.Ingest(&Event{Topic: "b", OK: false, Message: "fail"})

	all := mgr.All()
	if len(all) != 2 {
		t.Errorf("expected 2 topics, got %d", len(all))
	}
}

func TestBrokenTopics(t *testing.T) {
	mgr := newTestManager(t)
	mgr.Ingest(&Event{Topic: "ok-topic", OK: true, Message: "ok"})
	mgr.Ingest(&Event{Topic: "broken-topic", OK: true, Message: "ok"})
	mgr.Ingest(&Event{Topic: "broken-topic", OK: false, Message: "fail"})
	mgr.SetReminder("broken-topic", 1)

	// Backdate notification
	ts := mgr.Get("broken-topic")
	ts.LastNotifiedAt = time.Now().Unix() - 10

	broken := mgr.BrokenTopics()
	if len(broken) != 1 {
		t.Fatalf("expected 1 broken topic, got %d", len(broken))
	}
	if broken[0].Name != "broken-topic" {
		t.Errorf("expected broken-topic, got %q", broken[0].Name)
	}
}

func TestGetNonexistentTopic(t *testing.T) {
	mgr := newTestManager(t)
	if mgr.Get("nope") != nil {
		t.Error("expected nil for nonexistent topic")
	}
}

func TestIsOKNilState(t *testing.T) {
	var ts *TopicState
	if ts.IsOK() {
		t.Error("nil TopicState should not be OK")
	}
}

func TestIsOKNilEvent(t *testing.T) {
	ts := &TopicState{Name: "test"}
	if ts.IsOK() {
		t.Error("TopicState with nil LastEvent should not be OK")
	}
}
