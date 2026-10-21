package redact

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

const secret = "s3cr3tT0k3nXYZ"

func TestURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"https://hooks.slack.com/services/T000/B000/" + secret, "https://hooks.slack.com/services/REDACTED"},
		{"http://127.0.0.1:8080/services/T/B/" + secret + "?x=1", "http://127.0.0.1:8080/services/REDACTED?REDACTED"},
		{"https://user:" + secret + "@example.com/hook", "https://REDACTED@example.com/hook"},
		{"https://" + secret + "@example.com/hook", "https://REDACTED@example.com/hook"},
		{"https://example.com/hook?token=" + secret + "&a=b", "https://example.com/hook?REDACTED"},
		{"https://example.com/hook#" + secret, "https://example.com/hook#REDACTED"},
		{"https://example.com/plain/path", "https://example.com/plain/path"},
		{"https://slack.com/api/chat.postMessage", "https://slack.com/api/chat.postMessage"},
		{"https://hooks.slack.com/services/", "https://hooks.slack.com/services/"},
		{"http://[::1/services/T/B/" + secret, "http://REDACTED"},
		{"not a url " + secret, "REDACTED"},
		{"mailto:" + secret + "@example.com", "mailto:REDACTED"},
	}
	for _, c := range cases {
		if got := URL(c.in); got != c.want {
			t.Errorf("URL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestString(t *testing.T) {
	in := `slack handler: sending request: Post "https://hooks.slack.com/services/T1/B1/` + secret +
		`": dial tcp: lookup hooks.slack.com: no such host; also https://u:` + secret + `@h.example/x and done`
	got := String(in)
	if strings.Contains(got, secret) {
		t.Fatalf("String() kept the secret: %s", got)
	}
	want := `slack handler: sending request: Post "https://hooks.slack.com/services/REDACTED": dial tcp: lookup hooks.slack.com: no such host; also https://REDACTED@h.example/x and done`
	if got != want {
		t.Errorf("String() =\n %s\nwant\n %s", got, want)
	}
	if plain := "listening on [::]:7109"; String(plain) != plain {
		t.Errorf("String() changed text without a URL: %q", String(plain))
	}
}

func TestErrKeepsURLError(t *testing.T) {
	inner := context.DeadlineExceeded
	err := &url.Error{Op: "Post", URL: "https://hooks.slack.com/services/T/B/" + secret, Err: inner}
	got := Err(err)
	if strings.Contains(got.Error(), secret) {
		t.Fatalf("Err() kept the secret: %v", got)
	}
	var ue *url.Error
	if !errors.As(got, &ue) || ue.Op != "Post" || ue.URL != "https://hooks.slack.com/services/REDACTED" {
		t.Errorf("Err() = %#v, want a *url.Error with a masked URL", got)
	}
	if !errors.Is(got, context.DeadlineExceeded) || !ue.Timeout() {
		t.Error("Err() lost the cause of the url.Error")
	}
}

func TestErrOtherErrors(t *testing.T) {
	if Err(nil) != nil {
		t.Error("Err(nil) != nil")
	}
	plain := errors.New("no url here")
	if Err(plain) != plain {
		t.Error("Err() replaced an error without a URL")
	}
	leaky := fmt.Errorf("wrapped: %w", &url.Error{Op: "Post", URL: "https://x.example/?key=" + secret, Err: errors.New("boom")})
	got := Err(leaky)
	if strings.Contains(got.Error(), secret) || !strings.Contains(got.Error(), "?REDACTED") {
		t.Errorf("Err() = %v", got)
	}
	if errors.Unwrap(got) != nil {
		t.Error("a redacted error unwraps to the error that holds the secret")
	}
}

func TestErrFromHTTPClient(t *testing.T) {
	// A request to a URL that cannot parse fails before it is sent, and
	// its error quotes the URL.
	_, err := http.NewRequest(http.MethodPost, "http://[::1/services/T/B/"+secret, nil)
	if err == nil {
		t.Fatal("expected a parse error")
	}
	if !strings.Contains(err.Error(), secret) {
		t.Fatalf("test premise: the raw error should hold the secret: %v", err)
	}
	if got := Err(err); strings.Contains(got.Error(), secret) {
		t.Errorf("Err() kept the secret: %v", got)
	}
}

func TestWriter(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	line := []byte("handler \"slack\" error: Post \"https://hooks.slack.com/services/T/B/" + secret + "\": EOF\n")
	n, err := w.Write(line)
	if err != nil || n != len(line) {
		t.Fatalf("Write() = %d, %v; want %d, nil", n, err, len(line))
	}
	if strings.Contains(buf.String(), secret) || !strings.Contains(buf.String(), "/services/REDACTED") {
		t.Errorf("writer output = %q", buf.String())
	}
}
