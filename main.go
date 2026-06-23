// Package main is the entry point for the Agent Builder runtime and CLI adapter.
//
//	@title			Agent Builder API
//	@version		1.0
//	@description	Agent Builder is a desktop-first AI agent client. This API is served by the local runtime and provides programmatic access to workspaces, sessions, turns, tools, LSP, MCP, and more.
//	@contact.name	Charm
//	@contact.url	https://charm.sh
//	@license.name	MIT
//	@license.url	https://github.com/CIPFZ/agent-builder/blob/main/LICENSE.md
//	@BasePath		/v1
package main

import (
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/CIPFZ/agent-builder/internal/cmd"
	_ "github.com/CIPFZ/agent-builder/internal/dns"
	_ "github.com/joho/godotenv/autoload"
)

func main() {
	if os.Getenv("AGENT_BUILDER_PROFILE") != "" {
		go func() {
			slog.Info("Serving pprof at localhost:6060")
			if httpErr := http.ListenAndServe("localhost:6060", nil); httpErr != nil {
				slog.Error("Failed to pprof listen", "error", httpErr)
			}
		}()
	}

	cmd.Execute()
}
