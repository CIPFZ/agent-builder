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

	if err := app.RunCLI(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		log.Fatal(err)
	}
}
