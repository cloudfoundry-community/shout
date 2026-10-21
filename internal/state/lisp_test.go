package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The files in testdata/lisp were written by the Lisp server itself
// (lisp/api.lisp, built from this repository) running in a container.
//
//   - mixed.json holds topics that were broken from their first event,
//     went broken, were fixed, and kept working, one event with numeric and
//     boolean metadata, and the effect of an announcement, which adds no
//     topic.
//   - reloaded.json is mixed.json after the Lisp server loaded it, took one
//     more event, and saved it again. Reloading loses the event timestamps.
//   - reminder.json holds broken topics whose rules set a one-hour reminder.
//   - announcements-only.json and empty.json come from servers that saw only
//     announcements or nothing at all, and hold the bare word null.

const fixtureNow = 1800000000

type logRecorder struct {
	lines []string
}

func (r *logRecorder) logf(format string, args ...any) {
	r.lines = append(r.lines, fmt.Sprintf(format, args...))
}

// fixtureStore copies a fixture into a temporary directory and returns a
// store for it with a fixed clock and a recorded log.
func fixtureStore(t *testing.T, fixture string) (*JSONFileStore, *logRecorder, []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "lisp", fixture))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return storeWith(t, data)
}

func storeWith(t *testing.T, data []byte) (*JSONFileStore, *logRecorder, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shout.db")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("writing state file: %v", err)
	}
	rec := &logRecorder{}
	s := NewJSONFileStore(path)
	s.now = func() time.Time { return time.Unix(fixtureNow, 0) }
	s.logf = rec.logf
	return s, rec, data
}

func backupPath(s *JSONFileStore) string {
	return fmt.Sprintf("%s.lisp-%d", s.Path, fixtureNow)
}

// assertImported checks the files an import leaves behind: the original
// bytes in the backup, Go-format state at the path, and no temp file.
func assertImported(t *testing.T, s *JSONFileStore, original []byte, want map[string]*TopicState) {
	t.Helper()
	kept, err := os.ReadFile(backupPath(s))
	if err != nil {
		t.Fatalf("original Lisp file was not kept: %v", err)
	}
	if !bytes.Equal(kept, original) {
		t.Errorf("kept file differs from the original:\n got %s\nwant %s", kept, original)
	}
	if info, err := os.Stat(backupPath(s)); err == nil && info.Mode().Perm() != 0600 {
		t.Errorf("kept file mode = %v, want 0600", info.Mode().Perm())
	}
	if _, err := os.Stat(s.Path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("temp state file left behind: %v", err)
	}

	converted, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatalf("reading converted state: %v", err)
	}
	var onDisk map[string]*TopicState
	if err := json.Unmarshal(converted, &onDisk); err != nil {
		t.Fatalf("converted state is not a Go-format object: %v\n%s", err, converted)
	}
	if onDisk == nil {
		onDisk = map[string]*TopicState{}
	}
	if !reflect.DeepEqual(onDisk, want) {
		t.Errorf("converted state on disk differs from the loaded state")
	}

	// A second load reads the Go format and changes nothing.
	insp, err := s.Inspect()
	if err != nil || insp.Format != FormatGo || insp.Topics != len(want) {
		t.Errorf("after import Inspect() = %+v, %v; want Go format with %d topics", insp, err, len(want))
	}
}

func assertOneLogLine(t *testing.T, rec *logRecorder, contains ...string) {
	t.Helper()
	if len(rec.lines) != 1 {
		t.Fatalf("import logged %d lines, want 1: %q", len(rec.lines), rec.lines)
	}
	for _, c := range contains {
		if !strings.Contains(rec.lines[0], c) {
			t.Errorf("log line %q lacks %q", rec.lines[0], c)
		}
	}
}

