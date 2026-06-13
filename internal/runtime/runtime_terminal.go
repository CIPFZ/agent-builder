package runtime

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	pty "github.com/aymanbagabas/go-pty"
)

const (
	runtimeTerminalDefaultColumns = 100
	runtimeTerminalDefaultRows    = 24
	runtimeTerminalMaxEvents      = 4000
	runtimeTerminalMaxEventBytes  = 8 * 1024 * 1024
	runtimeTerminalSubscriberBase = 128
	runtimeTerminalSubscriberWait = 2 * time.Second
)

var errRuntimeTerminalMissing = errors.New("terminal session not found")

type runtimeTerminalState struct {
	mu          sync.Mutex
	ID          string
	Title       string
	CWD         string
	Shell       string
	ShellPath   string
	ShellArgs   []string
	Columns     int
	Rows        int
	Status      string
	ExitCode    *int
	Error       string
	PTY         pty.Pty
	Command     *pty.Cmd
	Cancel      context.CancelFunc
	Events      []RuntimeTerminalEvent
	EventBytes  int
	NextSeq     int64
	Subscribers map[*runtimeTerminalSubscriber]struct{}
}

type runtimeTerminalSubscriber struct {
	mu     sync.Mutex
	ch     chan RuntimeTerminalEvent
	closed bool
}

func newRuntimeTerminalSubscriber(size int) *runtimeTerminalSubscriber {
	return &runtimeTerminalSubscriber{ch: make(chan RuntimeTerminalEvent, size)}
}

func (s *runtimeTerminalSubscriber) send(event RuntimeTerminalEvent, wait time.Duration) bool {
	timer := time.NewTimer(wait)
	defer timer.Stop()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	select {
	case s.ch <- event:
		return true
	case <-timer.C:
		s.closed = true
		close(s.ch)
		return false
	}
}

func (s *runtimeTerminalSubscriber) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.ch)
}

func (r *runtimeService) CreateTerminal(ctx context.Context, req RuntimeTerminalCreateRequest) (RuntimeTerminalResponse, error) {
	cwd, err := r.runtimeTerminalInitialCWD(ctx, req.CWD)
	if err != nil {
		return RuntimeTerminalResponse{}, err
	}
	profile, err := runtimeTerminalProfile(req)
	if err != nil {
		return RuntimeTerminalResponse{}, err
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = fmt.Sprintf("terminal-%d", time.Now().UnixNano())
	}
	columns := normalizeTerminalColumns(req.Columns)
	rows := normalizeTerminalRows(req.Rows)

	terminalPTY, err := pty.New()
	if err != nil {
		return RuntimeTerminalResponse{}, err
	}
	if err := terminalPTY.Resize(columns, rows); err != nil {
		_ = terminalPTY.Close()
		return RuntimeTerminalResponse{}, err
	}

	terminalCtx, cancel := context.WithCancel(context.Background())
	cmd := terminalPTY.CommandContext(terminalCtx, profile.path, profile.args...)
	cmd.Dir = cwd
	cmd.Env = runtimeTerminalEnv(os.Environ())
	if err := cmd.Start(); err != nil {
		cancel()
		_ = terminalPTY.Close()
		return RuntimeTerminalResponse{}, err
	}

	state := &runtimeTerminalState{
		ID:          id,
		Title:       profile.title,
		CWD:         cwd,
		Shell:       profile.name,
		ShellPath:   profile.path,
		ShellArgs:   append([]string(nil), profile.args...),
		Columns:     columns,
		Rows:        rows,
		Status:      "running",
		PTY:         terminalPTY,
		Command:     cmd,
		Cancel:      cancel,
		Subscribers: make(map[*runtimeTerminalSubscriber]struct{}),
	}

	r.mu.Lock()
	if r.terminals == nil {
		r.terminals = make(map[string]*runtimeTerminalState)
	}
	previous := r.terminals[id]
	r.terminals[id] = state
	r.mu.Unlock()
	if previous != nil {
		previous.close("closed", nil, "replaced by a new terminal with the same id")
	}

	go state.readLoop()
	go state.waitLoop()

	return RuntimeTerminalResponse{Terminal: state.dto()}, nil
}

