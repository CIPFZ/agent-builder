package skills

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/charmbracelet/crush/internal/home"
	"github.com/charmbracelet/crush/internal/pubsub"
)

// Manager owns per-workspace skill discovery state and events.
type Manager struct {
	mu           sync.RWMutex
	allSkills    []*Skill
	activeSkills []*Skill
	states       []*SkillState

	// resolvedPaths are the expanded SkillsPaths used during discovery.
	// Stored so Catalog/ReadContent can label skills without
	// re-resolving.
	resolvedPaths []string
	workingDir    string

	broker       *pubsub.Broker[Event]
	globalMirror bool
}

// ManagerOption configures a Manager at construction time.
type ManagerOption func(*Manager)

// WithGlobalMirror mirrors manager state to the legacy package-level cache.
func WithGlobalMirror() ManagerOption {
	return func(m *Manager) {
		m.globalMirror = true
	}
}

// WithResolvedPaths stores the expanded skills directory paths that
// were used during discovery. Catalog and ReadContent use these for
// source labelling.
func WithResolvedPaths(paths []string) ManagerOption {
	return func(m *Manager) {
		m.resolvedPaths = paths
	}
}

// WithWorkingDir stores the workspace working directory. Catalog and
// ReadContent use it to distinguish project skills from user skills.
func WithWorkingDir(dir string) ManagerOption {
	return func(m *Manager) {
		m.workingDir = dir
	}
}

// NewManager constructs a workspace-scoped Manager with the given
// pre-computed discovery results. The slices are stored as-is; callers
// should not mutate them afterwards.
func NewManager(allSkills, activeSkills []*Skill, states []*SkillState, opts ...ManagerOption) *Manager {
	m := &Manager{
		allSkills:    allSkills,
		activeSkills: activeSkills,
		states:       cloneStates(states),
		broker:       pubsub.NewBroker[Event](),
	}
	for _, opt := range opts {
		opt(m)
	}
	if m.globalMirror {
		SetLatestStates(states)
	}
	return m
}

// AllSkills returns the deduplicated list of discovered skills.
func (m *Manager) AllSkills() []*Skill {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.allSkills
}

// ActiveSkills returns the active post-filter skill list.
func (m *Manager) ActiveSkills() []*Skill {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeSkills
}

// ResolvedPaths returns the expanded skills directory paths stored at
// construction time.
func (m *Manager) ResolvedPaths() []string {
	return m.resolvedPaths
}

// WorkingDir returns the workspace working directory stored at
// construction time.
func (m *Manager) WorkingDir() string {
	return m.workingDir
}

// States returns a clone of the latest discovery state snapshot.
func (m *Manager) States() []*SkillState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneStates(m.states)
}

// SetLatestStates updates the cached discovery snapshot.
func (m *Manager) SetLatestStates(states []*SkillState) {
	m.mu.Lock()
	m.states = cloneStates(states)
	m.mu.Unlock()
	if m.globalMirror {
		SetLatestStates(states)
	}
}

// SetDiscoverySnapshot replaces the runtime-owned skill inventory.
func (m *Manager) SetDiscoverySnapshot(allSkills, activeSkills []*Skill, states []*SkillState) {
	m.mu.Lock()
	m.allSkills = allSkills
	m.activeSkills = activeSkills
	m.states = cloneStates(states)
	m.mu.Unlock()
	if m.globalMirror {
		SetLatestStates(states)
	}
}

// PublishStates updates the snapshot and publishes a discovery event.
func (m *Manager) PublishStates(states []*SkillState) {
	m.SetLatestStates(states)
	m.broker.Publish(pubsub.UpdatedEvent, Event{States: cloneStates(states)})
	if m.globalMirror {
		PublishStates(states)
	}
}

// SubscribeEvents returns discovery events for this manager's workspace.
func (m *Manager) SubscribeEvents(ctx context.Context) <-chan pubsub.Event[Event] {
	return m.broker.Subscribe(ctx)
}

// Shutdown releases broker resources.
func (m *Manager) Shutdown() {
	if m.broker != nil {
		m.broker.Shutdown()
	}
}

// DiscoverFromConfig discovers builtin and configured user skills.
func DiscoverFromConfig(cfg DiscoveryConfig) (allSkills, activeSkills []*Skill, states []*SkillState) {
	builtin, builtinStates := DiscoverBuiltinWithStates()
	discovered := append([]*Skill(nil), builtin...)

	var userStates []*SkillState
	userPaths := cfg.ResolvePaths()
	if len(userPaths) > 0 {
		var userSkills []*Skill
		userSkills, userStates = DiscoverWithStates(userPaths)
		discovered = append(discovered, userSkills...)
	}

	allSkills = Deduplicate(discovered)
	activeSkills = Filter(allSkills, cfg.DisabledSkills)

	allStates := append([]*SkillState(nil), builtinStates...)
	allStates = append(allStates, userStates...)
	allStates = DeduplicateStates(allStates)
	slices.SortStableFunc(allStates, func(a, b *SkillState) int {
		return strings.Compare(strings.ToLower(a.Path), strings.ToLower(b.Path))
	})
	return allSkills, activeSkills, allStates
}

// DiscoveryConfig contains the inputs DiscoverFromConfig needs.
type DiscoveryConfig struct {
	SkillsPaths    []string
	DisabledSkills []string
	WorkingDir     string
	// Resolver expands $VAR-style references in paths. May be nil.
	Resolver func(string) (string, error)
}

// ResolvePaths expands home-directory and $VAR references in
// SkillsPaths. This is the canonical path-resolution logic used by
// DiscoverFromConfig; callers that need the resolved list (e.g. for
// Catalog labels) can call this directly.
func (c DiscoveryConfig) ResolvePaths() []string {
	if len(c.SkillsPaths) == 0 {
		return nil
	}
	out := make([]string, 0, len(c.SkillsPaths))
	for _, pth := range c.SkillsPaths {
		expanded := home.Long(pth)
		if strings.HasPrefix(expanded, "$") && c.Resolver != nil {
			if resolved, err := c.Resolver(expanded); err == nil {
				expanded = resolved
			}
		}
		out = append(out, expanded)
	}
	return out
}
