package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Status string

const (
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusStopped   Status = "stopped"
	StatusClosed    Status = "closed"
)

type ControlAction string

const (
	ActionSpawned ControlAction = "spawned"
	ActionSteered ControlAction = "steered"
	ActionResumed ControlAction = "resumed"
	ActionStopped ControlAction = "stopped"
	ActionClosed  ControlAction = "closed"
)

type RunContext struct {
	RunID           string
	ChildSessionID  string
	ChildSessionKey string
}

type SpawnRequest struct {
	ParentSessionID string
	ParentAgentID   string
	ChildSessionID  string
	ChildSessionKey string
	Label           string
	Prompt          string
	AllowedTools    []string
	Model           string
	Effort          string
	Run             func(context.Context, RunContext) (string, error)
}

type Run struct {
	ID              string
	ParentSessionID string
	ParentAgentID   string
	ChildSessionID  string
	ChildSessionKey string
	Label           string
	Prompt          string
	AllowedTools    []string
	Model           string
	Effort          string
	Status          Status
	LastAction      ControlAction
	Attempt         int
	Output          string
	OutputFile      string
	ErrorSummary    string
	ControlMessages []string
	CreatedAt       time.Time
	StartedAt       time.Time
	UpdatedAt       time.Time
	CompletedAt     time.Time
	LastActionAt    time.Time
	Err             error

	cancel       context.CancelFunc
	done         chan struct{}
	controlQueue []string
}

type Manager struct {
	nextID atomic.Uint64
	mu     sync.RWMutex
	runs   map[string]*Run
}

func NewManager() *Manager {
	return &Manager{
		runs: make(map[string]*Run),
	}
}

func (m *Manager) Spawn(ctx context.Context, req SpawnRequest) (*Run, error) {
	if req.Run == nil {
		return nil, fmt.Errorf("spawn run func is required")
	}

	now := time.Now().UTC()
	id := fmt.Sprintf("agent-%06d", m.nextID.Add(1))
	runCtx, cancel := context.WithCancel(ctx)

	m.mu.Lock()
	run := &Run{
		ID:        id,
		CreatedAt: now,
	}
	m.configureRunLocked(run, req, ActionSpawned, now, cancel)
	m.runs[run.ID] = run
	snapshot := cloneRun(run)
	m.mu.Unlock()

	m.launch(req.Run, run, runCtx)
	return &snapshot, nil
}

func (m *Manager) Resume(ctx context.Context, id string, req SpawnRequest) (*Run, error) {
	if req.Run == nil {
		return nil, fmt.Errorf("resume run func is required")
	}

	for {
		m.mu.Lock()
		run, ok := m.runs[id]
		if !ok {
			m.mu.Unlock()
			return nil, fmt.Errorf("run %q not found", id)
		}
		if run.Status == StatusRunning {
			m.mu.Unlock()
			return nil, fmt.Errorf("run %q is still running and cannot be resumed", id)
		}
		if run.Status == StatusClosed {
			m.mu.Unlock()
			return nil, fmt.Errorf("run %q is closed and cannot be resumed", id)
		}
		done := run.done
		if attemptQuiesced(done) {
			now := time.Now().UTC()
			runCtx, cancel := context.WithCancel(ctx)
			m.configureRunLocked(run, req, ActionResumed, now, cancel)
			snapshot := cloneRun(run)
			m.mu.Unlock()

			m.launch(req.Run, run, runCtx)
			return &snapshot, nil
		}
		m.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-done:
		}
	}
}

func (m *Manager) Wait(ctx context.Context, id string, timeout time.Duration) (Run, error) {
	m.mu.RLock()
	run, ok := m.runs[id]
	if !ok {
		m.mu.RUnlock()
		return Run{}, fmt.Errorf("run %q not found", id)
	}
	if run.Status != StatusRunning {
		snapshot := cloneRun(run)
		m.mu.RUnlock()
		return snapshot, nil
	}
	done := run.done
	m.mu.RUnlock()

	waitCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	select {
	case <-waitCtx.Done():
		return Run{}, waitCtx.Err()
	case <-done:
		m.mu.RLock()
		defer m.mu.RUnlock()
		run, ok := m.runs[id]
		if !ok {
			return Run{}, fmt.Errorf("run %q not found", id)
		}
		return cloneRun(run), nil
	}
}

func (m *Manager) List() []Run {
	m.mu.RLock()
	defer m.mu.RUnlock()

	runs := make([]Run, 0, len(m.runs))
	for _, run := range m.runs {
		runs = append(runs, cloneRun(run))
	}
	return runs
}

func (m *Manager) Active() []Run {
	all := m.List()
	active := make([]Run, 0, len(all))
	for _, run := range all {
		if run.Status == StatusRunning {
			active = append(active, run)
		}
	}
	return active
}

