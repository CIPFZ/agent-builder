package runtime

import "sync"

// runtimeResourceDispatcher keeps runnable work as identifiers only. It owns
// one coordinator goroutine regardless of queue depth and materializes a work
// item only after the corresponding governor dimension admits it.
type runtimeResourceDispatcher struct {
	governor *runtimeResourceGovernor
	kind     runtimeResourceKind
	execute  func(string)

	mu         sync.Mutex
	pending    []string
	pendingSet map[string]struct{}
	running    map[string]struct{}
	wake       chan struct{}
	startOnce  sync.Once
}

func newRuntimeResourceDispatcher(governor *runtimeResourceGovernor, kind runtimeResourceKind, execute func(string)) *runtimeResourceDispatcher {
	d := &runtimeResourceDispatcher{
		governor:   governor,
		kind:       kind,
		execute:    execute,
		pendingSet: make(map[string]struct{}),
		running:    make(map[string]struct{}),
		wake:       make(chan struct{}, 1),
	}
	return d
}

func (d *runtimeResourceDispatcher) enqueue(id string) bool {
	if d == nil || id == "" {
		return false
	}
	d.mu.Lock()
	if _, exists := d.pendingSet[id]; exists {
		d.mu.Unlock()
		return false
	}
	if _, exists := d.running[id]; exists {
		d.mu.Unlock()
		return false
	}
	d.pending = append(d.pending, id)
	d.pendingSet[id] = struct{}{}
	d.updateQueuedLocked()
	d.mu.Unlock()
	d.startOnce.Do(func() { go d.loop() })
	d.signal()
	return true
}

func (d *runtimeResourceDispatcher) cancel(id string) bool {
	if d == nil || id == "" {
		return false
	}
	d.mu.Lock()
	_, existed := d.pendingSet[id]
	delete(d.pendingSet, id)
	d.compactPendingLocked()
	d.updateQueuedLocked()
	d.mu.Unlock()
	return existed
}

func (d *runtimeResourceDispatcher) clear() {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.pending = nil
	d.pendingSet = make(map[string]struct{})
	d.updateQueuedLocked()
	d.mu.Unlock()
}

func (d *runtimeResourceDispatcher) counts() (queued, running int) {
	if d == nil {
		return 0, 0
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.pendingSet), len(d.running)
}

func (d *runtimeResourceDispatcher) loop() {
	for range d.wake {
		for d.dispatchOne() {
		}
	}
}

func (d *runtimeResourceDispatcher) dispatchOne() bool {
	d.mu.Lock()
	d.compactPendingLocked()
	if len(d.pending) == 0 {
		d.updateQueuedLocked()
		d.mu.Unlock()
		return false
	}
	id := d.pending[0]
	release, admitted := d.governor.tryAcquire(d.kind, 1, 0)
	if !admitted {
		d.mu.Unlock()
		return false
	}
	d.pending[0] = ""
	d.pending = d.pending[1:]
	delete(d.pendingSet, id)
	d.running[id] = struct{}{}
	d.compactPendingLocked()
	d.updateQueuedLocked()
	d.mu.Unlock()

	go func() {
		defer func() {
			release()
			d.mu.Lock()
			delete(d.running, id)
			d.mu.Unlock()
			d.signal()
		}()
		d.execute(id)
	}()
	return true
}

func (d *runtimeResourceDispatcher) compactPendingLocked() {
	for len(d.pending) > 0 {
		id := d.pending[0]
		if _, exists := d.pendingSet[id]; exists {
			break
		}
		d.pending[0] = ""
		d.pending = d.pending[1:]
	}
	if len(d.pending) == 0 {
		d.pending = nil
	}
}

func (d *runtimeResourceDispatcher) updateQueuedLocked() {
	d.governor.setQueued(d.kind, len(d.pendingSet))
}

func (d *runtimeResourceDispatcher) signal() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}