func TestLoadLispMixed(t *testing.T) {
	s, rec, original := fixtureStore(t, "mixed.json")
	s.ImportExpiry = 86400

	states, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	names := []string{"pipeline/broken-first", "pipeline/went-broken", "pipeline/fixed", "pipeline/working", "pipeline/numeric-meta"}
	if len(states) != len(names) {
		t.Fatalf("imported %d topics, want %d: %v", len(states), len(names), states)
	}
	for _, n := range names {
		ts := states[n]
		if ts == nil {
			t.Fatalf("topic %q missing", n)
		}
		if ts.Name != n || ts.Expiry != 86400 {
			t.Errorf("%s: name %q expiry %d, want the topic name and 86400", n, ts.Name, ts.Expiry)
		}
		if ts.LastEvent == nil || ts.LastEvent.Topic != n || ts.LastEvent.ReportedAt == 0 || ts.LastEvent.OccurredAt == 0 {
			t.Errorf("%s: last event %+v lacks its topic or timestamps", n, ts.LastEvent)
		}
	}
	if _, ok := states["release/announce"]; ok {
		t.Error("an announcement became a topic")
	}

	bf := states["pipeline/broken-first"]
	if bf.Status != "broken" || bf.IsOK() || bf.LastNotifiedAt != 1792543902 || bf.PreviousEvent != nil {
		t.Errorf("broken-first = %+v", bf)
	}
	if bf.LastEvent.Message != "build 7 failed" || bf.LastEvent.Link != "https://ci.example.com/builds/7" ||
		!reflect.DeepEqual(bf.LastEvent.Metadata, map[string]string{"buildId": "7"}) {
		t.Errorf("broken-first last event = %+v", bf.LastEvent)
	}

	wb := states["pipeline/went-broken"]
	if wb.Status != "broken" || wb.IsOK() || wb.PreviousEvent == nil || !wb.PreviousEvent.OK || wb.PreviousEvent.Message != "build 1 passed" {
		t.Errorf("went-broken = %+v, previous %+v", wb, wb.PreviousEvent)
	}

	fx := states["pipeline/fixed"]
	if fx.Status != "fixed" || !fx.IsOK() || fx.PreviousEvent == nil || fx.PreviousEvent.OK {
		t.Errorf("fixed = %+v, previous %+v", fx, fx.PreviousEvent)
	}

	wk := states["pipeline/working"]
	if wk.Status != "working" || !wk.IsOK() || wk.LastEvent.Message != "build 6 passed" || wk.FirstEvent.Message != "build 5 passed" {
		t.Errorf("working = %+v", wk)
	}

	nm := states["pipeline/numeric-meta"]
	wantMeta := map[string]string{"attempt": "3", "branch": "main", "retried": "true"}
	if !reflect.DeepEqual(nm.LastEvent.Metadata, wantMeta) {
		t.Errorf("numeric-meta metadata = %v, want %v", nm.LastEvent.Metadata, wantMeta)
	}

	assertImported(t, s, original, states)
	assertOneLogLine(t, rec, "imported 5 topics", s.Path, backupPath(s))
}

func TestLoadLispReloadedFillsLostTimestamps(t *testing.T) {
	s, rec, original := fixtureStore(t, "reloaded.json")

	states, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(states) != 6 {
		t.Fatalf("imported %d topics, want 6", len(states))
	}

	// The Lisp server lost these timestamps when it reloaded the file. The
	// import time stands in for the last report, so the default expiry does
	// not drop the topic on the first scan.
	wb := states["pipeline/went-broken"]
	if wb.LastEvent.ReportedAt != fixtureNow || wb.LastEvent.OccurredAt != 0 {
		t.Errorf("went-broken last event times = %d/%d, want reported %d", wb.LastEvent.OccurredAt, wb.LastEvent.ReportedAt, int64(fixtureNow))
	}
	if wb.Expiry != 0 {
		t.Errorf("expiry = %d without ImportExpiry, want 0", wb.Expiry)
	}

	// The topic added after the reload kept its own timestamps.
	ar := states["pipeline/after-reload"]
	if ar.Status != "broken" || ar.LastEvent.ReportedAt != 1792543906 {
		t.Errorf("after-reload = %+v, last %+v", ar, ar.LastEvent)
	}

	assertImported(t, s, original, states)
	assertOneLogLine(t, rec, "imported 6 topics")
}

func TestLoadLispEmptyStates(t *testing.T) {
	for _, fixture := range []string{"empty.json", "announcements-only.json"} {
		t.Run(fixture, func(t *testing.T) {
			s, rec, original := fixtureStore(t, fixture)

			states, err := s.Load()
			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}
			if states == nil || len(states) != 0 {
				t.Fatalf("Load() = %v, want an empty map", states)
			}
			assertImported(t, s, original, states)
			assertOneLogLine(t, rec, "imported 0 topics")
		})
	}
}