func (r *runtimeService) WriteTerminalInput(_ context.Context, terminalID string, req RuntimeTerminalInputRequest) (RuntimeTerminalResponse, error) {
	state, err := r.runtimeTerminal(terminalID)
	if err != nil {
		return RuntimeTerminalResponse{}, err
	}
	data := []byte(req.Data)
	if req.BinaryB64 != "" {
		decoded, decodeErr := base64.StdEncoding.DecodeString(req.BinaryB64)
		if decodeErr != nil {
			return RuntimeTerminalResponse{}, decodeErr
		}
		data = decoded
	}
	if len(data) == 0 {
		return RuntimeTerminalResponse{Terminal: state.dto()}, nil
	}

	state.mu.Lock()
	status := state.Status
	ptyHandle := state.PTY
	state.mu.Unlock()
	if status != "running" || ptyHandle == nil {
		return RuntimeTerminalResponse{}, errRuntimeTerminalMissing
	}
	if _, err := ptyHandle.Write(data); err != nil {
		return RuntimeTerminalResponse{}, err
	}
	return RuntimeTerminalResponse{Terminal: state.dto()}, nil
}

func (r *runtimeService) ResizeTerminal(_ context.Context, terminalID string, req RuntimeTerminalResizeRequest) (RuntimeTerminalResponse, error) {
	state, err := r.runtimeTerminal(terminalID)
	if err != nil {
		return RuntimeTerminalResponse{}, err
	}
	columns := normalizeTerminalColumns(req.Columns)
	rows := normalizeTerminalRows(req.Rows)

	state.mu.Lock()
	state.Columns = columns
	state.Rows = rows
	ptyHandle := state.PTY
	running := state.Status == "running"
	state.mu.Unlock()

	if running && ptyHandle != nil {
		if err := ptyHandle.Resize(columns, rows); err != nil {
			return RuntimeTerminalResponse{}, err
		}
	}
	return RuntimeTerminalResponse{Terminal: state.dto()}, nil
}

func (r *runtimeService) SubscribeTerminalEvents(ctx context.Context, terminalID string, afterValues ...int64) (<-chan RuntimeTerminalEvent, func()) {
	state, err := r.runtimeTerminal(terminalID)
	if err != nil {
		ch := make(chan RuntimeTerminalEvent, 1)
		ch <- RuntimeTerminalEvent{
			TerminalID: strings.TrimSpace(terminalID),
			Final:      true,
			Status:     "failed",
			Error:      err.Error(),
		}
		close(ch)
		return ch, func() {}
	}
	after := firstRuntimeTerminalSequence(afterValues)

	state.mu.Lock()
	replayCount := 0
	for _, event := range state.Events {
		if event.Sequence > after {
			replayCount++
		}
	}
	subscriber := newRuntimeTerminalSubscriber(min(runtimeTerminalMaxEvents, replayCount+runtimeTerminalSubscriberBase))
	for _, event := range state.Events {
		if event.Sequence > after {
			subscriber.ch <- event
		}
	}
	if state.Status != "running" {
		subscriber.close()
		state.mu.Unlock()
		return subscriber.ch, func() {}
	}
	state.Subscribers[subscriber] = struct{}{}
	state.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			state.mu.Lock()
			delete(state.Subscribers, subscriber)
			state.mu.Unlock()
			subscriber.close()
		})
	}
	go func() {
		<-ctx.Done()
		unsubscribe()
	}()
	return subscriber.ch, unsubscribe
}

func (r *runtimeService) DeleteTerminal(_ context.Context, terminalID string) (RuntimeTerminalResponse, error) {
	state, err := r.runtimeTerminal(terminalID)
	if err != nil {
		return RuntimeTerminalResponse{}, err
	}
	state.close("closed", nil, "")

	r.mu.Lock()
	delete(r.terminals, strings.TrimSpace(terminalID))
	r.mu.Unlock()

	return RuntimeTerminalResponse{Terminal: state.dto()}, nil
}

func (r *runtimeService) closeRuntimeTerminals(status, errorText string) {
	r.mu.Lock()
	terminals := make([]*runtimeTerminalState, 0, len(r.terminals))
	for _, state := range r.terminals {
		terminals = append(terminals, state)
	}
	r.terminals = make(map[string]*runtimeTerminalState)
	r.mu.Unlock()

	for _, state := range terminals {
		state.close(status, nil, errorText)
	}
}

func (r *runtimeService) runtimeTerminal(terminalID string) (*runtimeTerminalState, error) {
	terminalID = strings.TrimSpace(terminalID)
	if terminalID == "" {
		return nil, errRuntimeTerminalMissing
	}
	r.mu.Lock()
	state := r.terminals[terminalID]
	r.mu.Unlock()
	if state == nil {
		return nil, errRuntimeTerminalMissing
	}
	return state, nil
}

