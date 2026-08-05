package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/config"
)

func TestRuntimeMCPIdleTTLReleasesWorkerAndDefersForActiveTurns(t *testing.T) {
	service := newRuntimeService()
	service.mcpIdleTTL = 20 * time.Millisecond
	service.resourceGovernor = newRuntimeResourceGovernor(map[runtimeResourceKind]runtimeResourceLimit{
		runtimeResourceBrowserWorker: {Count: 1},
	})
	name := "browser-idle-ttl"
	heavy := config.MCPConfig{Type: config.MCPStdio, Command: "playwright"}
	store := config.NewTestStore(&config.Config{MCP: config.MCPs{name: heavy}})
	acquired, err := service.acquireMCPServerResource(context.Background(), name, heavy)
	if err != nil || !acquired {
		t.Fatalf("browser worker acquire = %v, %v", acquired, err)
	}
	service.mu.Lock()
	service.sessionTurns["session-active"] = "turn-active"
	service.mu.Unlock()
	service.scheduleMCPServerIdleEviction(store, name)
	time.Sleep(60 * time.Millisecond)
	if got := service.resourceGovernor.status().Resources[0].InUseCount; got != 1 {
		t.Fatalf("active Turn should defer idle eviction, worker count = %d", got)
	}
	service.mu.Lock()
	delete(service.sessionTurns, "session-active")
	service.mu.Unlock()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && service.resourceGovernor.status().Resources[0].InUseCount != 0 {
		time.Sleep(time.Millisecond)
	}
	if got := service.resourceGovernor.status().Resources[0].InUseCount; got != 0 {
		t.Fatalf("idle MCP worker lease was not released, count = %d", got)
	}
	service.cancelAllMCPIdleTimers()
}

