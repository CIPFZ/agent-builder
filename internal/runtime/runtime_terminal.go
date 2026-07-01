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
	ProjectID   string
	SessionID   string
	Title       string
	CWD         string
	InitialCWD  string
	Shell       string
	ShellPath   string
	ShellArgs   []string
	Columns     int
	Rows        int
	Status      string
	ExitCode    *int
	CreatedAt   int64
	UpdatedAt   int64
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
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return RuntimeTerminalResponse{}, errors.New("terminal session id is required")
	}
	projectID, projectPath, err := r.runtimeTerminalProject(ctx, sessionID)
	if err != nil {
		return RuntimeTerminalResponse{}, err
	}
	cwd, err := runtimeTerminalInitialCWD(projectPath, req.CWD)
	if err != nil {
		return RuntimeTerminalResponse{}, err
	}
	if !isPathInside(projectPath, cwd) {
		return RuntimeTerminalResponse{}, fmt.Errorf("terminal cwd %s is outside project path %s", cwd, projectPath)
	}
	if strings.TrimSpace(req.ProfileID) == "" {
		settings, settingsErr := loadRuntimeTerminalSettings()
		if settingsErr != nil {
			return RuntimeTerminalResponse{}, settingsErr
		}
		req.ProfileID = settings.ProfileID
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

	now := time.Now().UnixMilli()
	state := &runtimeTerminalState{
		ID:          id,
		ProjectID:   projectID,
		SessionID:   sessionID,
		Title:       profile.title,
		CWD:         cwd,
		InitialCWD:  cwd,
		Shell:       profile.name,
		ShellPath:   profile.path,
		ShellArgs:   append([]string(nil), profile.args...),
		Columns:     columns,
		Rows:        rows,
		Status:      "running",
		CreatedAt:   now,
		UpdatedAt:   now,
		PTY:         terminalPTY,
		Command:     cmd,
		Cancel:      cancel,
		Subscribers: make(map[*runtimeTerminalSubscriber]struct{}),
	}

	r.mu.Lock()
	if r.terminalsByID == nil {
		r.terminalsByID = make(map[string]*runtimeTerminalState)
	}
	if r.terminalIDsBySession == nil {
		r.terminalIDsBySession = make(map[string]map[string]struct{})
	}
	previous := r.terminalsByID[id]
	if previous != nil {
		r.removeRuntimeTerminalOwnershipLocked(previous.SessionID, id)
	}
	r.terminalsByID[id] = state
	if r.terminalIDsBySession[sessionID] == nil {
		r.terminalIDsBySession[sessionID] = make(map[string]struct{})
	}
	r.terminalIDsBySession[sessionID][id] = struct{}{}
	r.mu.Unlock()
	if previous != nil {
		previous.close("closed", nil, "replaced by a new terminal with the same id")
	}

	go state.readLoop()
	go state.waitLoop()

	return RuntimeTerminalResponse{Terminal: state.dto()}, nil
}

func (r *runtimeService) SessionTerminals(ctx context.Context, sessionID string) (RuntimeSessionTerminalsResponse, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return RuntimeSessionTerminalsResponse{}, errors.New("session id is required")
	}
	if _, _, err := r.runtimeTerminalProject(ctx, sessionID); err != nil {
		return RuntimeSessionTerminalsResponse{}, err
	}

	r.mu.Lock()
	states := make([]*runtimeTerminalState, 0, len(r.terminalIDsBySession[sessionID]))
	for id := range r.terminalIDsBySession[sessionID] {
		if state := r.terminalsByID[id]; state != nil {
			states = append(states, state)
		}
	}
	r.mu.Unlock()

	terminals := make([]RuntimeTerminal, 0, len(states))
	for _, state := range states {
		terminals = append(terminals, state.dto())
	}
	return RuntimeSessionTerminalsResponse{SessionID: sessionID, Terminals: terminals}, nil
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
	delete(r.terminalsByID, strings.TrimSpace(terminalID))
	r.removeRuntimeTerminalOwnershipLocked(state.SessionID, strings.TrimSpace(terminalID))
	r.mu.Unlock()

	return RuntimeTerminalResponse{Terminal: state.dto()}, nil
}

func (r *runtimeService) closeRuntimeTerminals(status, errorText string) {
	r.mu.Lock()
	terminals := make([]*runtimeTerminalState, 0, len(r.terminalsByID))
	for _, state := range r.terminalsByID {
		terminals = append(terminals, state)
	}
	r.terminalsByID = make(map[string]*runtimeTerminalState)
	r.terminalIDsBySession = make(map[string]map[string]struct{})
	r.mu.Unlock()

	for _, state := range terminals {
		state.close(status, nil, errorText)
	}
}

