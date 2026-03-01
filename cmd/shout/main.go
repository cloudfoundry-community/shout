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
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.Parse()

	if showVersion {
		fmt.Printf("shout v%s (%s)\n", version.Version, version.Release)
		os.Exit(0)
	}

	// Register notification handlers.
	handlers := notify.NewRegistry()
	handlers.Register(notify.NewSlackHandler())
	handlers.Register(notify.NewWebhookHandler())

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