func TestRuntimeMCPBrowserWorkersRetainAndReleaseResourceLeases(t *testing.T) {
	t.Parallel()
	service := newRuntimeService()
	service.resourceGovernor = newRuntimeResourceGovernor(map[runtimeResourceKind]runtimeResourceLimit{
		runtimeResourceBrowserWorker: {Count: 2},
	})
	heavy := config.MCPConfig{Type: config.MCPStdio, Command: "npx", Args: []string{"@playwright/mcp"}}
	for _, name := range []string{"browser-a", "browser-b"} {
		acquired, err := service.acquireMCPServerResource(context.Background(), name, heavy)
		if err != nil || !acquired {
			t.Fatalf("acquire %s = %v, %v", name, acquired, err)
		}
	}

	acquired := make(chan error, 1)
	go func() {
		_, err := service.acquireMCPServerResource(context.Background(), "browser-c", heavy)
		acquired <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	queued := false
	for time.Now().Before(deadline) {
		status := service.resourceGovernor.status().Resources[0]
		if status.InUseCount == 2 && status.QueuedCount == 1 {
			queued = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !queued {
		t.Fatal("third browser worker did not enter the Browser resource queue")
	}
	service.releaseMCPServerResource("browser-a")
	select {
	case err := <-acquired:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("third browser worker was not admitted after a lease release")
	}
	status := service.resourceGovernor.status().Resources[0]
	if status.InUseCount != 2 || status.QueuedCount != 0 {
		t.Fatalf("browser resource status = %#v", status)
	}
	service.releaseAllCapabilityResources()
	if got := service.resourceGovernor.status().Resources[0].InUseCount; got != 0 {
		t.Fatalf("browser leases after Runtime release = %d", got)
	}
}

func TestRuntimeGenericMCPDoesNotConsumeBrowserWorkerLease(t *testing.T) {
	t.Parallel()
	service := newRuntimeService()
	acquired, err := service.acquireMCPServerResource(context.Background(), "github", config.MCPConfig{Type: config.MCPHttp, URL: "https://mcp.example/api"})
	if err != nil || acquired {
		t.Fatalf("generic MCP browser admission = %v, %v", acquired, err)
	}
	for _, resource := range service.resourceGovernor.status().Resources {
		if resource.Kind == string(runtimeResourceBrowserWorker) && resource.InUseCount != 0 {
			t.Fatalf("generic MCP consumed browser worker lease: %#v", resource)
		}
	}
}

func TestTurnDemandMCPServersExcludeDisabledAndProcessHeavy(t *testing.T) {
	servers := config.MCPs{
		"z-light":  {Type: config.MCPHttp, URL: "https://mcp.example/z"},
		"a-light":  {Type: config.MCPHttp, URL: "https://mcp.example/a"},
		"browser":  {Type: config.MCPStdio, Command: "playwright"},
		"disabled": {Type: config.MCPHttp, URL: "https://mcp.example/off", Disabled: true},
	}
	names := turnDemandMCPServerNames(servers)
	if len(names) != 2 || names[0] != "a-light" || names[1] != "z-light" {
		t.Fatalf("turn-demand MCP servers = %#v", names)
	}
}

func TestProjectCapabilitySetsSharePerProjectAndReleaseAtLastServer(t *testing.T) {
	service := newRuntimeService()
	service.resourceGovernor = newRuntimeResourceGovernor(map[runtimeResourceKind]runtimeResourceLimit{
		runtimeResourceProjectCapability: {Count: 2, Bytes: 32 << 20},
	})
	acquired, err := service.acquireProjectCapabilityResource(context.Background(), "project-a")
	if err != nil || !acquired {
		t.Fatalf("project-a acquire = %v, %v", acquired, err)
	}
	service.registerMCPServerProject("server-a1", "project-a")
	acquired, err = service.acquireProjectCapabilityResource(context.Background(), "project-a")
	if err != nil || acquired {
		t.Fatalf("same project should share lease, acquire = %v, %v", acquired, err)
	}
	service.registerMCPServerProject("server-a2", "project-a")
	acquired, err = service.acquireProjectCapabilityResource(context.Background(), "project-b")
	if err != nil || !acquired {
		t.Fatalf("project-b acquire = %v, %v", acquired, err)
	}
	service.registerMCPServerProject("server-b", "project-b")
	status := service.resourceGovernor.status().Resources[0]
	if status.InUseCount != 2 || status.InUseBytes != 32<<20 {
		t.Fatalf("two warm project sets = %#v", status)
	}
	acquired, err = service.acquireProjectCapabilityResource(context.Background(), "project-c")
	if err != nil || !acquired {
		t.Fatalf("project-c was not admitted through LRU eviction: %v, %v", acquired, err)
	}
	service.registerMCPServerProject("server-c", "project-c")
	service.mu.Lock()
	_, projectARetained := service.capabilityResources["project:project-a"]
	_, serverA1Retained := service.mcpServerProjects["server-a1"]
	service.mu.Unlock()
	if projectARetained || serverA1Retained {
		t.Fatal("oldest idle project capability set was retained after third-project admission")
	}
	if got := service.resourceGovernor.status().Resources[0].InUseCount; got != 2 {
		t.Fatalf("LRU replacement should retain two project sets, count = %d", got)
	}
	service.releaseAllCapabilityResources()
}

func TestRuntimeCapabilityRevisionIsStableAndChangesWithCapabilityConfig(t *testing.T) {
	first := config.NewTestStore(&config.Config{MCP: config.MCPs{"docs": {Type: config.MCPHttp, URL: "https://mcp.example/a"}}})
	second := config.NewTestStore(&config.Config{MCP: config.MCPs{"docs": {Type: config.MCPHttp, URL: "https://mcp.example/b"}}})
	if runtimeCapabilityRevision(first) != runtimeCapabilityRevision(first) {
		t.Fatal("capability revision is not stable")
	}
	if runtimeCapabilityRevision(first) == runtimeCapabilityRevision(second) {
		t.Fatal("capability revision did not change with MCP configuration")
	}
}
