package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"myclaw/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := app.LoadRuntimeConfig(".")
	if err := app.RunDaemon(ctx, cfg, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