func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	run, ok := m.runs[id]
	if !ok {
		return fmt.Errorf("run %q not found", id)
	}
	if run.Status != StatusRunning {
		return nil
	}
	now := time.Now().UTC()
	run.Status = StatusStopped
	run.LastAction = ActionStopped
	run.LastActionAt = now
	run.UpdatedAt = now
	run.CompletedAt = now
	if run.cancel != nil {
		run.cancel()
	}
	return nil
}

func (m *Manager) Close(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	run, ok := m.runs[id]
	if !ok {
		return fmt.Errorf("run %q not found", id)
	}
	if run.Status == StatusRunning {
		return fmt.Errorf("run %q is still running and cannot be closed", id)
	}
	now := time.Now().UTC()
	run.Status = StatusClosed
	run.LastAction = ActionClosed
	run.LastActionAt = now
	run.UpdatedAt = now
	if run.CompletedAt.IsZero() {
		run.CompletedAt = now
	}
	return nil
}

func (m *Manager) Steer(id, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	run, ok := m.runs[id]
	if !ok {
		return fmt.Errorf("run %q not found", id)
	}
	if run.Status != StatusRunning {
		return fmt.Errorf("run %q is not running", id)
	}
	now := time.Now().UTC()
	run.ControlMessages = append(run.ControlMessages, message)
	run.controlQueue = append(run.controlQueue, message)
	run.LastAction = ActionSteered
	run.LastActionAt = now
	run.UpdatedAt = now
	return nil
}

func (m *Manager) DrainControlMessages(id string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	run, ok := m.runs[id]
	if !ok || len(run.controlQueue) == 0 {
		return nil
	}
	messages := append([]string(nil), run.controlQueue...)
	run.controlQueue = nil
	return messages
}

func (m *Manager) ControlMessages(id string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	run, ok := m.runs[id]
	if !ok || len(run.ControlMessages) == 0 {
		return nil
	}
	return append([]string(nil), run.ControlMessages...)
}

func (m *Manager) SetOutputFile(id, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	run, ok := m.runs[id]
	if !ok {
		return fmt.Errorf("run %q not found", id)
	}
	run.OutputFile = path
	run.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *Manager) Get(id string) (Run, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	run, ok := m.runs[id]
	if !ok {
		return Run{}, false
	}
	return cloneRun(run), true
}

func (m *Manager) configureRunLocked(run *Run, req SpawnRequest, action ControlAction, now time.Time, cancel context.CancelFunc) {
	run.ParentSessionID = req.ParentSessionID
	run.ParentAgentID = req.ParentAgentID
	run.ChildSessionID = req.ChildSessionID
	run.ChildSessionKey = req.ChildSessionKey
	run.Label = req.Label
	run.Prompt = req.Prompt
	run.AllowedTools = append([]string(nil), req.AllowedTools...)
	run.Model = req.Model
	run.Effort = req.Effort
	run.Status = StatusRunning
	run.LastAction = action
	run.Attempt++
	run.Output = ""
	run.OutputFile = ""
	run.ErrorSummary = ""
	run.StartedAt = now
	run.UpdatedAt = now
	run.CompletedAt = time.Time{}
	run.LastActionAt = now
	run.Err = nil
	run.cancel = cancel
	run.done = make(chan struct{})
	run.controlQueue = nil
	if run.ChildSessionID == "" {
		run.ChildSessionID = fmt.Sprintf("session-%s", run.ID)
	}
	if run.ChildSessionKey == "" {
		run.ChildSessionKey = fmt.Sprintf("agent:%s:child:%s", req.ParentAgentID, run.ID)
	}
}

func (m *Manager) launch(fn func(context.Context, RunContext) (string, error), run *Run, runCtx context.Context) {
	go func(runID string, childSessionID string, childSessionKey string, done chan struct{}) {
		defer close(done)

		output, err := fn(runCtx, RunContext{
			RunID:           runID,
			ChildSessionID:  childSessionID,
			ChildSessionKey: childSessionKey,
		})

		now := time.Now().UTC()
		m.mu.Lock()
		defer m.mu.Unlock()

		live, ok := m.runs[runID]
		if !ok || live.done != done {
			return
		}
		if runCtx.Err() != nil && (live.Status == StatusStopped || live.Status == StatusClosed) {
			return
		}
		live.UpdatedAt = now
		live.CompletedAt = now
		if err != nil {
			live.Status = StatusFailed
			live.Err = err
			live.ErrorSummary = err.Error()
			return
		}
		live.Status = StatusCompleted
		live.Output = output
		live.ErrorSummary = ""
		live.Err = nil
	}(run.ID, run.ChildSessionID, run.ChildSessionKey, run.done)
}

func cloneRun(run *Run) Run {
	if run == nil {
		return Run{}
	}
	cloned := *run
	cloned.AllowedTools = append([]string(nil), run.AllowedTools...)
	cloned.ControlMessages = append([]string(nil), run.ControlMessages...)
	cloned.cancel = nil
	cloned.done = nil
	cloned.controlQueue = nil
	return cloned
}

func attemptQuiesced(done chan struct{}) bool {
	if done == nil {
		return true
	}
	select {
	case <-done:
		return true
	default:
		return false
	}
}
