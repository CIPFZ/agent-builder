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

// NewManager constructs a workspace-scoped Manager with discovery results.
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

// States returns the latest discovery state snapshot.
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
	if len(cfg.SkillsPaths) > 0 {
		userPaths := make([]string, 0, len(cfg.SkillsPaths))
		for _, pth := range cfg.SkillsPaths {
			expanded := home.Long(pth)
			if strings.HasPrefix(expanded, "$") && cfg.Resolver != nil {
				if resolved, err := cfg.Resolver(expanded); err == nil {
					expanded = resolved
				}
			}
			userPaths = append(userPaths, expanded)
		}
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
	Resolver       func(string) (string, error)
}