func (s *runtimeTerminalState) readLoop() {
	buffer := make([]byte, 8192)
	for {
		n, err := s.PTY.Read(buffer)
		if n > 0 {
			s.publish(runtimeTerminalOutputEvent(s.ID, buffer[:n]))
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !strings.Contains(strings.ToLower(err.Error()), "file already closed") {
				s.close("failed", nil, err.Error())
			}
			return
		}
	}
}

func runtimeTerminalOutputEvent(terminalID string, data []byte) RuntimeTerminalEvent {
	event := RuntimeTerminalEvent{TerminalID: terminalID}
	if utf8.Valid(data) {
		event.Data = string(data)
	} else {
		event.BinaryB64 = base64.StdEncoding.EncodeToString(data)
	}
	return event
}

func (s *runtimeTerminalState) waitLoop() {
	err := s.Command.Wait()
	exitCode := 0
	status := "exited"
	errorText := ""
	if s.Command.ProcessState != nil {
		exitCode = s.Command.ProcessState.ExitCode()
	}
	if err != nil {
		status = "failed"
		errorText = err.Error()
	}
	s.close(status, &exitCode, errorText)
}

func (s *runtimeTerminalState) close(status string, exitCode *int, errorText string) {
	s.mu.Lock()
	if s.Status != "running" {
		s.mu.Unlock()
		return
	}
	s.Status = status
	s.ExitCode = exitCode
	s.Error = errorText
	cancel := s.Cancel
	ptyHandle := s.PTY
	process := s.Command.Process
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if process != nil && status == "closed" {
		_ = process.Kill()
	}
	if ptyHandle != nil {
		_ = ptyHandle.Close()
	}

	s.publish(RuntimeTerminalEvent{
		TerminalID: s.ID,
		Final:      true,
		Status:     status,
		ExitCode:   exitCode,
		Error:      errorText,
	})
}

func (s *runtimeTerminalState) publish(event RuntimeTerminalEvent) {
	s.mu.Lock()
	s.NextSeq++
	event.Sequence = s.NextSeq
	if event.TerminalID == "" {
		event.TerminalID = s.ID
	}
	s.Events = append(s.Events, event)
	s.EventBytes += runtimeTerminalEventSize(event)
	for len(s.Events) > runtimeTerminalMaxEvents || s.EventBytes > runtimeTerminalMaxEventBytes {
		s.EventBytes -= runtimeTerminalEventSize(s.Events[0])
		copy(s.Events, s.Events[1:])
		s.Events = s.Events[:len(s.Events)-1]
	}
	subscribers := make([]*runtimeTerminalSubscriber, 0, len(s.Subscribers))
	for subscriber := range s.Subscribers {
		subscribers = append(subscribers, subscriber)
	}
	final := event.Final
	s.mu.Unlock()

	var staleSubscribers []*runtimeTerminalSubscriber
	for _, subscriber := range subscribers {
		if !subscriber.send(event, runtimeTerminalSubscriberWait) {
			staleSubscribers = append(staleSubscribers, subscriber)
		}
	}
	if final || len(staleSubscribers) > 0 {
		s.mu.Lock()
		if final {
			for subscriber := range s.Subscribers {
				delete(s.Subscribers, subscriber)
				subscriber.close()
			}
		} else {
			for _, subscriber := range staleSubscribers {
				if _, ok := s.Subscribers[subscriber]; ok {
					delete(s.Subscribers, subscriber)
					subscriber.close()
				}
			}
		}
		s.mu.Unlock()
	}
}

func (s *runtimeTerminalState) dto() RuntimeTerminal {
	s.mu.Lock()
	defer s.mu.Unlock()
	return RuntimeTerminal{
		ID:        s.ID,
		Title:     s.Title,
		CWD:       s.CWD,
		Shell:     s.Shell,
		ShellPath: s.ShellPath,
		ShellArgs: append([]string(nil), s.ShellArgs...),
		Columns:   s.Columns,
		Rows:      s.Rows,
		Status:    s.Status,
		ExitCode:  s.ExitCode,
	}
}

func runtimeTerminalEventSize(event RuntimeTerminalEvent) int {
	return len(event.Data) + len(event.BinaryB64) + len(event.Error)
}

