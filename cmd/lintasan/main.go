package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
	"github.com/sanhaji182/lintasan-go/internal/mitm"
	"github.com/sanhaji182/lintasan-go/internal/server"
	"github.com/spf13/cobra"
)

// startPprofIfEnabled starts a profiling HTTP server bound to LOCALHOST ONLY.
// It is never exposed through nginx and never binds 0.0.0.0. Enabled only when
// LINTASAN_PPROF=1. Mutex and block profiling are sampled so the five standard
// profiles (cpu, heap, mutex, goroutine, block) are all available.
func startPprofIfEnabled() {
	if os.Getenv("LINTASAN_PPROF") != "1" {
		return
	}
	addr := os.Getenv("LINTASAN_PPROF_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6060" // localhost only
	}
	// Enable mutex + block profiling (off by default in Go).
	runtime.SetMutexProfileFraction(5)
	runtime.SetBlockProfileRate(10000) // 1 sample per ~10µs blocked
	go func() {
		fmt.Fprintf(os.Stderr, "🔬 pprof listening on http://%s/debug/pprof/ (localhost only)\n", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			fmt.Fprintf(os.Stderr, "pprof server error: %v\n", err)
		}
	}()
}

var version = "2.3.6"

func main() {
	rootCmd := &cobra.Command{
		Use:     "lintasan",
		Short:   "Lintasan - High-performance LLM proxy router",
		Version: version,
	}

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Lintasan proxy server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			database, err := db.Open(cfg.DBPath)
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer database.Close()

			startPprofIfEnabled()

			// MITM bridge is now owned by the server (started only when
			// LINTASAN_MITM_ENABLED is set, with a per-boot bypass secret).
			// No separate start here to avoid a double listener and the old
			// static bypass token.
			srv := server.New(cfg, database)
			fmt.Printf("🚀 Lintasan v%s listening on :%d\n", version, cfg.Port)
			return srv.Start()
		},
	}

	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Interactive setup wizard",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup()
		},
	}

	mitmCmd := &cobra.Command{
		Use:   "mitm",
		Short: "MITM bridge for IDE interception",
	}

	mitmStartCmd := &cobra.Command{
		Use:   "start",
		Short: "Start MITM bridge",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			database, err := db.Open(cfg.DBPath)
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer database.Close()

			return runMITM(cfg, database)
		},
	}

	mitmCmd.AddCommand(mitmStartCmd)
	rootCmd.AddCommand(startCmd, setupCmd, mitmCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runSetup() error {
	fmt.Println("🔧 Lintasan Setup Wizard")
	fmt.Println("========================")
	fmt.Println("TODO: Interactive setup")
	return nil
}

func runMITM(cfg *config.Config, database *db.DB) error {
	if !cfg.MITMEnabled {
		return fmt.Errorf("MITM bridge is disabled; set LINTASAN_MITM_ENABLED=true to use it")
	}
	secret, _ := database.GetSetting("mitm_secret")
	if secret == "" {
		return fmt.Errorf("no mitm_secret found; start the main server with LINTASAN_MITM_ENABLED=true first to generate one")
	}
	mitmProxy := mitm.New(cfg.MITMPort, cfg.Port, database, secret)
	return mitmProxy.Start()
}
