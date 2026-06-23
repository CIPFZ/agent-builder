// Package cmd contains a minimal non-TUI CLI compatibility entry point.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"

	"github.com/CIPFZ/agent-builder/internal/runtime"
)

// Execute runs the temporary root command.
func Execute() {
	if len(os.Args) > 1 && os.Args[1] == "serve-http" {
		serveHTTP()
		return
	}
	if len(os.Args) > 1 {
		fmt.Fprintf(os.Stderr, "Agent Builder no longer ships the legacy Agent Builder TUI/CLI command %q.\n", os.Args[1])
		os.Exit(2)
	}
	fmt.Fprintln(os.Stdout, "Agent Builder is desktop-first. Start the Wails desktop app from ./desktop.")
}

func serveHTTP() {
	address := firstNonEmpty(os.Getenv("AGENT_BUILDER_RUNTIME_HTTP_ADDR"), "127.0.0.1:5183")
	token := firstNonEmpty(os.Getenv("AGENT_BUILDER_RUNTIME_HTTP_TOKEN"), "agent-builder-dev")
	for _, arg := range os.Args[2:] {
		if value, ok := strings.CutPrefix(arg, "--addr="); ok {
			address = value
		}
		if value, ok := strings.CutPrefix(arg, "--token="); ok {
			token = value
		}
	}

	service := runtime.NewRuntimeService()
	endpoint, err := service.ServeHTTP(context.Background(), address, token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start Agent Builder runtime HTTP API: %v\n", err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(endpoint); err != nil {
		slog.Error("Failed to print runtime HTTP endpoint", "error", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	<-ctx.Done()

	if closer, ok := service.(interface {
		CloseHTTP(context.Context) error
	}); ok {
		_ = closer.CloseHTTP(context.Background())
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