func (r *runtimeService) runtimeTerminalInitialCWD(ctx context.Context, requested string) (string, error) {
	if strings.TrimSpace(requested) != "" {
		return cleanTerminalCWD(requested)
	}
	r.mu.Lock()
	if r.workspace != nil && strings.TrimSpace(r.workspace.Path) != "" {
		cwd := r.workspace.Path
		r.mu.Unlock()
		return cleanTerminalCWD(cwd)
	}
	r.mu.Unlock()
	status, err := r.Status(ctx)
	if err == nil && strings.TrimSpace(status.WorkingDir) != "" {
		return cleanTerminalCWD(status.WorkingDir)
	}
	if cwd, cwdErr := os.Getwd(); cwdErr == nil {
		return cleanTerminalCWD(cwd)
	}
	return "", err
}

func cleanTerminalCWD(cwd string) (string, error) {
	cwd = os.ExpandEnv(strings.TrimSpace(cwd))
	if cwd == "" {
		return "", errors.New("terminal cwd is empty")
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	return filepath.Clean(abs), nil
}

type runtimeTerminalShellProfile struct {
	name  string
	title string
	path  string
	args  []string
}

func runtimeTerminalProfile(req RuntimeTerminalCreateRequest) (runtimeTerminalShellProfile, error) {
	if strings.TrimSpace(req.ShellPath) != "" {
		path := strings.TrimSpace(req.ShellPath)
		return runtimeTerminalShellProfile{
			name:  filepath.Base(path),
			title: filepath.Base(path),
			path:  path,
			args:  append([]string(nil), req.ShellArgs...),
		}, nil
	}
	if runtime.GOOS == "windows" {
		if gitBash := runtimeTerminalLookPath(`C:\Program Files\Git\bin\bash.exe`, `C:\Program Files (x86)\Git\bin\bash.exe`); gitBash != "" {
			return runtimeTerminalShellProfile{
				name:  "MINGW64",
				title: "MINGW64",
				path:  gitBash,
				args:  []string{"--login", "-i"},
			}, nil
		}
		if powershell := runtimeTerminalLookPath("pwsh.exe", "powershell.exe"); powershell != "" {
			return runtimeTerminalShellProfile{
				name:  "PowerShell",
				title: "PowerShell",
				path:  powershell,
				args:  []string{"-NoLogo", "-NoProfile"},
			}, nil
		}
		if cmd := runtimeTerminalLookPath("cmd.exe"); cmd != "" {
			return runtimeTerminalShellProfile{name: "cmd", title: "cmd", path: cmd}, nil
		}
		return runtimeTerminalShellProfile{}, errors.New("no supported Windows shell found")
	}
	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		shell = runtimeTerminalLookPath("bash", "zsh", "sh")
	}
	if shell == "" {
		return runtimeTerminalShellProfile{}, errors.New("no supported shell found")
	}
	name := filepath.Base(shell)
	return runtimeTerminalShellProfile{name: name, title: name, path: shell, args: []string{"-l"}}, nil
}

func runtimeTerminalLookPath(candidates ...string) string {
	for _, candidate := range candidates {
		if strings.Contains(candidate, `\`) || strings.Contains(candidate, `/`) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
			continue
		}
		if path, err := ptyLookPath(candidate); err == nil {
			return path
		}
	}
	return ""
}

func ptyLookPath(name string) (string, error) {
	return exec.LookPath(name)
}

func runtimeTerminalEnv(env []string) []string {
	next := append([]string(nil), env...)
	next = appendOrReplaceEnv(next, "TERM", "xterm-256color")
	next = appendOrReplaceEnv(next, "COLORTERM", "truecolor")
	return next
}

func appendOrReplaceEnv(env []string, key, value string) []string {
	prefix := key + "="
	for index, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[index] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func normalizeTerminalColumns(columns int) int {
	if columns <= 0 {
		return runtimeTerminalDefaultColumns
	}
	if columns < 20 {
		return 20
	}
	if columns > 500 {
		return 500
	}
	return columns
}

func normalizeTerminalRows(rows int) int {
	if rows <= 0 {
		return runtimeTerminalDefaultRows
	}
	if rows < 6 {
		return 6
	}
	if rows > 200 {
		return 200
	}
	return rows
}

func firstRuntimeTerminalSequence(values []int64) int64 {
	if len(values) == 0 || values[0] < 0 {
		return 0
	}
	return values[0]
}
