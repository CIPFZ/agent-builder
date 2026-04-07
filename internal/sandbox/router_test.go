package sandbox

import (
	"context"
	"runtime"
	"testing"

	"myclaw/internal/session"
)

func TestRouterRoutesBySessionMainFlag(t *testing.T) {
	router := NewRouter(nil, nil)
	command := "printf hi"
	if runtime.GOOS == "windows" {
		command = "Write-Output hi"
	}

	mainOut, err := router.Run(context.Background(), session.Session{IsMain: true}, command)
	if err != nil {
		t.Fatalf("host run: %v", err)
	}
	if mainOut != "hi" {
		t.Fatalf("main output = %q, want %q", mainOut, "hi")
	}

	sandboxOut, err := router.Run(context.Background(), session.Session{IsMain: false}, command)
	if err != nil {
		t.Fatalf("sandbox run: %v", err)
	}
	if sandboxOut != "[sandbox] "+command {
		t.Fatalf("sandbox output = %q, want sandbox marker", sandboxOut)
	}
}