func (r *runtimeService) closeRuntimeTerminalsForSession(sessionID, status, errorText string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	r.mu.Lock()
	states := make([]*runtimeTerminalState, 0, len(r.terminalIDsBySession[sessionID]))
	for id := range r.terminalIDsBySession[sessionID] {
		if state := r.terminalsByID[id]; state != nil {
			states = append(states, state)
		}
		delete(r.terminalsByID, id)
	}
	delete(r.terminalIDsBySession, sessionID)
	r.mu.Unlock()

	for _, state := range states {
		state.close(status, nil, errorText)
	}
}

func (r *runtimeService) runtimeTerminal(terminalID string) (*runtimeTerminalState, error) {
	terminalID = strings.TrimSpace(terminalID)
	if terminalID == "" {
		return nil, errRuntimeTerminalMissing
	}
	r.mu.Lock()
	state := r.terminalsByID[terminalID]
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
	s.UpdatedAt = time.Now().UnixMilli()
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
		ID:         s.ID,
		ProjectID:  s.ProjectID,
		SessionID:  s.SessionID,
		Title:      s.Title,
		CWD:        s.CWD,
		InitialCWD: s.InitialCWD,
		Shell:      s.Shell,
		ShellPath:  s.ShellPath,
		ShellArgs:  append([]string(nil), s.ShellArgs...),
		Columns:    s.Columns,
		Rows:       s.Rows,
		Status:     s.Status,
		ExitCode:   s.ExitCode,
		CreatedAt:  s.CreatedAt,
		UpdatedAt:  s.UpdatedAt,
	}
}

func runtimeTerminalEventSize(event RuntimeTerminalEvent) int {
	return len(event.Data) + len(event.BinaryB64) + len(event.Error)
}

func (r *runtimeService) runtimeTerminalProject(ctx context.Context, sessionID string) (string, string, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return "", "", err
	}
	r.mu.Lock()
	if r.workspace == nil || r.runtime == nil {
		r.mu.Unlock()
		return "", "", errors.New("runtime project is not initialized")
	}
	workspaceID := r.workspace.ID
	projectID := r.activeProjectID
	projectPath := r.workspace.Path
	workbenchService := r.runtime
	r.mu.Unlock()

	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(projectPath) == "" {
		return "", "", errors.New("runtime project is not initialized")
	}
	if _, err := workbenchService.GetSession(ctx, workspaceID, sessionID); err != nil {
		return "", "", fmt.Errorf("failed to read terminal session: %w", err)
	}
	projectPath, err := cleanTerminalCWD(projectPath)
	if err != nil {
		return "", "", err
	}
	return projectID, projectPath, nil
}

func runtimeTerminalInitialCWD(projectPath, requested string) (string, error) {
	if strings.TrimSpace(requested) != "" {
		return cleanTerminalCWD(requested)
	}
	return cleanTerminalCWD(projectPath)
}