func TestLispImportKeepsBrokenTopicsWorking(t *testing.T) {
	s, _, _ := fixtureStore(t, "reminder.json")
	s.ImportExpiry = 86400
	m, err := NewManager(s, WithExpiry(86400))
	if err != nil {
		t.Fatalf("NewManager() error: %v", err)
	}

	prod := m.Get("deploy/prod")
	if prod == nil || prod.Status != "broken" || prod.RemindEvery != 3600 || prod.LastNotifiedAt != 1792544112 {
		t.Fatalf("deploy/prod = %+v, want broken with a 3600s reminder", prod)
	}

	// Once the imported reminder interval has passed since the last
	// notification, the scan loop keeps reminding about the broken topics.
	for _, name := range []string{"deploy/prod", "deploy/staging"} {
		ts := m.Get(name)
		if ts.RemindEvery != 3600 {
			t.Fatalf("%s reminder = %d, want 3600", name, ts.RemindEvery)
		}
		ts.LastNotifiedAt = time.Now().Unix() - 2*ts.RemindEvery
	}
	due := map[string]bool{}
	for _, ts := range m.BrokenTopics() {
		due[ts.Name] = true
	}
	if !due["deploy/prod"] || !due["deploy/staging"] {
		t.Errorf("BrokenTopics() = %v, want both imported broken topics", due)
	}

	// A topic that was broken at the upgrade reports "fixed" on its next
	// passing event, which is the notification the old move-aside lost.
	if got := m.Ingest(&Event{Topic: "deploy/prod", OK: true, Message: "deploy 43 passed"}); got != "fixed" {
		t.Errorf("Ingest(ok) on an imported broken topic = %q, want fixed", got)
	}
	if got := m.Ingest(&Event{Topic: "deploy/staging", OK: false, Message: "still failing"}); got != "" {
		t.Errorf("Ingest(failed) on an imported broken topic = %q, want no transition", got)
	}
}

func TestLispImportSurvivesFirstExpiry(t *testing.T) {
	// reloaded.json lost its event timestamps. Without a stand-in report
	// time, the first scan would expire every topic in it.
	s, _, _ := fixtureStore(t, "reloaded.json")
	s.now = time.Now
	s.ImportExpiry = 86400
	m, err := NewManager(s, WithExpiry(86400))
	if err != nil {
		t.Fatalf("NewManager() error: %v", err)
	}
	m.Expire()
	if m.Get("pipeline/went-broken") == nil {
		t.Error("an imported topic expired on the first scan")
	}
}

func TestLoadUnreadableStateRefuses(t *testing.T) {
	cases := map[string]string{
		"zero bytes":        "",
		"white space":       " \n\t\n",
		"not JSON":          "hello world\n",
		"bad Go object":     `{"ci/build": {"state": "broken"`,
		"wrong Go types":    `{"ci/build": {"name": 7}}`,
		"list of numbers":   `[1, 2, 3]`,
		"list then garbage": `[{"name": "a"}] trailing`,
		"bad Lisp event":    `[{"name": "a", "last": "not an event"}]`,
		"truncated list":    `[{"name": "a", "state": "broken"`,
		"bare string":       `"shout"`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			s, rec, original := storeWith(t, []byte(content))

			for _, op := range []string{"Inspect", "Load"} {
				var err error
				if op == "Inspect" {
					_, err = s.Inspect()
				} else {
					_, err = s.Load()
				}
				if !errors.Is(err, ErrUnreadable) {
					t.Fatalf("%s() error = %v, want ErrUnreadable", op, err)
				}
				if !strings.Contains(err.Error(), s.Path) {
					t.Errorf("%s() error %q does not name the file", op, err)
				}
			}

			after, err := os.ReadFile(s.Path)
			if err != nil || !bytes.Equal(after, original) {
				t.Errorf("state file changed: %q, %v", after, err)
			}
			matches, _ := filepath.Glob(s.Path + ".*")
			if len(matches) != 0 {
				t.Errorf("refusal left files behind: %v", matches)
			}
			if len(rec.lines) != 0 {
				t.Errorf("refusal logged %q", rec.lines)
			}
		})
	}
}

