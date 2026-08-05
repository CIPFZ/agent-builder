package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	mcptools "github.com/CIPFZ/agent-builder/internal/agent/tools/mcp"
	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

const defaultRuntimeMCPIdleTTL = 5 * time.Minute

func (r *runtimeService) touchMCPServerConsumer(cfg *config.ConfigStore, name string) {
	if state, ok := mcptools.GetState(strings.TrimSpace(name)); ok && state.State == mcptools.StateConnected {
		r.scheduleMCPServerIdleEviction(cfg, name)
	}
}

func (r *runtimeService) scheduleMCPServerIdleEviction(cfg *config.ConfigStore, name string) {
	name = strings.TrimSpace(name)
	if cfg == nil || name == "" {
		return
	}
	r.mu.Lock()
	if projectID := r.mcpServerProjects[name]; projectID != "" {
		r.projectCapabilityUsed[projectID] = time.Now().UnixMilli()
	}
	if previous := r.mcpIdleTimers[name]; previous != nil {
		previous.Stop()
	}
	ttl := r.mcpIdleTTL
	if ttl <= 0 {
		ttl = defaultRuntimeMCPIdleTTL
	}
	var timer *time.Timer
	timer = time.AfterFunc(ttl, func() { r.evictIdleMCPServer(cfg, name, timer) })
	r.mcpIdleTimers[name] = timer
	r.mu.Unlock()
}

func (r *runtimeService) evictIdleMCPServer(cfg *config.ConfigStore, name string, timer *time.Timer) {
	r.mu.Lock()
	if r.mcpIdleTimers[name] != timer {
		r.mu.Unlock()
		return
	}
	delete(r.mcpIdleTimers, name)
	hasActiveTurns := len(r.sessionTurns) > 0
	r.mu.Unlock()
	if hasActiveTurns {
		r.scheduleMCPServerIdleEviction(cfg, name)
		return
	}

	_ = mcptools.UnloadSingle(cfg, name)
	r.releaseMCPServerResource(name)
	r.unregisterMCPServerProject(name)
	r.markMCPServerCapabilitiesUnloaded(name, "idle_ttl")
	r.publishMCPServerEvent(runtimeapi.EventMCPServerUnloaded, name, mcpServerStateUnloaded, "", "idle_ttl")
	r.publishMCPUpdatedEvents(name)
}

func (r *runtimeService) currentCapabilityProjectID(workspaceID string, cfg *config.ConfigStore) string {
	r.mu.Lock()
	projectID := r.activeProjectID
	r.mu.Unlock()
	projectID = firstNonEmpty(strings.TrimSpace(projectID), strings.TrimSpace(workspaceID), "default")
	return projectID + "@" + runtimeCapabilityRevision(cfg)
}

func runtimeCapabilityRevision(store *config.ConfigStore) string {
	if store == nil || store.Config() == nil {
		return "none"
	}
	cfg := store.Config()
	payload := struct {
		MCP            config.MCPs
		LSP            config.LSPs
		SkillsPaths    []string
		DisabledSkills []string
	}{MCP: cfg.MCP, LSP: cfg.LSP}
	if cfg.Options != nil {
		payload.SkillsPaths = cfg.Options.SkillsPaths
		payload.DisabledSkills = cfg.Options.DisabledSkills
	}
	encoded, _ := json.Marshal(payload)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:8])
}

func (r *runtimeService) acquireProjectCapabilityResource(ctx context.Context, projectID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	projectID = firstNonEmpty(strings.TrimSpace(projectID), "default")
	key := "project:" + projectID
	r.mu.Lock()
	_, exists := r.capabilityResources[key]
	r.mu.Unlock()
	if exists {
		return false, nil
	}
	release, ok := r.resourceGovernor.tryAcquire(runtimeResourceProjectCapability, 1, runtimeProjectSetBytes)
	if !ok && r.evictLRUProjectCapabilitySet(projectID) {
		release, ok = r.resourceGovernor.tryAcquire(runtimeResourceProjectCapability, 1, runtimeProjectSetBytes)
	}
	var err error
	if !ok {
		release, err = r.resourceGovernor.AcquireProjectCapability(ctx, runtimeProjectSetBytes)
	}
	if err != nil {
		return false, err
	}
	r.mu.Lock()
	if _, exists := r.capabilityResources[key]; exists {
		r.mu.Unlock()
		release()
		return false, nil
	}
	r.capabilityResources[key] = release
	r.projectCapabilityUsed[projectID] = time.Now().UnixMilli()
	r.mu.Unlock()
	return true, nil
}

func (r *runtimeService) registerMCPServerProject(name, projectID string) {
	r.registerMCPServerProjectWithConfig(name, projectID, nil)
}

func (r *runtimeService) registerMCPServerProjectWithConfig(name, projectID string, cfg *config.ConfigStore) {
	name = strings.TrimSpace(name)
	projectID = firstNonEmpty(strings.TrimSpace(projectID), "default")
	if name == "" {
		return
	}
	r.mu.Lock()
	previous := r.mcpServerProjects[name]
	if previous != "" && previous != projectID {
		delete(r.projectMCPServers[previous], name)
		if len(r.projectMCPServers[previous]) == 0 {
			delete(r.projectMCPServers, previous)
		}
	}
	r.mcpServerProjects[name] = projectID
	if cfg != nil {
		r.mcpServerConfigs[name] = cfg
	}
	servers := r.projectMCPServers[projectID]
	if servers == nil {
		servers = make(map[string]struct{})
		r.projectMCPServers[projectID] = servers
	}
	servers[name] = struct{}{}
	r.projectCapabilityUsed[projectID] = time.Now().UnixMilli()
	r.mu.Unlock()
	if previous != "" && previous != projectID {
		r.releaseProjectCapabilityIfUnused(previous)
	}
}

