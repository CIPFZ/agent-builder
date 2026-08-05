package lsp

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestManagerIdleTimerResetsAndStops(t *testing.T) {
	manager := &Manager{idleTTL: 25 * time.Millisecond}
	fired := make(chan struct{}, 2)
	manager.idleStop = func() { fired <- struct{}{} }
	manager.scheduleIdleStop()
	time.Sleep(15 * time.Millisecond)
	manager.scheduleIdleStop()
	select {
	case <-fired:
		t.Fatal("reset idle timer fired at the original deadline")
	case <-time.After(15 * time.Millisecond):
	}
	select {
	case <-fired:
	case <-time.After(time.Second):
		t.Fatal("reset idle timer did not fire")
	}
	manager.scheduleIdleStop()
	manager.stopIdleTimer()
	select {
	case <-fired:
		t.Fatal("stopped idle timer fired")
	case <-time.After(40 * time.Millisecond):
	}
}

func TestManagerProjectCapabilityLeaseIsSharedAndReleased(t *testing.T) {
	manager := &Manager{}
	var acquired atomic.Int64
	var released atomic.Int64
	manager.SetResourceAdmission(func(context.Context) (func(), error) {
		acquired.Add(1)
		return func() { released.Add(1) }, nil
	})
	if err := manager.ensureResourceLease(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.ensureResourceLease(context.Background()); err != nil {
		t.Fatal(err)
	}
	if acquired.Load() != 1 {
		t.Fatalf("resource admission count = %d, want 1", acquired.Load())
	}
	manager.releaseResourceLease()
	manager.releaseResourceLease()
	if released.Load() != 1 {
		t.Fatalf("resource release count = %d, want 1", released.Load())
	}
}
