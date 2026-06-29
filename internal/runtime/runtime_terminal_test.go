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
	root := t.TempDir()
	service, sessionA := runtimeTerminalTestService(t, "terminal-stream", root)
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	created, err := service.CreateTerminal(context.Background(), RuntimeTerminalCreateRequest{
		SessionID: sessionA,
		ID:        "term-pty",
		CWD:       root,
		Columns:   100,
		Rows:      24,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Terminal.ID != "term-pty" || created.Terminal.SessionID != sessionA || created.Terminal.CWD != root || created.Terminal.InitialCWD != root || created.Terminal.Status != "running" {
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
	root := t.TempDir()
	service, sessionA := runtimeTerminalTestService(t, "terminal-resize", root)
	created, err := service.CreateTerminal(context.Background(), RuntimeTerminalCreateRequest{
		SessionID: sessionA,
		ID:        "term-resize",
		CWD:       root,
		Columns:   80,
		Rows:      20,
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

func TestRuntimeTerminalDefaultProfileSelectsAvailablePreferredProfile(t *testing.T) {
	t.Parallel()

	profiles := []RuntimeTerminalProfile{
		{ID: "cmd", Label: "Command Prompt"},
		{ID: "bash", Label: "Bash"},
		{ID: "zsh", Label: "Zsh"},
	}

	got := runtimeTerminalDefaultProfileID(profiles)
	switch runtime.GOOS {
	case "windows":
		if got != "cmd" {
			t.Fatalf("default profile = %q, want cmd", got)
		}
	case "darwin":
		if got != "zsh" {
			t.Fatalf("default profile = %q, want zsh", got)
		}
	default:
		if got != "bash" {
			t.Fatalf("default profile = %q, want bash", got)
		}
	}
}

func TestRuntimeTerminalDefaultProfileFallsBackToFirstAvailableProfile(t *testing.T) {
	t.Parallel()

	profiles := []RuntimeTerminalProfile{{ID: "custom-shell", Label: "Custom Shell"}}
	if got := runtimeTerminalDefaultProfileID(profiles); got != "custom-shell" {
		t.Fatalf("default profile = %q, want custom-shell", got)
	}
	if got := runtimeTerminalDefaultProfileID(nil); got != "" {
		t.Fatalf("default profile with no profiles = %q, want empty", got)
	}
}

func TestRuntimeRestartClosesActiveTerminals(t *testing.T) {
	root := t.TempDir()
	service, sessionA := runtimeTerminalTestService(t, "terminal-restart", root)
	created, err := service.CreateTerminal(context.Background(), RuntimeTerminalCreateRequest{
		SessionID: sessionA,
		ID:        "term-restart",
		CWD:       root,
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
	terminalCount := len(service.terminalsByID)
	ownershipCount := len(service.terminalIDsBySession)
	service.mu.Unlock()
	if terminalCount != 0 || ownershipCount != 0 {
		t.Fatalf("terminal maps after restart = terminals:%d ownership:%d, want 0/0", terminalCount, ownershipCount)
	}
}

func TestRuntimeTerminalRequiresSessionOwnership(t *testing.T) {
	root := t.TempDir()
	service, sessionA := runtimeTerminalTestService(t, "terminal-ownership", root)
	sessionB := runtimeTerminalCreateSession(t, service, "Session B")

	if _, err := service.CreateTerminal(context.Background(), RuntimeTerminalCreateRequest{ID: "missing-session", CWD: root}); err == nil {
		t.Fatal("CreateTerminal without sessionId should fail")
	}
	if _, err := service.CreateTerminal(context.Background(), RuntimeTerminalCreateRequest{SessionID: "missing", ID: "unknown", CWD: root}); err == nil {
		t.Fatal("CreateTerminal with unknown sessionId should fail")
	}

	created, err := service.CreateTerminal(context.Background(), RuntimeTerminalCreateRequest{
		SessionID: sessionA,
		ID:        "term-a",
		CWD:       root,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = service.DeleteTerminal(context.Background(), "term-a") }()
	if created.Terminal.ProjectID == "" || created.Terminal.SessionID != sessionA {
		t.Fatalf("created terminal ownership = %#v", created.Terminal)
	}

	listA, err := service.SessionTerminals(context.Background(), sessionA)
	if err != nil {
		t.Fatal(err)
	}
	if len(listA.Terminals) != 1 || listA.Terminals[0].ID != "term-a" || listA.Terminals[0].SessionID != sessionA {
		t.Fatalf("session A terminals = %#v", listA)
	}
	listB, err := service.SessionTerminals(context.Background(), sessionB)
	if err != nil {
		t.Fatal(err)
	}
	if len(listB.Terminals) != 0 {
		t.Fatalf("session B should not see session A terminal: %#v", listB)
	}

	if _, err := service.SelectSession(context.Background(), sessionB); err != nil {
		t.Fatal(err)
	}
	if _, err := service.WriteTerminalInput(context.Background(), "term-a", RuntimeTerminalInputRequest{Data: terminalTestCommand("echo still-running")}); err != nil {
		t.Fatal(err)
	}
	listA, err = service.SessionTerminals(context.Background(), sessionA)
	if err != nil {
		t.Fatal(err)
	}
	if len(listA.Terminals) != 1 {
		t.Fatalf("session switch closed terminal: %#v", listA)
	}

	if _, err := service.DeleteTerminal(context.Background(), "term-a"); err != nil {
		t.Fatal(err)
	}
	listA, err = service.SessionTerminals(context.Background(), sessionA)
	if err != nil {
		t.Fatal(err)
	}
	if len(listA.Terminals) != 0 {
		t.Fatalf("deleted terminal remains in session list: %#v", listA)
	}
}

func TestRuntimeTerminalSessionDeleteAndReplacementCleanOwnership(t *testing.T) {
	root := t.TempDir()
	service, sessionA := runtimeTerminalTestService(t, "terminal-session-delete", root)
	sessionB := runtimeTerminalCreateSession(t, service, "Session B")

	if _, err := service.CreateTerminal(context.Background(), RuntimeTerminalCreateRequest{SessionID: sessionA, ID: "shared", CWD: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateTerminal(context.Background(), RuntimeTerminalCreateRequest{SessionID: sessionB, ID: "shared", CWD: root}); err != nil {
		t.Fatal(err)
	}
	listA, err := service.SessionTerminals(context.Background(), sessionA)
	if err != nil {
		t.Fatal(err)
	}
	if len(listA.Terminals) != 0 {
		t.Fatalf("replaced terminal left stale session A ownership: %#v", listA)
	}
	listB, err := service.SessionTerminals(context.Background(), sessionB)
	if err != nil {
		t.Fatal(err)
	}
	if len(listB.Terminals) != 1 || listB.Terminals[0].ID != "shared" {
		t.Fatalf("session B replacement ownership = %#v", listB)
	}

	if _, err := service.DeleteSession(context.Background(), sessionB); err != nil {
		t.Fatal(err)
	}
	if _, err := service.WriteTerminalInput(context.Background(), "shared", RuntimeTerminalInputRequest{Data: terminalTestCommand("echo after-delete")}); err == nil {
		t.Fatal("terminal should be closed when owning session is deleted")
	}
	service.mu.Lock()
	terminalCount := len(service.terminalsByID)
	ownershipCount := len(service.terminalIDsBySession)
	service.mu.Unlock()
	if terminalCount != 0 || ownershipCount != 0 {
		t.Fatalf("terminal maps after session delete = terminals:%d ownership:%d, want 0/0", terminalCount, ownershipCount)
	}
}

func TestRuntimeTerminalCWDDefaultsToProjectAndRejectsOutsideProject(t *testing.T) {
	root := t.TempDir()
	service, sessionA := runtimeTerminalTestService(t, "terminal-cwd", root)
	outside := t.TempDir()

	created, err := service.CreateTerminal(context.Background(), RuntimeTerminalCreateRequest{SessionID: sessionA, ID: "term-default-cwd"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = service.DeleteTerminal(context.Background(), "term-default-cwd") }()
	if created.Terminal.CWD != root || created.Terminal.InitialCWD != root {
		t.Fatalf("default cwd = %#v, want %s", created.Terminal, root)
	}
	if _, err := service.CreateTerminal(context.Background(), RuntimeTerminalCreateRequest{SessionID: sessionA, ID: "term-outside", CWD: outside}); err == nil {
		t.Fatal("CreateTerminal should reject cwd outside project path")
	}
}

func runtimeTerminalTestService(t *testing.T, name, projectPath string) (*runtimeService, string) {
	t.Helper()
	root := runtimeDevTestRoot(t, name)
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	writeRuntimeDevModelConfig(t, root, "http://127.0.0.1:1")
	service := newRuntimeService()
	if _, err := service.Status(context.Background()); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	service.workspace.Path = projectPath
	service.mu.Unlock()
	return service, runtimeTerminalCreateSession(t, service, "Session A")
}

func runtimeTerminalCreateSession(t *testing.T, service *runtimeService, title string) string {
	t.Helper()
	service.mu.Lock()
	workspaceID := service.workspace.ID
	service.mu.Unlock()
	sess, err := service.runtime.CreateSession(context.Background(), workspaceID, title)
	if err != nil {
		t.Fatal(err)
	}
	return sess.ID
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
