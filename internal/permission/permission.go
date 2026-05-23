package permission

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/crush/internal/csync"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/google/uuid"
)

// hookApprovalKey is the unexported context key used to mark a tool call as
// pre-approved by a PreToolUse hook. The value is the tool call ID so an
// approval can't be reused across calls that happen to share a context.
type hookApprovalKey struct{}
type turnIDContextKey struct{}

func WithTurnID(ctx context.Context, turnID string) context.Context {
	return context.WithValue(ctx, turnIDContextKey{}, turnID)
}

func turnIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(turnIDContextKey{}).(string)
	return v
}

// WithHookApproval returns a context that marks the given tool call ID as
// pre-approved by a hook. When the permission service sees a matching
// request it short-circuits the normal prompt and grants immediately.
func WithHookApproval(ctx context.Context, toolCallID string) context.Context {
	return context.WithValue(ctx, hookApprovalKey{}, toolCallID)
}

// hookApproved reports whether the context carries a hook approval for the
// given tool call ID.
func hookApproved(ctx context.Context, toolCallID string) bool {
	if toolCallID == "" {
		return false
	}
	v, _ := ctx.Value(hookApprovalKey{}).(string)
	return v == toolCallID
}

type CreatePermissionRequest struct {
	SessionID   string `json:"session_id"`
	TurnID      string `json:"turn_id"`
	ToolCallID  string `json:"tool_call_id"`
	ToolName    string `json:"tool_name"`
	Source      string `json:"source"`
	Description string `json:"description"`
	Action      string `json:"action"`
	Params      any    `json:"params"`
	Path        string `json:"path"`
	Risk        Risk   `json:"risk"`
}

type PermissionNotification struct {
	ToolCallID string `json:"tool_call_id"`
	Granted    bool   `json:"granted"`
	Denied     bool   `json:"denied"`
}

type PolicyApplication struct {
	SessionID   string         `json:"session_id"`
	TurnID      string         `json:"turn_id"`
	ToolCallID  string         `json:"tool_call_id"`
	ToolName    string         `json:"tool_name"`
	Action      string         `json:"action"`
	Path        string         `json:"path"`
	Target      string         `json:"target"`
	Decision    PolicyDecision `json:"decision"`
	Risk        Risk           `json:"risk"`
	Reason      string         `json:"reason"`
	Mode        PolicyMode     `json:"mode"`
	AppliedAt   int64          `json:"applied_at"`
	Description string         `json:"description,omitempty"`
}

type PermissionRequest struct {
	ID           string `json:"id"`
	SessionID    string `json:"session_id"`
	TurnID       string `json:"turn_id"`
	ToolCallID   string `json:"tool_call_id"`
	ToolName     string `json:"tool_name"`
	Description  string `json:"description"`
	Action       string `json:"action"`
	Params       any    `json:"params"`
	Path         string `json:"path"`
	Risk         Risk   `json:"risk"`
	PolicyMode   string `json:"policy_mode,omitempty"`
	PolicyReason string `json:"policy_reason,omitempty"`
	Decision     string `json:"decision,omitempty"`
	Status       string `json:"status"`
	CreatedAt    int64  `json:"created_at"`
	DecidedAt    int64  `json:"decided_at,omitempty"`
}

type Service interface {
	pubsub.Subscriber[PermissionRequest]
	GrantPersistent(permission PermissionRequest)
	Grant(permission PermissionRequest)
	Deny(permission PermissionRequest)
	Request(ctx context.Context, opts CreatePermissionRequest) (bool, error)
	AutoApproveSession(sessionID string)
	SetSkipRequests(skip bool)
	SkipRequests() bool
	SetPolicyMode(mode PolicyMode)
	PolicyMode() PolicyMode
	SubscribePolicyApplications(ctx context.Context) <-chan pubsub.Event[PolicyApplication]
	SubscribeNotifications(ctx context.Context) <-chan pubsub.Event[PermissionNotification]
}

// PermissionKey is a composite key for session permission lookups.
type PermissionKey struct {
	SessionID string
	ToolName  string
	Action    string
	Path      string
}

type permissionService struct {
	*pubsub.Broker[PermissionRequest]

	notificationBroker    *pubsub.Broker[PermissionNotification]
	policyBroker          *pubsub.Broker[PolicyApplication]
	workingDir            string
	sessionPermissions    *csync.Map[PermissionKey, bool]
	pendingRequests       *csync.Map[string, chan bool]
	policy                PermissionPolicy
	policyMode            atomic.Value
	autoApproveSessions   map[string]bool
	autoApproveSessionsMu sync.RWMutex
	skip                  atomic.Bool
	allowedTools          []string

	// used to make sure we only process one request at a time
	requestMu       sync.Mutex
	activeRequest   *PermissionRequest
	activeRequestMu sync.Mutex
}

