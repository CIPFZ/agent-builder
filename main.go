// Package main is the legacy command-line notice entry point.
package main

import (
	"github.com/CIPFZ/agent-builder/internal/cmd"
	_ "github.com/CIPFZ/agent-builder/internal/dns"
	_ "github.com/joho/godotenv/autoload"
)

func main() {
	cmd.Execute()
}