func (r *runtimeService) removeRuntimeTerminalOwnershipLocked(sessionID, terminalID string) {
	if r.terminalIDsBySession == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	terminalID = strings.TrimSpace(terminalID)
	if sessionID == "" || terminalID == "" {
		return
	}
	ids := r.terminalIDsBySession[sessionID]
	if ids == nil {
		return
	}
	delete(ids, terminalID)
	if len(ids) == 0 {
		delete(r.terminalIDsBySession, sessionID)
	}
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

type runtimeTerminalProfileDefinition struct {
	id       string
	label    string
	resolver func() (runtimeTerminalShellProfile, error)
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
	profileID := strings.TrimSpace(req.ProfileID)
	if profileID == "" {
		settings, err := loadRuntimeTerminalSettings()
		if err != nil {
			return runtimeTerminalShellProfile{}, err
		}
		profileID = settings.ProfileID
	}
	for _, definition := range runtimeTerminalProfileDefinitions() {
		if definition.id == profileID {
			return definition.resolver()
		}
	}
	return runtimeTerminalShellProfile{}, fmt.Errorf("terminal profile %q is not available on this machine", profileID)
}

func runtimeTerminalAvailableProfiles() []RuntimeTerminalProfile {
	profiles := make([]RuntimeTerminalProfile, 0, len(runtimeTerminalProfileDefinitions()))
	for _, definition := range runtimeTerminalProfileDefinitions() {
		if _, err := definition.resolver(); err == nil {
			profiles = append(profiles, RuntimeTerminalProfile{ID: definition.id, Label: definition.label})
		}
	}
	return profiles
}

func runtimeTerminalProfileDefinitions() []runtimeTerminalProfileDefinition {
	if runtime.GOOS == "windows" {
		return []runtimeTerminalProfileDefinition{
			{id: "git-bash", label: "Git Bash", resolver: runtimeTerminalGitBashProfile},
			{id: "pwsh", label: "PowerShell 7", resolver: runtimeTerminalPowerShellCoreProfile},
			{id: "powershell", label: "Windows PowerShell", resolver: runtimeTerminalWindowsPowerShellProfile},
			{id: "cmd", label: "Command Prompt", resolver: runtimeTerminalCmdProfile},
		}
	}
	definitions := []runtimeTerminalProfileDefinition{
		{id: "bash", label: "Bash", resolver: runtimeTerminalNamedLoginShellProfile("bash", "Bash")},
		{id: "zsh", label: "Zsh", resolver: runtimeTerminalNamedLoginShellProfile("zsh", "Zsh")},
		{id: "fish", label: "Fish", resolver: runtimeTerminalNamedLoginShellProfile("fish", "Fish")},
		{id: "sh", label: "sh", resolver: runtimeTerminalNamedLoginShellProfile("sh", "sh")},
		{id: "pwsh", label: "PowerShell", resolver: runtimeTerminalPowerShellCoreProfile},
	}
	if envShell := strings.TrimSpace(os.Getenv("SHELL")); envShell != "" {
		envLabel := fmt.Sprintf("System Shell (%s)", filepath.Base(envShell))
		definitions = append([]runtimeTerminalProfileDefinition{
			{id: "system-shell", label: envLabel, resolver: runtimeTerminalEnvShellProfile},
		}, definitions...)
	}
	return definitions
}

func runtimeTerminalGitBashProfile() (runtimeTerminalShellProfile, error) {
	gitBash := runtimeTerminalLookPath(
		runtimeTerminalJoinEnvPath("ProgramFiles", "Git", "bin", "bash.exe"),
		runtimeTerminalJoinEnvPath("ProgramFiles(x86)", "Git", "bin", "bash.exe"),
		runtimeTerminalJoinEnvPath("LocalAppData", "Programs", "Git", "bin", "bash.exe"),
		`C:\Program Files\Git\bin\bash.exe`,
		`C:\Program Files (x86)\Git\bin\bash.exe`,
	)
	if gitBash == "" {
		return runtimeTerminalShellProfile{}, errors.New("Git Bash terminal profile is not available")
	}
	return runtimeTerminalShellProfile{
		name:  "MINGW64",
		title: "Git Bash",
		path:  gitBash,
		args:  []string{"--login", "-i"},
	}, nil
}

func runtimeTerminalWindowsPowerShellProfile() (runtimeTerminalShellProfile, error) {
	powershell := runtimeTerminalLookPath("powershell.exe")
	if powershell == "" {
		return runtimeTerminalShellProfile{}, errors.New("Windows PowerShell terminal profile is not available")
	}
	return runtimeTerminalShellProfile{
		name:  "Windows PowerShell",
		title: "Windows PowerShell",
		path:  powershell,
		args:  []string{"-NoLogo", "-NoProfile"},
	}, nil
}

func runtimeTerminalPowerShellCoreProfile() (runtimeTerminalShellProfile, error) {
	powershell := runtimeTerminalLookPath("pwsh", "pwsh.exe")
	if powershell == "" {
		return runtimeTerminalShellProfile{}, errors.New("PowerShell 7 terminal profile is not available")
	}
	return runtimeTerminalShellProfile{
		name:  "PowerShell 7",
		title: "PowerShell 7",
		path:  powershell,
		args:  []string{"-NoLogo", "-NoProfile"},
	}, nil
}

func runtimeTerminalCmdProfile() (runtimeTerminalShellProfile, error) {
	cmd := runtimeTerminalLookPath("cmd.exe")
	if cmd == "" {
		return runtimeTerminalShellProfile{}, errors.New("cmd terminal profile is not available")
	}
	return runtimeTerminalShellProfile{name: "cmd", title: "cmd", path: cmd}, nil
}

func runtimeTerminalNamedLoginShellProfile(name, title string) func() (runtimeTerminalShellProfile, error) {
	return func() (runtimeTerminalShellProfile, error) {
		path := runtimeTerminalLookPath(name)
		if path == "" {
			return runtimeTerminalShellProfile{}, fmt.Errorf("%s terminal profile is not available", title)
		}
		return runtimeTerminalShellProfile{name: name, title: title, path: path, args: []string{"-l"}}, nil
	}
}

func runtimeTerminalEnvShellProfile() (runtimeTerminalShellProfile, error) {
	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		return runtimeTerminalShellProfile{}, errors.New("SHELL is not set")
	}
	cleanShell, err := filepath.Abs(os.ExpandEnv(shell))
	if err != nil {
		return runtimeTerminalShellProfile{}, err
	}
	if info, err := os.Stat(cleanShell); err != nil || info.IsDir() {
		return runtimeTerminalShellProfile{}, fmt.Errorf("SHELL %s is not executable", shell)
	}
	name := filepath.Base(cleanShell)
	return runtimeTerminalShellProfile{name: name, title: fmt.Sprintf("System Shell (%s)", name), path: cleanShell, args: []string{"-l"}}, nil
}

func runtimeTerminalLookPath(candidates ...string) string {
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
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

func runtimeTerminalJoinEnvPath(envName string, elems ...string) string {
	root := strings.TrimSpace(os.Getenv(envName))
	if root == "" {
		return ""
	}
	return filepath.Join(append([]string{root}, elems...)...)
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
