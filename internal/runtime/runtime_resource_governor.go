package runtime

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/CIPFZ/agent-builder/internal/agent"
)

type runtimeResourceKind string

const (
	runtimeResourceModelRequest      runtimeResourceKind = "model_request"
	runtimeResourceTurnWorkingSet    runtimeResourceKind = "turn_working_set"
	runtimeResourceHeavyTool         runtimeResourceKind = "heavy_tool"
	runtimeResourceShellProcess      runtimeResourceKind = "shell_process"
	runtimeResourceBrowserWorker     runtimeResourceKind = "browser_worker"
	runtimeResourceTerminal          runtimeResourceKind = "terminal"
	runtimeResourceProjectCapability runtimeResourceKind = "project_capability"
)

type runtimeResourceLimit struct {
	Count int
	Bytes int64
}

type runtimeResourceUsage struct {
	Count int
	Bytes int64
}

const (
	runtimeModelRequestBytes   = 128 << 20
	runtimeProjectSetBytes     = 16 << 20
	runtimeHeavyToolInputBytes = 64 << 20
	runtimeShellInputBytes     = 4 << 20
	runtimeBrowserInputBytes   = 32 << 20
)

type runtimeResourceGovernor struct {
	mu     sync.Mutex
	limits map[runtimeResourceKind]runtimeResourceLimit
	usage  map[runtimeResourceKind]runtimeResourceUsage
	queued map[runtimeResourceKind]int
	wait   map[runtimeResourceKind]chan struct{}
}

func newRuntimeResourceGovernor(limits map[runtimeResourceKind]runtimeResourceLimit) *runtimeResourceGovernor {
	ownedLimits := make(map[runtimeResourceKind]runtimeResourceLimit, len(limits))
	for kind, limit := range limits {
		ownedLimits[kind] = limit
	}
	return &runtimeResourceGovernor{
		limits: ownedLimits,
		usage:  make(map[runtimeResourceKind]runtimeResourceUsage),
		queued: make(map[runtimeResourceKind]int),
		wait:   make(map[runtimeResourceKind]chan struct{}),
	}
}

func defaultRuntimeResourceGovernor() *runtimeResourceGovernor {
	return newRuntimeResourceGovernor(map[runtimeResourceKind]runtimeResourceLimit{
		// Only publish budgets that are connected to a real admission point.
		// The remaining kinds are added here as their durable runnable paths
		// gain acquire/release integration.
		runtimeResourceModelRequest:      {Count: 16, Bytes: runtimeModelRequestBytes},
		runtimeResourceTurnWorkingSet:    {Count: 32},
		runtimeResourceHeavyTool:         {Count: 4, Bytes: runtimeHeavyToolInputBytes},
		runtimeResourceShellProcess:      {Count: 8, Bytes: runtimeShellInputBytes},
		runtimeResourceBrowserWorker:     {Count: 2, Bytes: runtimeBrowserInputBytes},
		runtimeResourceTerminal:          {Count: runtimeTerminalMaxResident, Bytes: runtimeTerminalGlobalReplayBytes},
		runtimeResourceProjectCapability: {Count: 2, Bytes: 32 << 20},
	})
}

// tryAcquire reserves one independently budgeted resource working set. A zero
// limit means that dimension is not constrained. The returned release is safe
// to call more than once, which keeps cancellation and natural completion from
// double-decrementing usage.
func (g *runtimeResourceGovernor) tryAcquire(kind runtimeResourceKind, count int, bytes int64) (func(), bool) {
	if g == nil {
		return func() {}, true
	}
	if count < 0 || bytes < 0 {
		return nil, false
	}
	g.mu.Lock()
	if !g.canAcquireLocked(kind, count, bytes) {
		g.mu.Unlock()
		return nil, false
	}
	g.reserveLocked(kind, count, bytes)
	g.mu.Unlock()
	return g.releaseFunc(kind, count, bytes), true
}

