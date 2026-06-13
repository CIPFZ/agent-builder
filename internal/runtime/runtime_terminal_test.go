package runtime

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRuntimeTerminalStreamsPersistentShellOutput(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	created, err := service.CreateTerminal(context.Background(), RuntimeTerminalCreateRequest{
		ID:      "term-pty",
		CWD:     root,
		Columns: 100,
		Rows:    24,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Terminal.ID != "term-pty" || created.Terminal.CWD != root || created.Terminal.Status != "running" {
		t.Fatalf("created terminal = %#v", created.Terminal)
	}
	defer func() {
		_, _ = service.DeleteTerminal(context.Background(), "term-pty")
	}()

	events, unsubscribe := service.SubscribeTerminalEvents(context.Background(), "term-pty")
	defer unsubscribe()

	if _, err := service.WriteTerminalInput(context.Background(), "term-pty", RuntimeTerminalInputRequest{Data: terminalTestCommand("echo agent-builder-terminal")}); err != nil {
		t.Fatal(err)
	}
	if !waitForTerminalChunk(t, events, "agent-builder-terminal") {
		t.Fatal("terminal output missing echo result")
	}

	if _, err := service.WriteTerminalInput(context.Background(), "term-pty", RuntimeTerminalInputRequest{Data: terminalTestCommand("cd nested")}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.WriteTerminalInput(context.Background(), "term-pty", RuntimeTerminalInputRequest{Data: terminalTestCommand(terminalPrintWorkingDirectoryCommand())}); err != nil {
		t.Fatal(err)
	}
	if !waitForTerminalChunk(t, events, "nested") {
		t.Fatalf("terminal output missing nested cwd %q", nested)
	}
}

func TestRuntimeTerminalResizeAndClose(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	created, err := service.CreateTerminal(context.Background(), RuntimeTerminalCreateRequest{
		ID:      "term-resize",
		CWD:     t.TempDir(),
		Columns: 80,
		Rows:    20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Terminal.Columns != 80 || created.Terminal.Rows != 20 {
		t.Fatalf("initial size = %dx%d", created.Terminal.Columns, created.Terminal.Rows)
	}

	resized, err := service.ResizeTerminal(context.Background(), "term-resize", RuntimeTerminalResizeRequest{
		Columns: 120,
		Rows:    32,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resized.Terminal.Columns != 120 || resized.Terminal.Rows != 32 {
		t.Fatalf("resized terminal = %#v", resized.Terminal)
	}

	closed, err := service.DeleteTerminal(context.Background(), "term-resize")
	if err != nil {
		t.Fatal(err)
	}
	if closed.Terminal.Status != "closed" && closed.Terminal.Status != "exited" {
		t.Fatalf("closed terminal status = %q", closed.Terminal.Status)
	}
	if _, err := service.WriteTerminalInput(context.Background(), "term-resize", RuntimeTerminalInputRequest{Data: terminalTestCommand("echo after-close")}); err == nil {
		t.Fatal("WriteTerminalInput should fail for closed terminal")
	}
}

func TestRuntimeTerminalEventHistoryIsByteBounded(t *testing.T) {
	t.Parallel()

	state := &runtimeTerminalState{
		ID:          "term-history",
		Status:      "running",
		Subscribers: make(map[*runtimeTerminalSubscriber]struct{}),
	}
	chunk := strings.Repeat("x", 1024*1024)
	for i := 0; i < 12; i++ {
		state.publish(RuntimeTerminalEvent{Data: chunk})
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if state.EventBytes > runtimeTerminalMaxEventBytes {
		t.Fatalf("event history retained %d bytes, want <= %d", state.EventBytes, runtimeTerminalMaxEventBytes)
	}
	if len(state.Events) != 8 {
		t.Fatalf("event count = %d, want 8", len(state.Events))
	}
	if state.Events[0].Sequence != 5 || state.Events[len(state.Events)-1].Sequence != 12 {
		t.Fatalf("retained sequence range = %d..%d", state.Events[0].Sequence, state.Events[len(state.Events)-1].Sequence)
	}
}

func TestRuntimeTerminalOutputEventPreservesNonUTF8Bytes(t *testing.T) {
	t.Parallel()

	event := runtimeTerminalOutputEvent("term-binary", []byte{0xff, 0xfe, 0x00, 'A'})
	if event.Data != "" {
		t.Fatalf("non-UTF8 event should not use data field: %#v", event)
	}
	decoded, err := base64.StdEncoding.DecodeString(event.BinaryB64)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != string([]byte{0xff, 0xfe, 0x00, 'A'}) {
		t.Fatalf("decoded bytes = %#v", decoded)
	}
}

func TestRuntimeRestartClosesActiveTerminals(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	created, err := service.CreateTerminal(context.Background(), RuntimeTerminalCreateRequest{
		ID:  "term-restart",
		CWD: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Terminal.Status != "running" {
		t.Fatalf("created terminal = %#v", created.Terminal)
	}

	service.restart()

	if _, err := service.WriteTerminalInput(context.Background(), "term-restart", RuntimeTerminalInputRequest{Data: terminalTestCommand("echo after-restart")}); err == nil {
		t.Fatal("WriteTerminalInput should fail after runtime restart closes terminals")
	}
	service.mu.Lock()
	terminalCount := len(service.terminals)
	service.mu.Unlock()
	if terminalCount != 0 {
		t.Fatalf("terminal count after restart = %d, want 0", terminalCount)
	}
}

func waitForTerminalChunk(t *testing.T, events <-chan RuntimeTerminalEvent, want string) bool {
	t.Helper()
	deadline := time.After(5 * time.Second)
	var seen strings.Builder
	for {
		select {
		case event := <-events:
			seen.WriteString(event.Data)
			if strings.Contains(normalizeTerminalText(seen.String()), normalizeTerminalText(want)) {
				return true
			}
		case <-deadline:
			t.Logf("terminal output before timeout: %q", normalizeTerminalText(seen.String()))
			return false
		}
	}
}

func terminalTestCommand(command string) string {
	return command + "\r"
}

func terminalPrintWorkingDirectoryCommand() string {
	if runtime.GOOS == "windows" {
		return "pwd"
	}
	return "pwd"
}

func normalizeTerminalText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\\", "/")
	return text
}