func (s *permissionService) GrantPersistent(permission PermissionRequest) {
	permission.Status = "allowed_session"
	permission.DecidedAt = time.Now().UnixMilli()
	s.notificationBroker.Publish(pubsub.CreatedEvent, PermissionNotification{
		ToolCallID: permission.ToolCallID,
		Granted:    true,
	})
	respCh, ok := s.pendingRequests.Get(permission.ID)
	if ok {
		respCh <- true
	}

	s.sessionPermissions.Set(PermissionKey{
		SessionID: permission.SessionID,
		ToolName:  permission.ToolName,
		Action:    permission.Action,
		Path:      permission.Path,
	}, true)

	s.activeRequestMu.Lock()
	if s.activeRequest != nil && s.activeRequest.ID == permission.ID {
		s.activeRequest = nil
	}
	s.activeRequestMu.Unlock()
}

func (s *permissionService) Grant(permission PermissionRequest) {
	permission.Status = "allowed_once"
	permission.DecidedAt = time.Now().UnixMilli()
	s.notificationBroker.Publish(pubsub.CreatedEvent, PermissionNotification{
		ToolCallID: permission.ToolCallID,
		Granted:    true,
	})
	respCh, ok := s.pendingRequests.Get(permission.ID)
	if ok {
		respCh <- true
	}

	s.activeRequestMu.Lock()
	if s.activeRequest != nil && s.activeRequest.ID == permission.ID {
		s.activeRequest = nil
	}
	s.activeRequestMu.Unlock()
}

func (s *permissionService) Deny(permission PermissionRequest) {
	permission.Status = "denied"
	permission.DecidedAt = time.Now().UnixMilli()
	s.notificationBroker.Publish(pubsub.CreatedEvent, PermissionNotification{
		ToolCallID: permission.ToolCallID,
		Granted:    false,
		Denied:     true,
	})
	respCh, ok := s.pendingRequests.Get(permission.ID)
	if ok {
		respCh <- false
	}

	s.activeRequestMu.Lock()
	if s.activeRequest != nil && s.activeRequest.ID == permission.ID {
		s.activeRequest = nil
	}
	s.activeRequestMu.Unlock()
}

func (s *permissionService) Request(ctx context.Context, opts CreatePermissionRequest) (bool, error) {
	if opts.TurnID == "" {
		opts.TurnID = turnIDFromContext(ctx)
	}
	risk := opts.Risk
	if risk == "" {
		risk = ClassifyRisk(opts.ToolName, opts.Description)
	}
	policyResult := PolicyResult{
		Decision: PolicyAsk,
		Risk:     risk,
		Reason:   "Ask mode requires approval for tool calls.",
		Mode:     s.PolicyMode(),
	}
	if s.policy != nil {
		policyResult = s.policy.Evaluate(policyToolCall(opts, risk))
		risk = policyResult.Risk
		s.publishPolicyApplication(opts, policyResult)
		switch policyResult.Decision {
		case PolicyAllow:
			s.notificationBroker.Publish(pubsub.CreatedEvent, PermissionNotification{
				ToolCallID: opts.ToolCallID,
				Granted:    true,
			})
			return true, nil
		case PolicyDeny:
			s.notificationBroker.Publish(pubsub.CreatedEvent, PermissionNotification{
				ToolCallID: opts.ToolCallID,
				Denied:     true,
			})
			return false, nil
		}
	}

	if s.skip.Load() {
		return true, nil
	}

	// Check if the tool/action combination is in the allowlist. Deny decisions
	// above still win, so deterministic policy modes cannot be bypassed.
	commandKey := opts.ToolName + ":" + opts.Action
	if slices.Contains(s.allowedTools, commandKey) || slices.Contains(s.allowedTools, opts.ToolName) {
		return true, nil
	}

	// A PreToolUse hook that returned decision=allow stamps the context with the
	// tool call ID. Treat that as an approval only after runtime policy has had
	// the chance to deny the call.
	if hookApproved(ctx, opts.ToolCallID) {
		s.notificationBroker.Publish(pubsub.CreatedEvent, PermissionNotification{
			ToolCallID: opts.ToolCallID,
			Granted:    true,
		})
		return true, nil
	}

	s.requestMu.Lock()
	defer s.requestMu.Unlock()

	// tell the UI that a permission was requested
	s.notificationBroker.Publish(pubsub.CreatedEvent, PermissionNotification{
		ToolCallID: opts.ToolCallID,
	})

	s.autoApproveSessionsMu.RLock()
	autoApprove := s.autoApproveSessions[opts.SessionID]
	s.autoApproveSessionsMu.RUnlock()

	if autoApprove {
		s.notificationBroker.Publish(pubsub.CreatedEvent, PermissionNotification{
			ToolCallID: opts.ToolCallID,
			Granted:    true,
		})
		return true, nil
	}

	fileInfo, err := os.Stat(opts.Path)
	dir := opts.Path
	if err == nil {
		if fileInfo.IsDir() {
			dir = opts.Path
		} else {
			dir = filepath.Dir(opts.Path)
		}
	}

	if dir == "." {
		dir = s.workingDir
	}
	permission := PermissionRequest{
		ID:           uuid.New().String(),
		Path:         dir,
		SessionID:    opts.SessionID,
		TurnID:       opts.TurnID,
		ToolCallID:   opts.ToolCallID,
		ToolName:     opts.ToolName,
		Description:  opts.Description,
		Action:       opts.Action,
		Params:       opts.Params,
		Risk:         risk,
		PolicyMode:   string(s.PolicyMode()),
		PolicyReason: policyResult.Reason,
		Decision:     string(policyResult.Decision),
		Status:       "pending",
		CreatedAt:    time.Now().UnixMilli(),
	}

	if _, ok := s.sessionPermissions.Get(PermissionKey{
		SessionID: permission.SessionID,
		ToolName:  permission.ToolName,
		Action:    permission.Action,
		Path:      permission.Path,
	}); ok {
		s.notificationBroker.Publish(pubsub.CreatedEvent, PermissionNotification{
			ToolCallID: opts.ToolCallID,
			Granted:    true,
		})
		return true, nil
	}

	s.activeRequestMu.Lock()
	s.activeRequest = &permission
	s.activeRequestMu.Unlock()

	respCh := make(chan bool, 1)
	s.pendingRequests.Set(permission.ID, respCh)
	defer s.pendingRequests.Del(permission.ID)

	// Publish the request
	s.Publish(pubsub.CreatedEvent, permission)

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case granted := <-respCh:
		return granted, nil
	}
}

