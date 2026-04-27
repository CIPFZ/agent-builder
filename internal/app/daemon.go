package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"myclaw/internal/config"
	"myclaw/internal/gateway"
)

var daemonBootstrapRuntime = bootstrapRuntime

type DaemonOptions struct {
	OnStarted func()
}

func RunDaemon(ctx context.Context, cfg config.Config, stdout io.Writer) error {
	return RunDaemonWithOptions(ctx, cfg, stdout, DaemonOptions{})
}

func RunDaemonWithOptions(ctx context.Context, cfg config.Config, stdout io.Writer, options DaemonOptions) error {
	mux, _ := newDaemonHandler(cfg, stdout)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "myclawd listening on http://%s\n", cfg.HTTPAddr)
	_, _ = fmt.Fprintf(stdout, "operator UI available at http://%s/operator/\n", cfg.HTTPAddr)
	if options.OnStarted != nil {
		options.OnStarted()
	}

	errCh := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	case err := <-errCh:
		return err
	}
}

func newDaemonHandler(cfg config.Config, stdout io.Writer) (*http.ServeMux, *gateway.Server) {
	mux := http.NewServeMux()
	logger := log.New(stdout, "[gateway] ", log.LstdFlags)
	bootstrap, err := daemonBootstrapRuntime(".", cfg, bootstrapOptions{
		FallbackWorkspaceRoots: []string{"configs/workspace"},
	})
	if err != nil {
		logger.Printf("failed to bootstrap runtime: %v", err)
		bootstrap, err = daemonBootstrapRuntime(".", config.Default(), bootstrapOptions{
			FallbackWorkspaceRoots: []string{"configs/workspace"},
		})
		if err != nil {
			logger.Printf("failed to bootstrap fallback runtime: %v", err)
			return mux, gateway.NewServerWithOptions(logger, nil, nil, gateway.Options{})
		}
	}
	gatewayServer := gateway.NewServerWithOptions(logger, bootstrap.Sessions, LLMClientFromRuntimeConfig(cfg), gateway.Options{
		PermissionPolicy:       bootstrap.Policy,
		Runner:                 bootstrap.Runner,
		MainLoopModel:          cfg.LLM.Model,
		LLMProvider:            cfg.LLM.Provider,
		DisableMCPPromptSkills: !cfg.MCP.Skills,
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprintln(w, "myclawd is running")
	})
	mux.HandleFunc("/ui", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, webUIHTML(cfg.WSPath))
	})
	operatorHandler := handleOperatorUI()
	mux.Handle("/operator", operatorHandler)
	mux.Handle("/operator/", operatorHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/statusz", StatusHandler(gatewayServer.SessionManager()))
	mux.HandleFunc(cfg.WSPath, gatewayServer.HandleWebSocket)
	return mux, gatewayServer
}
