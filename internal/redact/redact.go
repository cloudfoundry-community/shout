// Package redact masks the secret parts of URLs before they reach a log
// line or an API response.
//
// Notification targets carry their credentials inside the URL. A Slack
// incoming webhook keeps its secret in the path after /services/, other
// webhooks put tokens in the query string, and any URL can carry a user
// name and password. Errors from net/http quote the full request URL, so a
// failed delivery would otherwise write those secrets to the log.
package redact

import (
	"io"
	"net/url"
	"regexp"
	"strings"
)

// Mask replaces every secret that this package removes.
const Mask = "REDACTED"

// slackServices is the path prefix of a Slack incoming webhook. Everything
// after it is the secret. The rule applies on any host, so a proxy, a mock
// receiver, or a Slack government host gets the same protection.
const slackServices = "/services/"

// urlPattern finds URLs inside free text. A URL ends at white space, at a
// quote, or at an angle bracket, which covers the way net/http quotes a URL
// in its errors (Post "https://...": ...).
var urlPattern = regexp.MustCompile("(?i)\\b[a-z][a-z0-9+.-]*://[^\\s\"'`<>]+")

// URL returns raw with its secret parts replaced by Mask. It masks the user
// information, the query, the fragment, and, for a Slack webhook, the path
// after /services/. A string that does not parse as an absolute URL keeps
// only its scheme.
func URL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || (u.Host == "" && u.Opaque == "" && u.User == nil) {
		return maskUnparsed(raw)
	}
	if u.User != nil {
		u.User = url.User(Mask)
	}
	if u.Opaque != "" {
		u.Opaque = Mask
	}
	if u.RawQuery != "" || u.ForceQuery {
		u.RawQuery = Mask
		u.ForceQuery = false
	}
	if u.Fragment != "" || u.RawFragment != "" {
		u.Fragment = Mask
		u.RawFragment = ""
	}
	if i := strings.Index(u.Path, slackServices); i >= 0 && len(u.Path) > i+len(slackServices) {
		u.Path = u.Path[:i+len(slackServices)] + Mask
		u.RawPath = ""
	}
	return u.String()
}

// maskUnparsed keeps the scheme of a string that url.Parse rejected and
// masks the rest, since nothing tells us where its secret ends.
func maskUnparsed(raw string) string {
	if i := strings.Index(raw, "://"); i > 0 {
		return raw[:i+len("://")] + Mask
	}
	return Mask
}

// String returns s with every URL in it passed through URL.
func String(s string) string {
	if !strings.Contains(s, "://") {
		return s
	}
	return urlPattern.ReplaceAllStringFunc(s, URL)
}

// Err returns an error whose message carries no URL secrets. A *url.Error,
// which is what net/http returns, comes back as a *url.Error with its URL
// masked, so callers can still inspect it with errors.As. Any other error
// whose message contains a URL comes back as a plain error with the masked
// message. An error without a URL in its message comes back unchanged.
func Err(err error) error {
	if err == nil {
		return nil
	}
	//nolint:errorlint // only the outermost error's URL is rewritten here.
	if ue, ok := err.(*url.Error); ok {
		return &url.Error{Op: ue.Op, URL: URL(ue.URL), Err: Err(ue.Err)}
	}
	msg := err.Error()
	clean := String(msg)
	if clean == msg {
		return err
	}
	return redactedError(clean)
}

// redactedError replaces an error whose message held a URL. It does not
// unwrap, because the wrapped error would print the secret again.
type redactedError string

func (e redactedError) Error() string { return string(e) }

// NewWriter returns a writer that masks URL secrets in everything written
// through it. The log package writes each message with a single call, so a
// URL is never split across two writes.
func NewWriter(w io.Writer) io.Writer {
	return writer{w: w}
}

type writer struct {
	w io.Writer
}

func (rw writer) Write(p []byte) (int, error) {
	if _, err := io.WriteString(rw.w, String(string(p))); err != nil {
		return 0, err
	}
	return len(p), nil
}
