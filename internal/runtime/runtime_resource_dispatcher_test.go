package runtime

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/message"
)

func TestRuntimeResourceDispatcherBoundsThousandRunnableItems(t *testing.T) {
	t.Parallel()
	governor := newRuntimeResourceGovernor(map[runtimeResourceKind]runtimeResourceLimit{
		runtimeResourceTurnWorkingSet: {Count: 4},
	})
	unblock := make(chan struct{})
	var active atomic.Int64
	var maximum atomic.Int64
	dispatcher := newRuntimeResourceDispatcher(governor, runtimeResourceTurnWorkingSet, func(string) {
		current := active.Add(1)
		for {
			observed := maximum.Load()
			if current <= observed || maximum.CompareAndSwap(observed, current) {
				break
			}
		}
		<-unblock
		active.Add(-1)
	})
	for index := 0; index < 1000; index++ {
		if !dispatcher.enqueue(fmt.Sprintf("turn-%04d", index)) {
			t.Fatalf("turn %d was not enqueued", index)
		}
	}
	waitForRuntimeDispatcherCounts(t, dispatcher, 996, 4)
	status := governor.status()
	if len(status.Resources) != 1 || status.Resources[0].InUseCount != 4 || status.Resources[0].QueuedCount != 996 {
		t.Fatalf("governor status = %#v", status.Resources)
	}
	for index := 4; index < 504; index++ {
		if !dispatcher.cancel(fmt.Sprintf("turn-%04d", index)) {
			t.Fatalf("queued turn %d was not cancelled", index)
		}
	}
	waitForRuntimeDispatcherCounts(t, dispatcher, 496, 4)
	close(unblock)
	waitForRuntimeDispatcherCounts(t, dispatcher, 0, 0)
	if got := maximum.Load(); got != 4 {
		t.Fatalf("maximum concurrent executions = %d, want 4", got)
	}
}

