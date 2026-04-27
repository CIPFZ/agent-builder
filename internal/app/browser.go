package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"runtime"
)

var runBrowserDaemon = RunDaemon
var runBrowserDaemonWithOptions = RunDaemonWithOptions
var openBrowserURL = openURL

func runBrowser(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("browser", flag.ContinueOnError)
	flags.SetOutput(stderr)
	openBrowser := flags.Bool("open", true, "open the operator UI in the default browser")
	addr := flags.String("addr", "", "override the myclawd HTTP address, for example 127.0.0.1:18080")
	if err := flags.Parse(args); err != nil {
		return err
	}

	cfg := LoadRuntimeConfig(".")
	if *addr != "" {
		cfg.HTTPAddr = *addr
		cfg.Server.HTTPAddr = *addr
	}
	url := fmt.Sprintf("http://%s/operator/", cfg.HTTPAddr)
	_, _ = fmt.Fprintf(stdout, "starting myclaw browser at %s\n", url)
	if !*openBrowser {
		_, _ = fmt.Fprintf(stdout, "open manually: %s\n", url)
		return runBrowserDaemon(ctx, cfg, stdout)
	}
	return runBrowserDaemonWithOptions(ctx, cfg, stdout, DaemonOptions{
		OnStarted: func() {
			if err := openBrowserURL(url); err != nil {
				_, _ = fmt.Fprintf(stderr, "warning: failed to open browser: %v\n", err)
				_, _ = fmt.Fprintf(stderr, "open manually: %s\n", url)
			}
		},
	})
}

func openURL(url string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "windows":
		command = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		command = "open"
		args = []string{url}
	default:
		command = "xdg-open"
		args = []string{url}
	}
	return exec.Command(command, args...).Start()
}
