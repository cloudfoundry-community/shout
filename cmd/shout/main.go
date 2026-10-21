package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cloudfoundry-community/shout/internal/api"
	"github.com/cloudfoundry-community/shout/internal/notify"
	"github.com/cloudfoundry-community/shout/internal/redact"
	"github.com/cloudfoundry-community/shout/internal/state"
	"github.com/cloudfoundry-community/shout/pkg/version"
)

// Exit codes of -check-state.
const (
	exitStateOK         = 0 // missing, Go format, or a Lisp file that converts
	exitStateError      = 1 // the file could not be read at all
	exitStateUnreadable = 3 // the file is in neither format
)

func main() {
	// Every log line passes through the redactor, so a URL secret that
	// slips into an error message still never reaches the log file.
	log.SetOutput(redact.NewWriter(os.Stderr))

	var cfg api.Config
	var showVersion, checkState bool

	flag.IntVar(&cfg.Port, "port", envInt("SHOUT_PORT", 7109), "HTTP listen port")
	flag.StringVar(&cfg.DBPath, "db", envStr("SHOUT_DATABASE", "/var/db/shout.db"), "state database path")
	flag.StringVar(&cfg.RulesFile, "rules", envStr("SHOUT_RULES", ""), "path to YAML rules file")
	flag.StringVar(&cfg.OpsCreds, "ops", envStr("SHOUT_OPS_AUTH", "shout:shout"), "ops credentials (user:pass)")
	flag.StringVar(&cfg.AdminCreds, "admin", envStr("SHOUT_ADMIN_AUTH", "shout:shout"), "admin credentials (user:pass)")
	flag.Int64Var(&cfg.Expiry, "expiry", int64(envInt("SHOUT_EXPIRY", 86400)), "state expiry in seconds (0=no expiry)")
	flag.StringVar(&cfg.TLSCert, "tls-cert", envStr("SHOUT_TLS_CERT", ""), "path to TLS certificate file")
	flag.StringVar(&cfg.TLSKey, "tls-key", envStr("SHOUT_TLS_KEY", ""), "path to TLS private key file")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.BoolVar(&checkState, "check-state", false, "report whether the state file can be loaded, without changing it, and exit")
	flag.Parse()

	if showVersion {
		fmt.Printf("shout v%s (%s)\n", version.Version(), version.Release)
		if version.BuildDate != "" {
			fmt.Printf("  built:  %s\n", version.BuildDate)
			fmt.Printf("  commit: %s (%s)\n", version.BuildVcsId, version.BuildVcsIdDate)
		}
		os.Exit(0)
	}

	if checkState {
		os.Exit(runCheckState(cfg.DBPath, os.Stdout, os.Stderr))
	}

	// Register notification handlers.
	handlers := notify.NewRegistry()
	handlers.Register(notify.NewSlackHandler())
	handlers.Register(notify.NewSlackAppHandler())
	handlers.Register(notify.NewWebhookHandler())
	handlers.Register(notify.NewEmailHandler())

	srv, err := api.New(cfg, handlers)
	if err != nil {
		log.Fatalf("failed to initialize: %v", err)
	}

	// The first SIGINT or SIGTERM starts a graceful shutdown that drains
	// requests and saves state. Stopping the notification as soon as it
	// fires restores the default action, so a second signal exits at once.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		stop()
	}()

	if err := srv.Run(ctx); err != nil {
		log.Printf("server error: %v", err)
		os.Exit(1)
	}
}

// runCheckState reports on the state file at path without writing to it,
// and returns the exit code. A missing file, a Go-format file, and a Lisp
// file that the server will convert at startup all exit 0. A file in
// neither format exits 3, and a file that cannot be read exits 1.
func runCheckState(path string, stdout, stderr io.Writer) int {
	insp, err := state.NewJSONFileStore(path).Inspect()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "shout: %s\n", redact.String(err.Error()))
		if errors.Is(err, state.ErrUnreadable) {
			return exitStateUnreadable
		}
		return exitStateError
	}
	switch insp.Format {
	case state.FormatMissing:
		_, _ = fmt.Fprintf(stdout, "state file %s: missing; the server starts with no topics\n", path)
	case state.FormatGo:
		_, _ = fmt.Fprintf(stdout, "state file %s: Go format, %s\n", path, topics(insp.Topics))
	case state.FormatLisp:
		_, _ = fmt.Fprintf(stdout, "state file %s: Lisp format, %s to import, %d skipped; the server converts it at startup\n",
			path, topics(insp.Topics), insp.Skipped)
	}
	return exitStateOK
}

func topics(n int) string {
	if n == 1 {
		return "1 topic"
	}
	return fmt.Sprintf("%d topics", n)
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return fallback
	}
	return n
}