func (s *permissionService) AutoApproveSession(sessionID string) {
	s.autoApproveSessionsMu.Lock()
	s.autoApproveSessions[sessionID] = true
	s.autoApproveSessionsMu.Unlock()
}

func (s *permissionService) SubscribeNotifications(ctx context.Context) <-chan pubsub.Event[PermissionNotification] {
	return s.notificationBroker.Subscribe(ctx)
}

func (s *permissionService) SubscribePolicyApplications(ctx context.Context) <-chan pubsub.Event[PolicyApplication] {
	return s.policyBroker.Subscribe(ctx)
}

func (s *permissionService) SetSkipRequests(skip bool) {
	s.skip.Store(skip)
}

func (s *permissionService) SkipRequests() bool {
	return s.skip.Load()
}

func (s *permissionService) SetPolicyMode(mode PolicyMode) {
	mode = NormalizePolicyMode(mode)
	s.policy = NewPermissionPolicy(mode)
	s.policyMode.Store(mode)
}

func (s *permissionService) PolicyMode() PolicyMode {
	mode, _ := s.policyMode.Load().(PolicyMode)
	return NormalizePolicyMode(mode)
}

func (s *permissionService) publishPolicyApplication(opts CreatePermissionRequest, result PolicyResult) {
	s.policyBroker.Publish(pubsub.CreatedEvent, PolicyApplication{
		SessionID:   opts.SessionID,
		TurnID:      opts.TurnID,
		ToolCallID:  opts.ToolCallID,
		ToolName:    opts.ToolName,
		Action:      opts.Action,
		Path:        opts.Path,
		Target:      opts.Path,
		Decision:    result.Decision,
		Risk:        result.Risk,
		Reason:      result.Reason,
		Mode:        result.Mode,
		AppliedAt:   time.Now().UnixMilli(),
		Description: opts.Description,
	})
}

func NewPermissionService(workingDir string, skip bool, allowedTools []string) Service {
	svc := &permissionService{
		Broker:              pubsub.NewBroker[PermissionRequest](),
		notificationBroker:  pubsub.NewBroker[PermissionNotification](),
		policyBroker:        pubsub.NewBroker[PolicyApplication](),
		workingDir:          workingDir,
		sessionPermissions:  csync.NewMap[PermissionKey, bool](),
		autoApproveSessions: make(map[string]bool),
		allowedTools:        allowedTools,
		pendingRequests:     csync.NewMap[string, chan bool](),
	}
	svc.skip.Store(skip)
	svc.SetPolicyMode(PolicyModeAsk)
	return svc
}
