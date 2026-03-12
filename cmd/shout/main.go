package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cloudfoundry-community/shout/internal/api"
	"github.com/cloudfoundry-community/shout/internal/notify"
	"github.com/cloudfoundry-community/shout/pkg/version"
)

func main() {
	var cfg api.Config
	var showVersion bool

	flag.IntVar(&cfg.Port, "port", envInt("SHOUT_PORT", 7109), "HTTP listen port")
	flag.StringVar(&cfg.DBPath, "db", envStr("SHOUT_DATABASE", "/var/db/shout.db"), "state database path")
	flag.StringVar(&cfg.RulesFile, "rules", envStr("SHOUT_RULES", ""), "path to YAML rules file")
	flag.StringVar(&cfg.OpsCreds, "ops", envStr("SHOUT_OPS_AUTH", "shout:shout"), "ops credentials (user:pass)")
	flag.StringVar(&cfg.AdminCreds, "admin", envStr("SHOUT_ADMIN_AUTH", "shout:shout"), "admin credentials (user:pass)")
	flag.Int64Var(&cfg.Expiry, "expiry", int64(envInt("SHOUT_EXPIRY", 86400)), "state expiry in seconds (0=no expiry)")
	flag.StringVar(&cfg.TLSCert, "tls-cert", envStr("SHOUT_TLS_CERT", ""), "path to TLS certificate file")
	flag.StringVar(&cfg.TLSKey, "tls-key", envStr("SHOUT_TLS_KEY", ""), "path to TLS private key file")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.Parse()

	if showVersion {
		fmt.Printf("shout v%s (%s)\n", version.Version(), version.Release)
		if version.BuildDate != "" {
			fmt.Printf("  built:  %s\n", version.BuildDate)
			fmt.Printf("  commit: %s (%s)\n", version.BuildVcsId, version.BuildVcsIdDate)
		}
		os.Exit(0)
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

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := srv.Run(ctx); err != nil {
		log.Fatalf("server error: %v", err)
	}
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
