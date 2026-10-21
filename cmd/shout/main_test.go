//go:build unix

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// runMainEnv makes the test binary run main() instead of the tests, so the
// tests can start the real server as a child process and signal it.
const runMainEnv = "SHOUT_TEST_RUN_MAIN"

func TestMain(m *testing.M) {
	if os.Getenv(runMainEnv) == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// syncBuffer collects the child's output while it is still running.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type child struct {
	cmd    *exec.Cmd
	base   string
	dbPath string
	out    *syncBuffer
	exited chan struct{}
	err    error
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// startShout starts the server as a child process and waits until it
// answers GET /info.
func startShout(t *testing.T, extraArgs ...string) *child {
	t.Helper()
	dir := t.TempDir()
	port := freePort(t)
	c := &child{
		base:   fmt.Sprintf("http://127.0.0.1:%d", port),
		dbPath: filepath.Join(dir, "shout.db"),
		out:    &syncBuffer{},
		exited: make(chan struct{}),
	}
	args := append([]string{
		fmt.Sprintf("--port=%d", port),
		"--db=" + c.dbPath,
		"--ops=ops:ops",
		"--admin=admin:admin",
		"--expiry=0",
	}, extraArgs...)
	c.cmd = exec.Command(os.Args[0], args...)
	c.cmd.Env = append(os.Environ(), runMainEnv+"=1")
	c.cmd.Stdout = c.out
	c.cmd.Stderr = c.out
	if err := c.cmd.Start(); err != nil {
		t.Fatalf("starting shout: %v", err)
	}
	go func() {
		c.err = c.cmd.Wait()
		close(c.exited)
	}()
	t.Cleanup(func() {
		select {
		case <-c.exited:
		default:
			_ = c.cmd.Process.Kill()
			<-c.exited
		}
		if t.Failed() {
			t.Logf("shout output:\n%s", c.out.String())
		}
	})

	deadline := time.Now().Add(15 * time.Second)
	for {
		resp, err := http.Get(c.base + "/info")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return c
			}
		}
		select {
		case <-c.exited:
			t.Fatalf("shout exited before it was ready: %v", c.err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("shout did not become ready")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (c *child) postEvent(body string) (int, error) {
	req, err := http.NewRequest(http.MethodPost, c.base+"/events", strings.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("ops", "ops")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

func (c *child) waitExit(t *testing.T, within time.Duration) {
	t.Helper()
	select {
	case <-c.exited:
	case <-time.After(within):
		t.Fatalf("shout did not exit within %s", within)
	}
}

func TestShutdownSignalSavesStateAndExitsZero(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGINT} {
		t.Run(sig.String(), func(t *testing.T) {
			c := startShout(t)

			for _, body := range []string{
				`{"topic":"ci/build","ok":true,"message":"passed"}`,
				`{"topic":"ci/build","ok":false,"message":"failed"}`,
			} {
				code, err := c.postEvent(body)
				if err != nil || code != http.StatusOK {
					t.Fatalf("POST /events: code %d, err %v", code, err)
				}
			}

			// The periodic save runs once a minute, so nothing is on disk
			// yet. Only the shutdown save can put the state there.
			if _, err := os.Stat(c.dbPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("state file exists before the signal (err=%v)", err)
			}

			if err := c.cmd.Process.Signal(sig); err != nil {
				t.Fatalf("sending %v: %v", sig, err)
			}
			c.waitExit(t, 15*time.Second)
			if c.err != nil {
				t.Fatalf("shout exited with %v after %v, want exit status 0", c.err, sig)
			}
			if code := c.cmd.ProcessState.ExitCode(); code != 0 {
				t.Fatalf("exit status %d, want 0", code)
			}

			data, err := os.ReadFile(c.dbPath)
			if err != nil {
				t.Fatalf("reading state file after %v: %v", sig, err)
			}
			var states map[string]struct {
				Status string `json:"state"`
				Last   struct {
					Message string `json:"message"`
				} `json:"last"`
			}
			if err := json.Unmarshal(data, &states); err != nil {
				t.Fatalf("parsing state file: %v\n%s", err, data)
			}
			ts, ok := states["ci/build"]
			if !ok {
				t.Fatalf("topic ci/build missing from state file:\n%s", data)
			}
			if ts.Status != "broken" || ts.Last.Message != "failed" {
				t.Errorf("saved state = %+v, want broken with last message %q", ts, "failed")
			}
			if _, err := os.Stat(c.dbPath + ".tmp"); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("temp state file left behind: %v", err)
			}
			if !strings.Contains(c.out.String(), "graceful shutdown complete") {
				t.Errorf("output lacks the shutdown message:\n%s", c.out.String())
			}
		})
	}
}

func TestSecondSignalExitsImmediately(t *testing.T) {
	// A webhook receiver that never answers keeps one request in flight,
	// so the first signal leaves the server draining.
	received := make(chan struct{}, 1)
	unblock := make(chan struct{})
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case received <- struct{}{}:
		default:
		}
		<-unblock
	}))
	defer hook.Close()
	defer close(unblock)

	rules := filepath.Join(t.TempDir(), "rules.yml")
	ruleText := fmt.Sprintf(`rules:
  - for: "*"
    when:
      - match: "*"
        do:
          - webhook:
              url: %q
`, hook.URL)
	if err := os.WriteFile(rules, []byte(ruleText), 0o600); err != nil {
		t.Fatalf("writing rules: %v", err)
	}

	c := startShout(t, "--rules="+rules)
	go func() { _, _ = c.postEvent(`{"topic":"ci/hang","ok":false,"message":"failed"}`) }()
	select {
	case <-received:
	case <-time.After(10 * time.Second):
		t.Fatal("webhook was never called")
	}

	if err := c.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("sending SIGTERM: %v", err)
	}
	select {
	case <-c.exited:
		t.Fatalf("shout exited (%v) while a request was still in flight", c.err)
	case <-time.After(500 * time.Millisecond):
	}

	if err := c.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("sending second SIGTERM: %v", err)
	}
	c.waitExit(t, 5*time.Second)
	ws, ok := c.cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGTERM {
		t.Fatalf("after a second SIGTERM shout exited with %v, want death by SIGTERM", c.err)
	}
}
