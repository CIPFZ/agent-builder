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
)

type RunContext struct {
	RunID          string
	ChildSessionID string
	ChildSessionKey string
}

type SpawnRequest struct {
	ParentSessionID string
	ParentAgentID   string
	ChildSessionID  string
	ChildSessionKey string
	Label           string
	Prompt          string
	Run             func(context.Context, RunContext) (string, error)
}

type Run struct {
	ID             string
	ParentSessionID string
	ParentAgentID  string
	ChildSessionID string
	ChildSessionKey string
	Label          string
	Prompt         string
	Status         Status
	Output         string
	Err            error
	cancel         context.CancelFunc
	done           chan struct{}
}

type Manager struct {
	nextID atomic.Uint64
	mu     sync.RWMutex
	runs   map[string]*Run
	controlMessages map[string][]string
}

func NewManager() *Manager {
	return &Manager{
		runs:            make(map[string]*Run),
		controlMessages: make(map[string][]string),
	}
}

func (m *Manager) Spawn(ctx context.Context, req SpawnRequest) (*Run, error) {
	if req.Run == nil {
		return nil, fmt.Errorf("spawn run func is required")
	}
	id := fmt.Sprintf("agent-%06d", m.nextID.Add(1))
	runCtx, cancel := context.WithCancel(ctx)
	run := &Run{
		ID:              id,
		ParentSessionID: req.ParentSessionID,
		ParentAgentID:   req.ParentAgentID,
		ChildSessionID:  req.ChildSessionID,
		ChildSessionKey: req.ChildSessionKey,
		Label:           req.Label,
		Prompt:          req.Prompt,
		Status:          StatusRunning,
		cancel:          cancel,
		done:            make(chan struct{}),
	}
	if run.ChildSessionID == "" {
		run.ChildSessionID = fmt.Sprintf("session-%s", id)
	}
	if run.ChildSessionKey == "" {
		run.ChildSessionKey = fmt.Sprintf("agent:%s:child:%s", req.ParentAgentID, id)
	}

	m.mu.Lock()
	m.runs[run.ID] = run
	m.mu.Unlock()

	go func() {
		defer close(run.done)
		output, err := req.Run(runCtx, RunContext{
			RunID:           run.ID,
			ChildSessionID:  run.ChildSessionID,
			ChildSessionKey: run.ChildSessionKey,
		})

		m.mu.Lock()
		defer m.mu.Unlock()
		if runCtx.Err() != nil && run.Status == StatusStopped {
			return
		}
		if err != nil {
			run.Status = StatusFailed
			run.Err = err
			return
		}
		run.Status = StatusCompleted
		run.Output = output
	}()

	return run, nil
}

func (m *Manager) Wait(ctx context.Context, id string, timeout time.Duration) (Run, error) {
	m.mu.RLock()
	run, ok := m.runs[id]
	m.mu.RUnlock()
	if !ok {
		return Run{}, fmt.Errorf("run %q not found", id)
	}

	waitCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	select {
	case <-waitCtx.Done():
		return Run{}, waitCtx.Err()
	case <-run.done:
		m.mu.RLock()
		defer m.mu.RUnlock()
		return *run, nil
	}
}

func (m *Manager) List() []Run {
	m.mu.RLock()
	defer m.mu.RUnlock()

	runs := make([]Run, 0, len(m.runs))
	for _, run := range m.runs {
		runs = append(runs, *run)
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
	run.Status = StatusStopped
	if run.cancel != nil {
		run.cancel()
	}
	return nil
}

func (m *Manager) Steer(id, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.runs[id]; !ok {
		return fmt.Errorf("run %q not found", id)
	}
	m.controlMessages[id] = append(m.controlMessages[id], message)
	return nil
}

func (m *Manager) ControlMessages(id string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	messages := m.controlMessages[id]
	return append([]string(nil), messages...)
}

func (m *Manager) Get(id string) (Run, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	run, ok := m.runs[id]
	if !ok {
		return Run{}, false
	}
	return *run, true
}