func TestInspectDoesNotConvert(t *testing.T) {
	s, rec, original := fixtureStore(t, "mixed.json")
	insp, err := s.Inspect()
	if err != nil {
		t.Fatalf("Inspect() error: %v", err)
	}
	if insp != (Inspection{Format: FormatLisp, Topics: 5}) {
		t.Errorf("Inspect() = %+v", insp)
	}
	after, _ := os.ReadFile(s.Path)
	if !bytes.Equal(after, original) {
		t.Error("Inspect() changed the state file")
	}
	if _, err := os.Stat(backupPath(s)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Inspect() wrote a backup: %v", err)
	}
	if len(rec.lines) != 0 {
		t.Errorf("Inspect() logged %q", rec.lines)
	}

	missing := NewJSONFileStore(filepath.Join(t.TempDir(), "absent.db"))
	if insp, err := missing.Inspect(); err != nil || insp.Format != FormatMissing {
		t.Errorf("Inspect() on a missing file = %+v, %v", insp, err)
	}
}

func TestLoadLispSkipsNamelessAndKeepsLastDuplicate(t *testing.T) {
	data := []byte(`[
		{"name": null, "state": "broken", "last": {"ok": null, "message": "no name"}},
		{"name": "dup", "state": "broken", "notified": 10, "last": {"ok": null, "message": "first copy", "reported_at": 5}},
		null,
		{"name": 42, "state": "working"},
		{"name": "dup", "notified": 20, "last": {"ok": "yes", "message": "second copy", "occurred_at": 7}},
		{"name": "flag", "last": {"ok": false, "message": 3.5, "link": null, "metadata": ["not", "an", "object"]}}
	]`)
	s, rec, _ := storeWith(t, data)

	states, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("imported %v, want dup and flag", states)
	}
	dup := states["dup"]
	if dup.LastNotifiedAt != 20 || dup.LastEvent.Message != "second copy" || !dup.LastEvent.OK || dup.Status != "working" {
		t.Errorf("dup = %+v, last %+v; want the later entry, ok, and a derived working status", dup, dup.LastEvent)
	}
	if dup.LastEvent.ReportedAt != 7 {
		t.Errorf("dup reported_at = %d, want the occurred_at 7 to stand in", dup.LastEvent.ReportedAt)
	}
	flag := states["flag"]
	if flag.IsOK() || flag.Status != "broken" || flag.LastEvent.Message != "3.5" || flag.LastEvent.Metadata != nil {
		t.Errorf("flag = %+v, last %+v", flag, flag.LastEvent)
	}
	assertOneLogLine(t, rec, "imported 2 topics", "skipped 3 without a topic name")
}

func TestLoadLispReusesIdenticalBackup(t *testing.T) {
	s, _, original := fixtureStore(t, "empty.json")
	if err := os.WriteFile(backupPath(s), original, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err != nil {
		t.Fatalf("Load() with an identical backup in place: %v", err)
	}
}

func TestLoadLispRefusesToOverwriteDifferentBackup(t *testing.T) {
	s, rec, original := fixtureStore(t, "mixed.json")
	if err := os.WriteFile(backupPath(s), []byte("something else"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Load() error = %v, want a refusal to overwrite the backup", err)
	}
	after, _ := os.ReadFile(s.Path)
	if !bytes.Equal(after, original) {
		t.Error("a failed import changed the state file")
	}
	if len(rec.lines) != 0 {
		t.Errorf("a failed import logged %q", rec.lines)
	}
}

func TestLoadGoFormatIsNotImported(t *testing.T) {
	s, rec, _ := storeWith(t, []byte(`{"ci/build": {"name": "ci/build", "state": "broken", "notified": 1}}`))
	states, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if states["ci/build"] == nil || states["ci/build"].Status != "broken" {
		t.Errorf("Load() = %v", states)
	}
	matches, _ := filepath.Glob(s.Path + ".*")
	if len(matches) != 0 || len(rec.lines) != 0 {
		t.Errorf("a Go-format load left %v and logged %q", matches, rec.lines)
	}
}
