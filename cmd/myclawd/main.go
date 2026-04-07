package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"myclaw/internal/app"
	"myclaw/internal/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.LoadFromDir(".")
	if err := app.RunDaemon(ctx, cfg, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
