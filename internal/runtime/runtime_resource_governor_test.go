package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/agent"
)

func TestRuntimeResourceGovernorBudgetsDimensionsIndependently(t *testing.T) {
	t.Parallel()
	governor := newRuntimeResourceGovernor(map[runtimeResourceKind]runtimeResourceLimit{
		runtimeResourceModelRequest: {Count: 2},
		runtimeResourceTerminal:     {Count: 3, Bytes: 10},
	})

	releaseModelA, ok := governor.tryAcquire(runtimeResourceModelRequest, 1, 0)
	if !ok {
		t.Fatal("first model request should be admitted")
	}
	releaseModelB, ok := governor.tryAcquire(runtimeResourceModelRequest, 1, 0)
	if !ok {
		t.Fatal("second model request should be admitted")
	}
	if _, ok := governor.tryAcquire(runtimeResourceModelRequest, 1, 0); ok {
		t.Fatal("third model request should exceed only the model count budget")
	}
	releaseTerminal, ok := governor.tryAcquire(runtimeResourceTerminal, 1, 10)
	if !ok {
		t.Fatal("terminal should use its independent budget")
	}
	if _, ok := governor.tryAcquire(runtimeResourceTerminal, 1, 1); ok {
		t.Fatal("terminal byte budget should reject before its count budget")
	}

	releaseModelA()
	releaseModelA()
	if _, ok := governor.tryAcquire(runtimeResourceModelRequest, 1, 0); !ok {
		t.Fatal("idempotent release should restore exactly one model slot")
	}
	releaseModelB()
	releaseTerminal()
	usage := governor.snapshot()
	if got := usage[runtimeResourceTerminal]; got != (runtimeResourceUsage{}) {
		t.Fatalf("terminal usage after release = %#v", got)
	}
}

func TestRuntimeResourceGovernorModelAdmissionWaitsAndWakes(t *testing.T) {
	t.Parallel()
	governor := newRuntimeResourceGovernor(map[runtimeResourceKind]runtimeResourceLimit{
		runtimeResourceModelRequest: {Count: 1},
	})
	first, err := governor.AcquireModel(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan func(), 1)
	go func() {
		release, acquireErr := governor.AcquireModel(context.Background(), 0)
		if acquireErr == nil {
			acquired <- release
		}
	}()
	select {
	case <-acquired:
		t.Fatal("second model request bypassed the limit")
	case <-time.After(50 * time.Millisecond):
	}
	first()
	select {
	case release := <-acquired:
		release()
	case <-time.After(5 * time.Second):
		t.Fatal("model waiter was not woken after release")
	}

	occupied, err := governor.AcquireModel(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := governor.AcquireModel(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled model admission error = %v", err)
	}
	occupied()
}

func TestRuntimeResourceGovernorModelByteAdmissionWaitsAndRejectsOversize(t *testing.T) {
	t.Parallel()
	governor := newRuntimeResourceGovernor(map[runtimeResourceKind]runtimeResourceLimit{
		runtimeResourceModelRequest: {Count: 4, Bytes: 8},
	})
	first, err := governor.AcquireModel(context.Background(), 6)
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan func(), 1)
	go func() {
		release, acquireErr := governor.AcquireModel(context.Background(), 3)
		if acquireErr == nil {
			acquired <- release
		}
	}()
	select {
	case <-acquired:
		t.Fatal("model request bypassed the aggregate byte budget")
	case <-time.After(50 * time.Millisecond):
	}
	status := governor.status().Resources[0]
	if status.InUseCount != 1 || status.InUseBytes != 6 || status.QueuedCount != 1 || status.LimitBytes != 8 {
		t.Fatalf("model byte status = %#v", status)
	}
	first()
	select {
	case release := <-acquired:
		release()
	case <-time.After(5 * time.Second):
		t.Fatal("model byte waiter was not woken")
	}
	if _, err := governor.AcquireModel(context.Background(), 9); err == nil {
		t.Fatal("oversized model payload should fail without queuing")
	}
	status = governor.status().Resources[0]
	if status.InUseCount != 0 || status.InUseBytes != 0 || status.QueuedCount != 0 {
		t.Fatalf("oversized model payload mutated usage: %#v", status)
	}
}

func TestRuntimeResourceGovernorCancelledWaiterLeavesNoQueuedUsage(t *testing.T) {
	t.Parallel()
	governor := newRuntimeResourceGovernor(map[runtimeResourceKind]runtimeResourceLimit{
		runtimeResourceHeavyTool: {Count: 1},
	})
	occupied, err := governor.AcquireTool(context.Background(), agent.ToolResourceHeavy, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer occupied()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, acquireErr := governor.AcquireTool(ctx, agent.ToolResourceHeavy, 4)
		done <- acquireErr
	}()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status := governor.status()
		if len(status.Resources) == 1 && status.Resources[0].QueuedCount == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled tool admission error = %v", err)
	}
	status := governor.status()
	if got := status.Resources[0].QueuedCount; got != 0 {
		t.Fatalf("queued tool count after cancellation = %d, want 0", got)
	}
}

