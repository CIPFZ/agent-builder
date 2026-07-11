// Package cmd contains a minimal non-TUI CLI compatibility entry point.
package cmd

import (
	"fmt"
	"os"
)

// Execute runs the temporary root command.
func Execute() {
	if len(os.Args) > 1 {
		fmt.Fprintf(os.Stderr, "Agent Builder no longer ships the legacy Agent Builder TUI/CLI command %q.\n", os.Args[1])
		os.Exit(2)
	}
	fmt.Fprintln(os.Stdout, "Agent Builder is desktop-first. Start the Wails desktop app from ./desktop.")
}
