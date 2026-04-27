package app

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"myclaw/internal/config"
)

type tuiDaemon struct {
	BaseURL  string
	Embedded bool
	Cleanup  func(context.Context) error
}

func prepareTUIDaemon(ctx context.Context, cfg config.Config, stdout, stderr io.Writer) (tuiDaemon, error) {
	if daemonHealthy(ctx, cfg.HTTPAddr) {
		return tuiDaemon{BaseURL: tuiBaseURL(cfg.HTTPAddr, cfg.WSPath)}, nil
	}
	daemon, err := startEmbeddedTUIDaemon(ctx, cfg, stdout)
	if err != nil {
		return tuiDaemon{}, err
	}
	if daemon.BaseURL != tuiBaseURL(cfg.HTTPAddr, cfg.WSPath) {
		_, _ = fmt.Fprintf(stderr, "warning: configured myclawd endpoint %s was not healthy; started embedded daemon at %s\n", tuiBaseURL(cfg.HTTPAddr, cfg.WSPath), daemon.BaseURL)
	}
	return daemon, nil
}

func daemonHealthy(ctx context.Context, addr string) bool {
	checkCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, "http://"+addr+"/healthz", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func startEmbeddedTUIDaemon(ctx context.Context, cfg config.Config, stdout io.Writer) (tuiDaemon, error) {
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		listener, err = net.Listen("tcp", fallbackLoopbackAddr(cfg.HTTPAddr))
		if err != nil {
			return tuiDaemon{}, fmt.Errorf("start embedded myclawd: %w", err)
		}
	}

	actualAddr := clientAddr(listener.Addr().String())
	serverCfg := cfg
	serverCfg.HTTPAddr = actualAddr
	serverCfg.Server.HTTPAddr = actualAddr
	handler, _ := newDaemonHandler(serverCfg, stdout)
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	if err := waitForEmbeddedDaemon(ctx, actualAddr, errCh); err != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return tuiDaemon{}, err
	}

	return tuiDaemon{
		BaseURL:  tuiBaseURL(actualAddr, cfg.WSPath),
		Embedded: true,
		Cleanup:  server.Shutdown,
	}, nil
}

func waitForEmbeddedDaemon(ctx context.Context, addr string, errCh <-chan error) error {
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if daemonHealthy(ctx, addr) {
			return nil
		}
		select {
		case err := <-errCh:
			if err == nil {
				return fmt.Errorf("embedded myclawd stopped before becoming ready")
			}
			return fmt.Errorf("embedded myclawd failed: %w", err)
		case <-deadline.C:
			return fmt.Errorf("embedded myclawd did not become ready at http://%s/healthz", addr)
		case <-ticker.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func tuiBaseURL(addr, wsPath string) string {
	if strings.TrimSpace(wsPath) == "" {
		wsPath = "/ws"
	}
	if !strings.HasPrefix(wsPath, "/") {
		wsPath = "/" + wsPath
	}
	return "http://" + addr + wsPath
}

func fallbackLoopbackAddr(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || strings.TrimSpace(host) == "" || host == "::" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, "0")
}

func clientAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" || host == "::" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}
