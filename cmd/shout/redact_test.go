//go:build unix

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runShoutOnce runs the binary to completion and returns its exit code and
// combined output.
func runShoutOnce(t *testing.T, args ...string) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], args...)
	cmd.Env = append(os.Environ(), runMainEnv+"=1")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("shout %v did not exit:\n%s", args, out.String())
	}
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("running shout: %v", err)
	}
	return cmd.ProcessState.ExitCode(), out.String()
}

func TestStartupErrorLogHidesWebhookSecret(t *testing.T) {
	const secret = "MAINS3CR3TH00K"
	rules := filepath.Join(t.TempDir(), "rules.yml")
	text := fmt.Sprintf(`rules:
  - for: "*"
    when:
      - match: 'topic == "https://hooks.slack.com/services/T1/B1/%s" &&'
        do: []
`, secret)
	if err := os.WriteFile(rules, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out := runShoutOnce(t, "--port=0", "--db="+filepath.Join(t.TempDir(), "shout.db"), "--rules="+rules)
	if code != 1 || !strings.Contains(out, "failed to initialize") {
		t.Fatalf("exit status %d, want 1 with an init error:\n%s", code, out)
	}
	if strings.Contains(out, secret) {
		t.Fatalf("startup log reveals the secret:\n%s", out)
	}
	if !strings.Contains(out, "/services/REDACTED") {
		t.Errorf("startup log does not show the masked URL:\n%s", out)
	}
}