func (r *runtimeService) unregisterMCPServerProject(name string) {
	name = strings.TrimSpace(name)
	r.mu.Lock()
	projectID := r.mcpServerProjects[name]
	delete(r.mcpServerProjects, name)
	delete(r.mcpServerConfigs, name)
	if servers := r.projectMCPServers[projectID]; servers != nil {
		delete(servers, name)
		if len(servers) == 0 {
			delete(r.projectMCPServers, projectID)
			delete(r.projectCapabilityUsed, projectID)
		}
	}
	r.mu.Unlock()
	r.releaseProjectCapabilityIfUnused(projectID)
}

func (r *runtimeService) evictLRUProjectCapabilitySet(excludeProjectID string) bool {
	type idleServer struct {
		name string
		cfg  *config.ConfigStore
	}
	r.mu.Lock()
	currentProjectID := strings.TrimSpace(r.activeProjectID)
	candidate := ""
	oldest := int64(0)
	for projectID, servers := range r.projectMCPServers {
		if projectID == excludeProjectID || (currentProjectID != "" && strings.HasPrefix(projectID, currentProjectID+"@")) {
			continue
		}
		busy := false
		for name := range servers {
			if strings.HasPrefix(name, "@lsp:") {
				busy = true
				break
			}
		}
		if !busy {
			for _, status := range r.activeSessionStatuses {
				if status.Status != "attention" && status.ProjectID != "" && strings.HasPrefix(projectID, status.ProjectID+"@") {
					busy = true
					break
				}
			}
		}
		used := r.projectCapabilityUsed[projectID]
		if !busy && (candidate == "" || used < oldest || (used == oldest && projectID < candidate)) {
			candidate, oldest = projectID, used
		}
	}
	if candidate == "" {
		r.mu.Unlock()
		return false
	}
	var servers []idleServer
	var timers []*time.Timer
	var releases []func()
	for name := range r.projectMCPServers[candidate] {
		servers = append(servers, idleServer{name: name, cfg: r.mcpServerConfigs[name]})
		delete(r.mcpServerProjects, name)
		delete(r.mcpServerConfigs, name)
		if timer := r.mcpIdleTimers[name]; timer != nil {
			timers = append(timers, timer)
		}
		delete(r.mcpIdleTimers, name)
		if release := r.capabilityResources["mcp:"+name]; release != nil {
			releases = append(releases, release)
		}
		delete(r.capabilityResources, "mcp:"+name)
	}
	delete(r.projectMCPServers, candidate)
	delete(r.projectCapabilityUsed, candidate)
	if release := r.capabilityResources["project:"+candidate]; release != nil {
		releases = append(releases, release)
	}
	delete(r.capabilityResources, "project:"+candidate)
	r.mu.Unlock()
	for _, timer := range timers {
		timer.Stop()
	}
	for _, server := range servers {
		if server.cfg != nil {
			_ = mcptools.UnloadSingle(server.cfg, server.name)
			r.markMCPServerCapabilitiesUnloaded(server.name, "project_lru")
			r.publishMCPServerEvent(runtimeapi.EventMCPServerUnloaded, server.name, mcpServerStateUnloaded, "", "project_lru")
			r.publishMCPUpdatedEvents(server.name)
		}
	}
	for _, release := range releases {
		release()
	}
	return true
}

func (r *runtimeService) releaseProjectCapabilityIfUnused(projectID string) {
	if projectID == "" {
		return
	}
	key := "project:" + projectID
	r.mu.Lock()
	if len(r.projectMCPServers[projectID]) > 0 {
		r.mu.Unlock()
		return
	}
	release := r.capabilityResources[key]
	delete(r.capabilityResources, key)
	delete(r.projectCapabilityUsed, projectID)
	r.mu.Unlock()
	if release != nil {
		release()
	}
}

func (r *runtimeService) cancelMCPIdleTimer(name string) {
	name = strings.TrimSpace(name)
	r.mu.Lock()
	timer := r.mcpIdleTimers[name]
	delete(r.mcpIdleTimers, name)
	r.mu.Unlock()
	if timer != nil {
		timer.Stop()
	}
}

func (r *runtimeService) cancelAllMCPIdleTimers() {
	r.mu.Lock()
	timers := make([]*time.Timer, 0, len(r.mcpIdleTimers))
	for name, timer := range r.mcpIdleTimers {
		delete(r.mcpIdleTimers, name)
		if timer != nil {
			timers = append(timers, timer)
		}
	}
	r.mu.Unlock()
	for _, timer := range timers {
		timer.Stop()
	}
}

func (r *runtimeService) markMCPServerCapabilitiesUnloaded(server, reason string) {
	r.mu.Lock()
	started := r.runtime != nil && r.workspace != nil
	r.mu.Unlock()
	if !started {
		return
	}
	for _, capability := range r.mcpCapabilitiesForServer(server) {
		if !capability.Enabled {
			continue
		}
		capability.State = capabilityStateUnloaded
		capability.Reason = reason
		r.setCapabilityLoadRecord(capability.ID, runtimeCapabilityLoadRecord{
			State: capabilityStateUnloaded, Reason: reason, UpdatedAt: time.Now().UnixMilli(),
		})
		r.recordCapabilityLoad(capability, capabilityStateUnloaded, reason, "", 0)
	}
}
