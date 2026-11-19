// Command vista is Spark's read-only dashboard. One binary runs the server or
// one small operator command, picked by the first argument.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lsariol/blackboxlog"
	"github.com/lsariol/vista/internal/access"
	"github.com/lsariol/vista/internal/config"
	"github.com/lsariol/vista/internal/source"
	"github.com/lsariol/vista/internal/web"
)

// command is one entry of the command table; help comes from it.
type command struct {
	name, summary, usage, example string
	run                           func(args []string) int
}

var commands []command

func init() {
	commands = []command{
		{"serve", "Run the dashboard (what the compose file starts)", "vista serve", "vista serve", runServe},
		{"health", "Exit 0 if the dashboard answers its health check", "vista health", "docker exec vista /vista health", runHealth},
		{"version", "Print the version", "vista version", "vista version", runVersion},
		{"help", "Show commands, or one command's usage", "vista help [command]", "vista help serve", runHelp},
	}
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		os.Exit(runHelp(nil))
	}
	for _, c := range commands {
		if c.name == args[0] {
			os.Exit(c.run(args[1:]))
		}
	}
	fmt.Fprintf(os.Stderr, "! Unknown command %q. Run \"vista help\" to list the commands.\n", args[0])
	os.Exit(1)
}

func runHelp(args []string) int {
	if len(args) > 0 {
		for _, c := range commands {
			if c.name == args[0] {
				fmt.Printf("%s\n\nUsage:\n  %s\n\nExample:\n  %s\n", c.summary, c.usage, c.example)
				return 0
			}
		}
		fmt.Fprintf(os.Stderr, "! Unknown command %q.\nUsage: vista help [command]\n", args[0])
		return 1
	}
	fmt.Print("Usage: vista <command>\n\nCommands:\n")
	for _, c := range commands {
		fmt.Printf("  %-9s %s\n", c.name, c.summary)
	}
	return 0
}

func runVersion(args []string) int {
	fmt.Println(version())
	return 0
}

// runServe runs the dashboard until SIGINT or SIGTERM (docker stop).
func runServe(args []string) int {
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "! serve takes no arguments.\nUsage: vista serve")
		return 1
	}
	log := blackboxlog.New(blackboxlog.Options{})
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Error("startup failed", "component", "config", "err", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sources := source.NewSet(cfg, log)
	var handler http.Handler = web.New(sources, log, cfg.Version)
	if cfg.AccessEnabled() {
		handler = access.NewVerifier(cfg.AccessTeam, cfg.AccessAUD).Middleware(log, map[string]bool{"/healthz": true}, handler)
	} else {
		log.Warn("Cloudflare Access check is off", "component", "access", "reason", "VISTA_ACCESS_TEAM and VISTA_ACCESS_AUD aren't set (dev only)")
	}
	for _, s := range []struct {
		name      string
		connected bool
	}{{"sonar", cfg.SonarURL != "" && cfg.SonarToken != ""}, {"blackbox", cfg.BlackboxURL != "" && cfg.BlackboxToken != ""}} {
		if !s.connected {
			log.Warn("source not connected", "component", "source", "source", s.name)
		}
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("server started", "component", "http", "addr", srv.Addr, "access_check", cfg.AccessEnabled())

	select {
	case err := <-errc:
		log.Error("server stopped", "component", "http", "err", fmt.Errorf("http: %w", err))
		return 1
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil {
		log.Error("shutdown failed", "component", "http", "err", fmt.Errorf("http: %w", err))
		return 1
	}
	log.Info("server stopped", "component", "http")
	return 0
}

// runHealth asks the server in this container; the compose healthcheck runs it.
func runHealth(args []string) int {
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://localhost:" + strings.TrimPrefix(addr(), ":") + "/healthz")
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ health: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "✗ health: the server answered %s\n", resp.Status)
		return 1
	}
	return 0
}

// addr is the listen address, VISTA_ADDR or :3010.
func addr() string {
	if a := os.Getenv("VISTA_ADDR"); a != "" {
		return a
	}
	return ":3010"
}

// version is the deployed release, from VISTA_VERSION.
func version() string {
	if v := os.Getenv("VISTA_VERSION"); v != "" {
		return v
	}
	return "dev"
}
