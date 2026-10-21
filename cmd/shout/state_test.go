//go:build unix

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// lispFixture is a state file written by the Lisp server, with two broken
// topics that have one-hour reminders.
var lispFixture = filepath.Join("..", "..", "internal", "state", "testdata", "lisp", "reminder.json")

func copyFixture(t *testing.T, src string) (string, []byte) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "shout.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing state file: %v", err)
	}
	return path, data
}

func TestStartupImportsLispState(t *testing.T) {
	dbPath, original := copyFixture(t, lispFixture)
	c := startShout(t, "--db="+dbPath)

	if out := c.out.String(); strings.Count(out, "imported 2 topics from the Lisp state file "+dbPath) != 1 {
		t.Fatalf("startup did not log the import once:\n%s", out)
	}
	backups, _ := filepath.Glob(dbPath + ".lisp-*")
	if len(backups) != 1 {
		t.Fatalf("backups = %v, want one", backups)
	}
	if kept, _ := os.ReadFile(backups[0]); !bytes.Equal(kept, original) {
		t.Error("the kept file is not the original Lisp state")
	}
	data, err := os.ReadFile(dbPath)
	if err != nil || !bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		t.Fatalf("state file was not converted to the Go format: %v\n%s", err, data)
	}

	// The topic that was broken under the Lisp server reports a fix.
	req, _ := http.NewRequest(http.MethodPost, c.base+"/events",
		strings.NewReader(`{"topic":"deploy/prod","ok":true,"message":"deploy 43 passed"}`))
	req.SetBasicAuth("ops", "ops")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /events: %v", err)
	}
	var body struct {
		Transition string `json:"transition"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	if body.Transition != "fixed" {
		t.Errorf("transition = %q, want fixed", body.Transition)
	}

	if err := c.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	c.waitExit(t, 15*time.Second)
	if code := c.cmd.ProcessState.ExitCode(); code != 0 {
		t.Fatalf("exit status %d, want 0", code)
	}

	// A restart reads the Go file and imports nothing.
	c2 := startShout(t, "--db="+dbPath)
	if strings.Contains(c2.out.String(), "imported") {
		t.Errorf("second start imported again:\n%s", c2.out.String())
	}
	if backups, _ := filepath.Glob(dbPath + ".lisp-*"); len(backups) != 1 {
		t.Errorf("backups after restart = %v, want one", backups)
	}
}

func TestStartupRefusesUnreadableState(t *testing.T) {
	for name, content := range map[string]string{
		"zero bytes": "",
		"garbage":    "not json at all\n",
		"bad list":   `[{"name": "a", "last": 7}]`,
		"bad object": `{"ci/build": `,
	} {
		t.Run(name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "shout.db")
			if err := os.WriteFile(dbPath, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			code, out := runShoutOnce(t, "--port=0", "--db="+dbPath, "--expiry=0")
			if code != 1 {
				t.Fatalf("exit status %d, want 1:\n%s", code, out)
			}
			want := "failed to initialize: initializing state: parsing state file " + dbPath + ": state file is unreadable: "
			if !strings.Contains(out, want) {
				t.Errorf("output lacks %q:\n%s", want, out)
			}
			after, _ := os.ReadFile(dbPath)
			if string(after) != content {
				t.Error("the refused state file was changed")
			}
			if others, _ := filepath.Glob(dbPath + ".*"); len(others) != 0 {
				t.Errorf("refusal left files behind: %v", others)
			}
		})
	}
}

func TestCheckState(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	lispPath, lispData := copyFixture(t, lispFixture)

	cases := []struct {
		name string
		path string
		code int
		want string
	}{
		{"missing", filepath.Join(dir, "absent.db"), 0, ": missing; the server starts with no topics"},
		{"go", write("go.db", `{"a": {"name": "a", "state": "working"}}`), 0, ": Go format, 1 topic"},
		{"lisp", lispPath, 0, ": Lisp format, 2 topics to import, 0 skipped; the server converts it at startup"},
		{"lisp empty", write("null.db", "null\n"), 0, ": Lisp format, 0 topics to import"},
		{"zero bytes", write("zero.db", ""), 3, "state file is unreadable: the file is empty"},
		{"garbage", write("junk.db", "junk"), 3, "state file is unreadable: expected a JSON object"},
		{"bad lisp", write("badlisp.db", "[1]"), 3, "state file is unreadable: not a valid Lisp state list"},
		{"directory", dir, 1, "loading state:"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, out := runShoutOnce(t, "--check-state", "--db="+c.path)
			if code != c.code {
				t.Fatalf("exit status %d, want %d:\n%s", code, c.code, out)
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("output lacks %q:\n%s", c.want, out)
			}
			if strings.Count(strings.TrimSpace(out), "\n") != 0 {
				t.Errorf("want a single line of output:\n%s", out)
			}
		})
	}

	if after, _ := os.ReadFile(lispPath); !bytes.Equal(after, lispData) {
		t.Error("-check-state changed the Lisp state file")
	}
	if backups, _ := filepath.Glob(lispPath + ".lisp-*"); len(backups) != 0 {
		t.Errorf("-check-state wrote %v", backups)
	}
}
