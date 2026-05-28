package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
	"github.com/muesli/termenv"

	"sshdash/internal/config"
	"sshdash/internal/dashboard"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfgPath := env("SSHDASH_CONFIG", "config.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		slog.Error("load config", "path", cfgPath, "error", err)
		os.Exit(1)
	}
	slog.Info("loaded config", "path", cfgPath, "services", len(cfg.Services), "apis", len(cfg.APIs))

	addr := net.JoinHostPort(cfg.Server.Host, fmt.Sprintf("%d", cfg.Server.Port))
	server, err := wish.NewServer(
		wish.WithAddress(addr),
		wish.WithHostKeyPath(cfg.Server.HostKeyPath),
		wish.WithMiddleware(
			bubbletea.MiddlewareWithColorProfile(dashboard.NewProgram(cfg, cfgPath), termenv.ANSI256),
			logging.Middleware(),
		),
	)
	if err != nil {
		slog.Error("create ssh server", "error", err)
		os.Exit(1)
	}

	go func() {
		slog.Info("starting ssh dashboard", "address", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			slog.Error("ssh server stopped", "error", err)
			stop()
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown ssh server", "error", err)
		os.Exit(1)
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
