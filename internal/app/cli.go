package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"myclaw/internal/compaction"
	"myclaw/internal/config"
	"myclaw/internal/diagnostics"
	"myclaw/internal/tui"
)

var runTUI = func(ctx context.Context, _ []string, stdout, stderr io.Writer) error {
	cfg, err := configForCLI(".")
	if err != nil {
		return err
	}
	logPath := filepath.Join("logs", "myclaw.jsonl")
	logger, err := diagnostics.NewLogger(logPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "warning: failed to open diagnostics log %q: %v\n", logPath, err)
	}
	if logger != nil {
		defer logger.Close()
	}
	llmLabel := cfg.LLM.Provider
	if cfg.LLM.Model != "" {
		llmLabel = llmLabel + " / " + cfg.LLM.Model
	}
	if cfg.LLM.APIKey == "" {
		llmLabel = "mock / builtin"
	}
	baseURL := "http://" + cfg.HTTPAddr + cfg.WSPath
	return tui.Run(ctx, baseURL, tui.Options{
		LLMLabel: llmLabel,
		Logger:   logger,
	})
}

func configForCLI(dir string) (config.Config, error) {
	cfg, err := configLoadWithFallback(dir)
	if err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func configLoadWithFallback(dir string) (config.Config, error) {
	defer func() {
		_ = recover()
	}()
	return config.LoadFromDir(dir), nil
}

func resolveTUIWorkspaceRoots(baseDir string, roots []string) ([]string, error) {
	if len(roots) > 0 {
		return roots, nil
	}
	base := baseDir
	if strings.TrimSpace(base) == "" {
		base = "."
	}
	if _, err := os.Stat(base); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(base)
	if err != nil {
		return nil, err
	}
	return []string{filepath.Clean(abs)}, nil
}

func newTUICompactor(cfg config.Config) *compaction.Service {
	if !cfg.Compact.VerificationMode {
		return nil
	}
	return compaction.NewService(compaction.Config{
		MaxMessages:             4,
		MaxEstimatedTokens:      48,
		ContextWindowTokens:     64,
		WarningBufferTokens:     28,
		ErrorBufferTokens:       20,
		AutoCompactBufferTokens: 16,
		BlockingBufferTokens:    6,
		PreserveRecentTurns:     2,
		SummaryPrefix:           "Summary:",
	})
}

func RunCLI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if len(args) == 0 {
		printCLIHelp(stdout)
		return nil
	}

	switch args[0] {
	case "help", "--help", "-h":
		printCLIHelp(stdout)
		return nil
	case "version", "--version", "-v":
		_, err := fmt.Fprintln(stdout, "myclaw dev")
		return err
	case "tui":
		return runTUI(ctx, args[1:], stdout, stderr)
	default:
		_, err := fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		if err != nil {
			return err
		}
		printCLIHelp(stderr)
		return errors.New("invalid command")
	}
}

func printCLIHelp(w io.Writer) {
	lines := []string{
		"myclaw - a Go learning replica of openclaw",
		"",
		"Usage:",
		"  myclaw [command]",
		"",
		"Available commands:",
		"  help      Show this help message",
		"  tui       Launch the interactive terminal UI",
		"  version   Print build version",
		"",
		"Planned chat commands:",
		"  /status   Show runtime status",
		"  /new      Create a new session",
		"  /reset    Reset the current session",
	}

	_, _ = fmt.Fprintln(w, strings.Join(lines, "\n"))
}