func TestRuntimeModelDispatcherHydratesPromptOnlyAfterAdmission(t *testing.T) {
	service, releaseDB := runtimeRunTransitionWriterTestService(t)
	defer releaseDB()
	runtimeWorkbench, workspace := workbenchForSkillTest(t)
	service.runtime = runtimeWorkbench
	service.workspace = &workspace
	service.runtimeCtx = context.Background()
	service.userInputs = newRuntimeUserInputStore(service.turns.db)
	service.resourceGovernor = newRuntimeResourceGovernor(map[runtimeResourceKind]runtimeResourceLimit{
		runtimeResourceTurnWorkingSet: {Count: 1},
	})
	service.turnDispatcher = newRuntimeResourceDispatcher(service.resourceGovernor, runtimeResourceTurnWorkingSet, service.runQueuedModelTurn)

	started := make(chan string, 2)
	unblock := make(chan struct{})
	coordinator := &blockingModelCoordinator{
		phase25RuntimeWorkbenchCoordinator: &phase25RuntimeWorkbenchCoordinator{service: service},
		started:                            started,
		unblock:                            unblock,
	}
	workspaceRuntime, err := runtimeWorkbench.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	workspaceRuntime.AgentCoordinator = coordinator

	for index, prompt := range []string{"prompt A", "prompt B"} {
		session, err := runtimeWorkbench.CreateSession(context.Background(), workspace.ID, fmt.Sprintf("Session %d", index))
		if err != nil {
			t.Fatal(err)
		}
		turnID := fmt.Sprintf("model-turn-%d", index)
		input := RuntimeNormalizedInput{ID: "input-" + turnID, SessionID: session.ID, Mode: runtimeInputModePrompt, Prompt: prompt, CreatedAt: time.Now().UnixMilli()}
		if _, err := service.userInputs.Upsert(context.Background(), input, nil, turnID); err != nil {
			t.Fatal(err)
		}
		if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{ID: turnID, SessionID: session.ID, Status: turnStatusQueued, StartedAt: time.Now().UnixMilli()}); err != nil {
			t.Fatal(err)
		}
		service.requests[turnID] = runtimeRequestState{SessionID: session.ID, Status: turnStatusQueued, StartedAt: time.Now().UnixMilli()}
		if !service.turnDispatcher.enqueue(turnID) {
			t.Fatalf("failed to enqueue %s", turnID)
		}
	}

	select {
	case prompt := <-started:
		if prompt != "prompt A" {
			t.Fatalf("first hydrated prompt = %q", prompt)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first model turn did not start")
	}
	select {
	case prompt := <-started:
		t.Fatalf("second prompt %q hydrated before model admission", prompt)
	case <-time.After(100 * time.Millisecond):
	}
	waitForRuntimeDispatcherCounts(t, service.turnDispatcher, 1, 1)
	second, err := service.turns.Get(context.Background(), "model-turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != turnStatusQueued {
		t.Fatalf("second durable turn status = %q, want queued", second.Status)
	}
	close(unblock)
	select {
	case prompt := <-started:
		if prompt != "prompt B" {
			t.Fatalf("second hydrated prompt = %q", prompt)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second model turn did not start after release")
	}
	waitForRuntimeDispatcherCounts(t, service.turnDispatcher, 0, 0)
}

func TestRuntimeQueuedModelRecoveryRebuildsOnlyLightweightRunnableIDs(t *testing.T) {
	service, releaseDB := runtimeRunTransitionWriterTestService(t)
	defer releaseDB()
	service.userInputs = newRuntimeUserInputStore(service.turns.db)
	service.resourceGovernor = newRuntimeResourceGovernor(map[runtimeResourceKind]runtimeResourceLimit{
		runtimeResourceTurnWorkingSet: {Count: 1},
	})
	occupied, ok := service.resourceGovernor.tryAcquire(runtimeResourceTurnWorkingSet, 1, 0)
	if !ok {
		t.Fatal("failed to occupy model resource for recovery test")
	}
	defer occupied()
	service.turnDispatcher = newRuntimeResourceDispatcher(service.resourceGovernor, runtimeResourceTurnWorkingSet, service.runQueuedModelTurn)
	for index := 0; index < 100; index++ {
		turnID := fmt.Sprintf("recovered-%03d", index)
		input := RuntimeNormalizedInput{ID: "input-" + turnID, SessionID: fmt.Sprintf("session-%03d", index), Mode: runtimeInputModePrompt, Prompt: "durable prompt", CreatedAt: int64(index + 1)}
		if _, err := service.userInputs.Upsert(context.Background(), input, nil, turnID); err != nil {
			t.Fatal(err)
		}
		if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{ID: turnID, SessionID: input.SessionID, Status: turnStatusQueued, StartedAt: int64(index + 1)}); err != nil {
			t.Fatal(err)
		}
	}
	recovered, err := service.recoverQueuedModelTurns(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 100 {
		t.Fatalf("recovered queued turns = %d, want 100", len(recovered))
	}
	waitForRuntimeDispatcherCounts(t, service.turnDispatcher, 100, 0)
	if got := service.resourceGovernor.status().Resources[0].QueuedCount; got != 100 {
		t.Fatalf("queued model governor count = %d, want 100", got)
	}
	for _, turn := range recovered {
		if !service.turnDispatcher.cancel(turn.ID) {
			t.Fatalf("recovered turn %s was not cancellable", turn.ID)
		}
	}
}

type blockingModelCoordinator struct {
	*phase25RuntimeWorkbenchCoordinator
	started chan<- string
	unblock <-chan struct{}
}

func (c *blockingModelCoordinator) Run(ctx context.Context, sessionID, turnID, prompt string, attachments ...message.Attachment) (*fantasy.AgentResult, error) {
	c.started <- prompt
	select {
	case <-c.unblock:
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *blockingModelCoordinator) RunWithMetadata(ctx context.Context, sessionID, turnID, prompt string, _ map[string]string, attachments ...message.Attachment) (*fantasy.AgentResult, error) {
	return c.Run(ctx, sessionID, turnID, prompt, attachments...)
}

var _ agent.Coordinator = (*blockingModelCoordinator)(nil)

func waitForRuntimeDispatcherCounts(t *testing.T, dispatcher *runtimeResourceDispatcher, wantQueued, wantRunning int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		queued, running := dispatcher.counts()
		if queued == wantQueued && running == wantRunning {
			return
		}
		time.Sleep(time.Millisecond)
	}
	queued, running := dispatcher.counts()
	t.Fatalf("dispatcher counts = queued:%d running:%d, want %d/%d", queued, running, wantQueued, wantRunning)
}