func (g *runtimeResourceGovernor) acquire(ctx context.Context, kind runtimeResourceKind, count int, bytes int64) (func(), error) {
	if g == nil {
		return func() {}, nil
	}
	if count < 0 || bytes < 0 {
		return nil, context.Canceled
	}
	g.mu.Lock()
	limit, configured := g.limits[kind]
	g.mu.Unlock()
	if configured && ((limit.Count > 0 && count > limit.Count) || (limit.Bytes > 0 && bytes > limit.Bytes)) {
		return nil, fmt.Errorf("%s resource request exceeds hard limit", kind)
	}
	queued := false
	for {
		if err := ctx.Err(); err != nil {
			if queued {
				g.adjustQueued(kind, -1)
			}
			return nil, err
		}
		g.mu.Lock()
		if g.canAcquireLocked(kind, count, bytes) {
			if queued {
				g.queued[kind]--
				if g.queued[kind] <= 0 {
					delete(g.queued, kind)
				}
			}
			g.reserveLocked(kind, count, bytes)
			g.mu.Unlock()
			return g.releaseFunc(kind, count, bytes), nil
		}
		if !queued {
			g.queued[kind]++
			queued = true
		}
		wait := g.wait[kind]
		if wait == nil {
			wait = make(chan struct{})
			g.wait[kind] = wait
		}
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			g.adjustQueued(kind, -1)
			return nil, ctx.Err()
		case <-wait:
		}
	}
}

func (g *runtimeResourceGovernor) adjustQueued(kind runtimeResourceKind, delta int) {
	g.mu.Lock()
	g.queued[kind] += delta
	if g.queued[kind] <= 0 {
		delete(g.queued, kind)
	}
	g.mu.Unlock()
}

func (g *runtimeResourceGovernor) AcquireModel(ctx context.Context, payloadBytes int64) (func(), error) {
	return g.acquire(ctx, runtimeResourceModelRequest, 1, payloadBytes)
}

func (g *runtimeResourceGovernor) AcquireTool(ctx context.Context, class string, inputBytes int64) (func(), error) {
	var kind runtimeResourceKind
	switch class {
	case agent.ToolResourceHeavy:
		kind = runtimeResourceHeavyTool
	case agent.ToolResourceShell:
		kind = runtimeResourceShellProcess
	case agent.ToolResourceBrowser:
		kind = runtimeResourceBrowserWorker
	default:
		return func() {}, nil
	}
	return g.acquire(ctx, kind, 1, inputBytes)
}

func (g *runtimeResourceGovernor) AcquireProjectCapability(ctx context.Context, residentBytes int64) (func(), error) {
	return g.acquire(ctx, runtimeResourceProjectCapability, 1, residentBytes)
}

func (g *runtimeResourceGovernor) canAcquireLocked(kind runtimeResourceKind, count int, bytes int64) bool {
	limit, configured := g.limits[kind]
	current := g.usage[kind]
	return !configured || ((limit.Count <= 0 || current.Count+count <= limit.Count) && (limit.Bytes <= 0 || current.Bytes+bytes <= limit.Bytes))
}

func (g *runtimeResourceGovernor) reserveLocked(kind runtimeResourceKind, count int, bytes int64) {
	current := g.usage[kind]
	current.Count += count
	current.Bytes += bytes
	g.usage[kind] = current
}

func (g *runtimeResourceGovernor) releaseFunc(kind runtimeResourceKind, count int, bytes int64) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			current := g.usage[kind]
			current.Count -= count
			current.Bytes -= bytes
			if current.Count <= 0 && current.Bytes <= 0 {
				delete(g.usage, kind)
			} else {
				g.usage[kind] = current
			}
			if wait := g.wait[kind]; wait != nil {
				delete(g.wait, kind)
				close(wait)
			}
			g.mu.Unlock()
		})
	}
}

func (g *runtimeResourceGovernor) snapshot() map[runtimeResourceKind]runtimeResourceUsage {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	result := make(map[runtimeResourceKind]runtimeResourceUsage, len(g.usage))
	for kind, usage := range g.usage {
		result[kind] = usage
	}
	return result
}

func (g *runtimeResourceGovernor) status() RuntimeResourceGovernorStatus {
	if g == nil {
		return RuntimeResourceGovernorStatus{Resources: []RuntimeResourceStatus{}}
	}
	g.mu.Lock()
	resources := make([]RuntimeResourceStatus, 0, len(g.limits))
	for kind, limit := range g.limits {
		usage := g.usage[kind]
		resources = append(resources, RuntimeResourceStatus{
			Kind:        string(kind),
			InUseCount:  usage.Count,
			QueuedCount: g.queued[kind],
			LimitCount:  limit.Count,
			InUseBytes:  usage.Bytes,
			LimitBytes:  limit.Bytes,
		})
	}
	g.mu.Unlock()
	sort.Slice(resources, func(i, j int) bool { return resources[i].Kind < resources[j].Kind })
	return RuntimeResourceGovernorStatus{Resources: resources}
}

func (g *runtimeResourceGovernor) setQueued(kind runtimeResourceKind, count int) {
	if g == nil {
		return
	}
	g.mu.Lock()
	if count <= 0 {
		delete(g.queued, kind)
	} else {
		g.queued[kind] = count
	}
	g.mu.Unlock()
}