func TestRuntimeResourceGovernorToolClassesAreIndependent(t *testing.T) {
	t.Parallel()
	governor := newRuntimeResourceGovernor(map[runtimeResourceKind]runtimeResourceLimit{
		runtimeResourceHeavyTool:     {Count: 1},
		runtimeResourceShellProcess:  {Count: 1},
		runtimeResourceBrowserWorker: {Count: 1},
	})

	heavy, err := governor.AcquireTool(context.Background(), agent.ToolResourceHeavy, 3)
	if err != nil {
		t.Fatal(err)
	}
	shell, err := governor.AcquireTool(context.Background(), agent.ToolResourceShell, 5)
	if err != nil {
		t.Fatal(err)
	}
	browser, err := governor.AcquireTool(context.Background(), agent.ToolResourceBrowser, 7)
	if err != nil {
		t.Fatal(err)
	}
	status := governor.status()
	wantBytes := map[string]int64{
		string(runtimeResourceHeavyTool):     3,
		string(runtimeResourceShellProcess):  5,
		string(runtimeResourceBrowserWorker): 7,
	}
	for _, resource := range status.Resources {
		if resource.InUseCount != 1 {
			t.Fatalf("resource %s in-use count = %d, want 1", resource.Kind, resource.InUseCount)
		}
		if resource.InUseBytes != wantBytes[resource.Kind] {
			t.Fatalf("resource %s in-use bytes = %d, want %d", resource.Kind, resource.InUseBytes, wantBytes[resource.Kind])
		}
	}
	heavy()
	shell()
	browser()
}

func TestRuntimeResourceGovernorRejectsOversizedToolWithoutQueuing(t *testing.T) {
	t.Parallel()
	governor := newRuntimeResourceGovernor(map[runtimeResourceKind]runtimeResourceLimit{
		runtimeResourceHeavyTool: {Count: 1, Bytes: 8},
	})
	if _, err := governor.AcquireTool(context.Background(), agent.ToolResourceHeavy, 9); err == nil {
		t.Fatal("oversized tool input should fail its hard byte budget")
	}
	status := governor.status().Resources[0]
	if status.InUseCount != 0 || status.InUseBytes != 0 || status.QueuedCount != 0 {
		t.Fatalf("oversized request mutated resource status: %#v", status)
	}
}

func TestRuntimeResourceGovernorStatusIsBoundedAndDeterministic(t *testing.T) {
	t.Parallel()
	governor := newRuntimeResourceGovernor(map[runtimeResourceKind]runtimeResourceLimit{
		runtimeResourceTerminal:     {Count: 8, Bytes: 32},
		runtimeResourceModelRequest: {Count: 16},
	})
	release, ok := governor.tryAcquire(runtimeResourceTerminal, 1, 4)
	if !ok {
		t.Fatal("terminal should be admitted")
	}
	defer release()
	status := governor.status()
	if len(status.Resources) != 2 || status.Resources[0].Kind != string(runtimeResourceModelRequest) || status.Resources[1].Kind != string(runtimeResourceTerminal) {
		t.Fatalf("resource status order = %#v", status.Resources)
	}
	terminal := status.Resources[1]
	if terminal.InUseCount != 1 || terminal.InUseBytes != 4 || terminal.LimitCount != 8 || terminal.LimitBytes != 32 {
		t.Fatalf("terminal resource status = %#v", terminal)
	}
}

func TestRuntimeTerminalResourceLeaseTransfersOnReplacement(t *testing.T) {
	t.Parallel()
	governor := newRuntimeResourceGovernor(map[runtimeResourceKind]runtimeResourceLimit{
		runtimeResourceTerminal: {Count: 1, Bytes: runtimeTerminalMaxEventBytes},
	})
	release, ok := governor.tryAcquire(runtimeResourceTerminal, 1, runtimeTerminalMaxEventBytes)
	if !ok {
		t.Fatal("terminal lease should be admitted")
	}
	previous := &runtimeTerminalState{Status: "running", ResourceRelease: release}
	replacement := &runtimeTerminalState{Status: "running", ResourceRelease: previous.takeResourceRelease()}
	previous.close("closed", nil, "replaced")
	previous.releaseResource()
	if got := governor.snapshot()[runtimeResourceTerminal].Count; got != 1 {
		t.Fatalf("replacement should retain one terminal lease, got %d", got)
	}
	replacement.close("closed", nil, "")
	replacement.releaseResource()
	if got := governor.snapshot()[runtimeResourceTerminal].Count; got != 0 {
		t.Fatalf("closing replacement should release terminal lease, got %d", got)
	}
}
