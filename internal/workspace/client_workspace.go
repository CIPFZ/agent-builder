package workspace

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/CIPFZ/agent-builder/internal/agent/notify"
	"github.com/CIPFZ/agent-builder/internal/agent/tools/mcp"
	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/client"
	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/history"
	"github.com/CIPFZ/agent-builder/internal/lsp"
	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/CIPFZ/agent-builder/internal/oauth"
	"github.com/CIPFZ/agent-builder/internal/permission"
	"github.com/CIPFZ/agent-builder/internal/pubsub"
	"github.com/CIPFZ/agent-builder/internal/session"
	"github.com/CIPFZ/agent-builder/internal/skills"
	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
)

// ClientWorkspace implements the Workspace interface by delegating all
// operations to a remote server via the client SDK. It caches the
// apitypes.Workspace returned at creation time and refreshes it after
// config-mutating operations.
type ClientWorkspace struct {
	client *client.Client

	mu sync.RWMutex
	ws apitypes.Workspace
}

// NewClientWorkspace creates a new ClientWorkspace that proxies all
// operations through the given client SDK. The ws parameter is the
// apitypes.Workspace snapshot returned by the server at creation time.
func NewClientWorkspace(c *client.Client, ws apitypes.Workspace) *ClientWorkspace {
	if ws.Config != nil {
		ws.Config.SetupAgents()
	}
	return &ClientWorkspace{
		client: c,
		ws:     ws,
	}
}

// refreshWorkspace re-fetches the workspace from the server, updating
// the cached snapshot. Called after config-mutating operations.
func (w *ClientWorkspace) refreshWorkspace() {
	updated, err := w.client.GetWorkspace(context.Background(), w.ws.ID)
	if err != nil {
		slog.Error("Failed to refresh workspace", "error", err)
		return
	}
	if updated.Config != nil {
		updated.Config.SetupAgents()
	}
	w.mu.Lock()
	w.ws = *updated
	w.mu.Unlock()
}

// cached returns a snapshot of the cached workspace.
func (w *ClientWorkspace) cached() apitypes.Workspace {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.ws
}

// workspaceID returns the cached workspace ID.
func (w *ClientWorkspace) workspaceID() string {
	return w.cached().ID
}

// -- Sessions --

func (w *ClientWorkspace) CreateSession(ctx context.Context, title string) (session.Session, error) {
	sess, err := w.client.CreateSession(ctx, w.workspaceID(), title)
	if err != nil {
		return session.Session{}, err
	}
	return apiTypeToSession(*sess), nil
}

func (w *ClientWorkspace) GetSession(ctx context.Context, sessionID string) (session.Session, error) {
	sess, err := w.client.GetSession(ctx, w.workspaceID(), sessionID)
	if err != nil {
		return session.Session{}, err
	}
	return apiTypeToSession(*sess), nil
}

func (w *ClientWorkspace) ListSessions(ctx context.Context) ([]session.Session, error) {
	protoSessions, err := w.client.ListSessions(ctx, w.workspaceID())
	if err != nil {
		return nil, err
	}
	sessions := make([]session.Session, len(protoSessions))
	for i, s := range protoSessions {
		sessions[i] = apiTypeToSession(s)
	}
	return sessions, nil
}

func (w *ClientWorkspace) SaveSession(ctx context.Context, sess session.Session) (session.Session, error) {
	saved, err := w.client.SaveSession(ctx, w.workspaceID(), sessionToAPIType(sess))
	if err != nil {
		return session.Session{}, err
	}
	return apiTypeToSession(*saved), nil
}

func (w *ClientWorkspace) DeleteSession(ctx context.Context, sessionID string) error {
	return w.client.DeleteSession(ctx, w.workspaceID(), sessionID)
}

func (w *ClientWorkspace) CreateAgentToolSessionID(messageID, toolCallID string) string {
	return fmt.Sprintf("%s$$%s", messageID, toolCallID)
}

func (w *ClientWorkspace) ParseAgentToolSessionID(sessionID string) (string, string, bool) {
	parts := strings.Split(sessionID, "$$")
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// -- Messages --

func (w *ClientWorkspace) ListMessages(ctx context.Context, sessionID string) ([]message.Message, error) {
	msgs, err := w.client.ListMessages(ctx, w.workspaceID(), sessionID)
	if err != nil {
		return nil, err
	}
	return apiTypeToMessages(msgs), nil
}

func (w *ClientWorkspace) ListUserMessages(ctx context.Context, sessionID string) ([]message.Message, error) {
	msgs, err := w.client.ListUserMessages(ctx, w.workspaceID(), sessionID)
	if err != nil {
		return nil, err
	}
	return apiTypeToMessages(msgs), nil
}

func (w *ClientWorkspace) ListAllUserMessages(ctx context.Context) ([]message.Message, error) {
	msgs, err := w.client.ListAllUserMessages(ctx, w.workspaceID())
	if err != nil {
		return nil, err
	}
	return apiTypeToMessages(msgs), nil
}

// -- Agent --

func (w *ClientWorkspace) AgentRun(ctx context.Context, sessionID, prompt string, attachments ...message.Attachment) error {
	return w.client.SendMessage(ctx, w.workspaceID(), sessionID, prompt, attachments...)
}

func (w *ClientWorkspace) AgentCancel(sessionID string) {
	_ = w.client.CancelAgentSession(context.Background(), w.workspaceID(), sessionID)
}

func (w *ClientWorkspace) AgentIsBusy() bool {
	info, err := w.client.GetAgentInfo(context.Background(), w.workspaceID())
	if err != nil {
		return false
	}
	return info.IsBusy
}

func (w *ClientWorkspace) AgentIsSessionBusy(sessionID string) bool {
	info, err := w.client.GetAgentSessionInfo(context.Background(), w.workspaceID(), sessionID)
	if err != nil {
		return false
	}
	return info.IsBusy
}

func (w *ClientWorkspace) AgentModel() AgentModel {
	info, err := w.client.GetAgentInfo(context.Background(), w.workspaceID())
	if err != nil {
		return AgentModel{}
	}
	return AgentModel{
		CatwalkCfg: info.Model,
		ModelCfg:   info.ModelCfg,
	}
}

func (w *ClientWorkspace) AgentIsReady() bool {
	info, err := w.client.GetAgentInfo(context.Background(), w.workspaceID())
	if err != nil {
		return false
	}
	return info.IsReady
}

func (w *ClientWorkspace) AgentQueuedPrompts(sessionID string) int {
	count, err := w.client.GetAgentSessionQueuedPrompts(context.Background(), w.workspaceID(), sessionID)
	if err != nil {
		return 0
	}
	return count
}

func (w *ClientWorkspace) AgentQueuedPromptsList(sessionID string) []string {
	prompts, err := w.client.GetAgentSessionQueuedPromptsList(context.Background(), w.workspaceID(), sessionID)
	if err != nil {
		return nil
	}
	return prompts
}

func (w *ClientWorkspace) AgentClearQueue(sessionID string) {
	_ = w.client.ClearAgentSessionQueuedPrompts(context.Background(), w.workspaceID(), sessionID)
}

func (w *ClientWorkspace) AgentSummarize(ctx context.Context, sessionID string) error {
	return w.client.AgentSummarizeSession(ctx, w.workspaceID(), sessionID)
}

func (w *ClientWorkspace) UpdateAgentModel(ctx context.Context) error {
	return w.client.UpdateAgent(ctx, w.workspaceID())
}

func (w *ClientWorkspace) InitCoderAgent(ctx context.Context) error {
	return w.client.InitiateAgentProcessing(ctx, w.workspaceID())
}

func (w *ClientWorkspace) GetDefaultSmallModel(providerID string) config.SelectedModel {
	model, err := w.client.GetDefaultSmallModel(context.Background(), w.workspaceID(), providerID)
	if err != nil {
		return config.SelectedModel{}
	}
	return *model
}

// -- Permissions --

func (w *ClientWorkspace) PermissionGrant(perm permission.PermissionRequest) {
	_ = w.client.GrantPermission(context.Background(), w.workspaceID(), apitypes.PermissionGrant{
		Permission: apitypes.PermissionRequest{
			ID:          perm.ID,
			SessionID:   perm.SessionID,
			ToolCallID:  perm.ToolCallID,
			ToolName:    perm.ToolName,
			Description: perm.Description,
			Action:      perm.Action,
			Path:        perm.Path,
			Params:      perm.Params,
		},
		Action: apitypes.PermissionAllowForSession,
	})
}

func (w *ClientWorkspace) PermissionGrantPersistent(perm permission.PermissionRequest) {
	_ = w.client.GrantPermission(context.Background(), w.workspaceID(), apitypes.PermissionGrant{
		Permission: apitypes.PermissionRequest{
			ID:          perm.ID,
			SessionID:   perm.SessionID,
			ToolCallID:  perm.ToolCallID,
			ToolName:    perm.ToolName,
			Description: perm.Description,
			Action:      perm.Action,
			Path:        perm.Path,
			Params:      perm.Params,
		},
		Action: apitypes.PermissionAllow,
	})
}

func (w *ClientWorkspace) PermissionDeny(perm permission.PermissionRequest) {
	_ = w.client.GrantPermission(context.Background(), w.workspaceID(), apitypes.PermissionGrant{
		Permission: apitypes.PermissionRequest{
			ID:          perm.ID,
			SessionID:   perm.SessionID,
			ToolCallID:  perm.ToolCallID,
			ToolName:    perm.ToolName,
			Description: perm.Description,
			Action:      perm.Action,
			Path:        perm.Path,
			Params:      perm.Params,
		},
		Action: apitypes.PermissionDeny,
	})
}

func (w *ClientWorkspace) PermissionSkipRequests() bool {
	skip, err := w.client.GetPermissionsSkipRequests(context.Background(), w.workspaceID())
	if err != nil {
		return false
	}
	return skip
}

func (w *ClientWorkspace) PermissionSetSkipRequests(skip bool) {
	_ = w.client.SetPermissionsSkipRequests(context.Background(), w.workspaceID(), skip)
}

// -- FileTracker --

func (w *ClientWorkspace) FileTrackerRecordRead(ctx context.Context, sessionID, path string) {
	_ = w.client.FileTrackerRecordRead(ctx, w.workspaceID(), sessionID, path)
}

func (w *ClientWorkspace) FileTrackerLastReadTime(ctx context.Context, sessionID, path string) time.Time {
	t, err := w.client.FileTrackerLastReadTime(ctx, w.workspaceID(), sessionID, path)
	if err != nil {
		return time.Time{}
	}
	return t
}

func (w *ClientWorkspace) FileTrackerListReadFiles(ctx context.Context, sessionID string) ([]string, error) {
	return w.client.FileTrackerListReadFiles(ctx, w.workspaceID(), sessionID)
}

// -- History --

func (w *ClientWorkspace) ListSessionHistory(ctx context.Context, sessionID string) ([]history.File, error) {
	files, err := w.client.ListSessionHistoryFiles(ctx, w.workspaceID(), sessionID)
	if err != nil {
		return nil, err
	}
	return apiTypeToFiles(files), nil
}

// -- LSP --

func (w *ClientWorkspace) LSPStart(ctx context.Context, path string) {
	_ = w.client.LSPStart(ctx, w.workspaceID(), path)
}

func (w *ClientWorkspace) LSPStopAll(ctx context.Context) {
	_ = w.client.LSPStopAll(ctx, w.workspaceID())
}

func (w *ClientWorkspace) LSPGetStates() map[string]LSPClientInfo {
	states, err := w.client.GetLSPs(context.Background(), w.workspaceID())
	if err != nil {
		return nil
	}
	result := make(map[string]LSPClientInfo, len(states))
	for k, v := range states {
		result[k] = LSPClientInfo{
			Name:            v.Name,
			State:           v.State,
			Error:           v.Error,
			DiagnosticCount: v.DiagnosticCount,
			ConnectedAt:     v.ConnectedAt,
		}
	}
	return result
}

func (w *ClientWorkspace) LSPGetDiagnosticCounts(name string) lsp.DiagnosticCounts {
	diags, err := w.client.GetLSPDiagnostics(context.Background(), w.workspaceID(), name)
	if err != nil {
		return lsp.DiagnosticCounts{}
	}
	var counts lsp.DiagnosticCounts
	for _, fileDiags := range diags {
		for _, d := range fileDiags {
			switch d.Severity {
			case protocol.SeverityError:
				counts.Error++
			case protocol.SeverityWarning:
				counts.Warning++
			case protocol.SeverityInformation:
				counts.Information++
			case protocol.SeverityHint:
				counts.Hint++
			}
		}
	}
	return counts
}

// -- Config (read-only) --

func (w *ClientWorkspace) Config() *config.Config {
	return w.cached().Config
}

func (w *ClientWorkspace) WorkingDir() string {
	return w.cached().Path
}

func (w *ClientWorkspace) Resolver() config.VariableResolver {
	return config.IdentityResolver()
}

// -- Config mutations --

func (w *ClientWorkspace) UpdatePreferredModel(scope config.Scope, modelType config.SelectedModelType, model config.SelectedModel) error {
	err := w.client.UpdatePreferredModel(context.Background(), w.workspaceID(), scope, modelType, model)
	if err == nil {
		w.refreshWorkspace()
	}
	return err
}

func (w *ClientWorkspace) SetProviderAPIKey(scope config.Scope, providerID string, apiKey any) error {
	err := w.client.SetProviderAPIKey(context.Background(), w.workspaceID(), scope, providerID, apiKey)
	if err == nil {
		w.refreshWorkspace()
	}
	return err
}

func (w *ClientWorkspace) SetConfigField(scope config.Scope, key string, value any) error {
	err := w.client.SetConfigField(context.Background(), w.workspaceID(), scope, key, value)
	if err == nil {
		w.refreshWorkspace()
	}
	return err
}

func (w *ClientWorkspace) RemoveConfigField(scope config.Scope, key string) error {
	err := w.client.RemoveConfigField(context.Background(), w.workspaceID(), scope, key)
	if err == nil {
		w.refreshWorkspace()
	}
	return err
}

func (w *ClientWorkspace) ImportCopilot() (*oauth.Token, bool) {
	token, ok, err := w.client.ImportCopilot(context.Background(), w.workspaceID())
	if err != nil {
		return nil, false
	}
	if ok {
		w.refreshWorkspace()
	}
	return token, ok
}

func (w *ClientWorkspace) RefreshOAuthToken(ctx context.Context, scope config.Scope, providerID string) error {
	err := w.client.RefreshOAuthToken(ctx, w.workspaceID(), scope, providerID)
	if err == nil {
		w.refreshWorkspace()
	}
	return err
}

// -- Project lifecycle --

func (w *ClientWorkspace) ProjectNeedsInitialization() (bool, error) {
	return w.client.ProjectNeedsInitialization(context.Background(), w.workspaceID())
}

func (w *ClientWorkspace) MarkProjectInitialized() error {
	return w.client.MarkProjectInitialized(context.Background(), w.workspaceID())
}

func (w *ClientWorkspace) InitializePrompt() (string, error) {
	return w.client.GetInitializePrompt(context.Background(), w.workspaceID())
}

func (w *ClientWorkspace) ListSkills(ctx context.Context) ([]skills.CatalogEntry, error) {
	entries, err := w.client.ListSkills(ctx, w.workspaceID())
	if err != nil {
		return nil, err
	}
	result := make([]skills.CatalogEntry, len(entries))
	for i, entry := range entries {
		result[i] = skills.CatalogEntry{
			ID:          entry.ID,
			Name:        entry.Name,
			Description: entry.Description,
			Label:       entry.Label,
			Source:      skills.SourceType(entry.Source),
		}
	}
	return result, nil
}

func (w *ClientWorkspace) ReadSkill(ctx context.Context, skillID string) ([]byte, skills.SkillReadResult, error) {
	resp, err := w.client.ReadSkill(ctx, w.workspaceID(), skillID)
	if err != nil {
		return nil, skills.SkillReadResult{}, err
	}
	return resp.Content, skills.SkillReadResult{
		Name:        resp.Result.Name,
		Description: resp.Result.Description,
		Source:      skills.SourceType(resp.Result.Source),
		Builtin:     resp.Result.Builtin,
	}, nil
}

// -- MCP operations --

func (w *ClientWorkspace) MCPGetStates() map[string]mcp.ClientInfo {
	states, err := w.client.MCPGetStates(context.Background(), w.workspaceID())
	if err != nil {
		return nil
	}
	result := make(map[string]mcp.ClientInfo, len(states))
	for k, v := range states {
		result[k] = mcp.ClientInfo{
			Name:  v.Name,
			State: mcp.State(v.State),
			Error: v.Error,
			Counts: mcp.Counts{
				Tools:     v.ToolCount,
				Prompts:   v.PromptCount,
				Resources: v.ResourceCount,
			},
			ConnectedAt: v.ConnectedAt,
		}
	}
	return result
}

func (w *ClientWorkspace) MCPRefreshPrompts(ctx context.Context, name string) {
	_ = w.client.MCPRefreshPrompts(ctx, w.workspaceID(), name)
}

func (w *ClientWorkspace) MCPRefreshResources(ctx context.Context, name string) {
	_ = w.client.MCPRefreshResources(ctx, w.workspaceID(), name)
}

func (w *ClientWorkspace) RefreshMCPTools(ctx context.Context, name string) {
	_ = w.client.RefreshMCPTools(ctx, w.workspaceID(), name)
}

func (w *ClientWorkspace) ReadMCPResource(ctx context.Context, name, uri string) ([]MCPResourceContents, error) {
	contents, err := w.client.ReadMCPResource(ctx, w.workspaceID(), name, uri)
	if err != nil {
		return nil, err
	}
	result := make([]MCPResourceContents, len(contents))
	for i, c := range contents {
		result[i] = MCPResourceContents{
			URI:      c.URI,
			MIMEType: c.MIMEType,
			Text:     c.Text,
			Blob:     c.Blob,
		}
	}
	return result, nil
}

func (w *ClientWorkspace) GetMCPPrompt(clientID, promptID string, args map[string]string) (string, error) {
	return w.client.GetMCPPrompt(context.Background(), w.workspaceID(), clientID, promptID, args)
}

func (w *ClientWorkspace) EnableDockerMCP(ctx context.Context) error {
	return w.client.EnableDockerMCP(ctx, w.workspaceID())
}

func (w *ClientWorkspace) DisableDockerMCP() error {
	return w.client.DisableDockerMCP(context.Background(), w.workspaceID())
}

func (w *ClientWorkspace) Shutdown() {
	_ = w.client.DeleteWorkspace(context.Background(), w.workspaceID())
}

// translateEvent converts API-typed SSE events into workspace domain events.
func translateEvent(ev any) any {
	switch e := ev.(type) {
	case pubsub.Event[apitypes.LSPEvent]:
		return pubsub.Event[LSPEvent]{
			Type: e.Type,
			Payload: LSPEvent{
				Type:            LSPEventType(e.Payload.Type),
				Name:            e.Payload.Name,
				State:           e.Payload.State,
				Error:           e.Payload.Error,
				DiagnosticCount: e.Payload.DiagnosticCount,
			},
		}
	case pubsub.Event[apitypes.MCPEvent]:
		return pubsub.Event[mcp.Event]{
			Type: e.Type,
			Payload: mcp.Event{
				Type:  apiTypeToMCPEventType(e.Payload.Type),
				Name:  e.Payload.Name,
				State: mcp.State(e.Payload.State),
				Error: e.Payload.Error,
				Counts: mcp.Counts{
					Tools:     e.Payload.ToolCount,
					Prompts:   e.Payload.PromptCount,
					Resources: e.Payload.ResourceCount,
				},
			},
		}
	case pubsub.Event[apitypes.PermissionRequest]:
		return pubsub.Event[permission.PermissionRequest]{
			Type: e.Type,
			Payload: permission.PermissionRequest{
				ID:          e.Payload.ID,
				SessionID:   e.Payload.SessionID,
				ToolCallID:  e.Payload.ToolCallID,
				ToolName:    e.Payload.ToolName,
				Description: e.Payload.Description,
				Action:      e.Payload.Action,
				Path:        e.Payload.Path,
				Params:      e.Payload.Params,
			},
		}
	case pubsub.Event[apitypes.PermissionNotification]:
		return pubsub.Event[permission.PermissionNotification]{
			Type: e.Type,
			Payload: permission.PermissionNotification{
				ToolCallID: e.Payload.ToolCallID,
				Granted:    e.Payload.Granted,
				Denied:     e.Payload.Denied,
			},
		}
	case pubsub.Event[apitypes.Message]:
		return pubsub.Event[message.Message]{
			Type:    e.Type,
			Payload: apiTypeToMessage(e.Payload),
		}
	case pubsub.Event[apitypes.Session]:
		return pubsub.Event[session.Session]{
			Type:    e.Type,
			Payload: apiTypeToSession(e.Payload),
		}
	case pubsub.Event[apitypes.File]:
		return pubsub.Event[history.File]{
			Type:    e.Type,
			Payload: apiTypeToFile(e.Payload),
		}
	case pubsub.Event[apitypes.AgentEvent]:
		return pubsub.Event[notify.Notification]{
			Type: e.Type,
			Payload: notify.Notification{
				SessionID:    e.Payload.SessionID,
				SessionTitle: e.Payload.SessionTitle,
				Type:         notify.Type(e.Payload.Type),
			},
		}
	case pubsub.Event[apitypes.SkillsEvent]:
		return pubsub.Event[skills.Event]{
			Type:    e.Type,
			Payload: skills.Event{States: apiTypeToSkillStates(e.Payload.States)},
		}
	default:
		slog.Warn("Unknown event type in translateEvent", "type", fmt.Sprintf("%T", ev))
		return nil
	}
}

func apiTypeToSkillStates(in []apitypes.SkillState) []*skills.SkillState {
	if len(in) == 0 {
		return nil
	}
	out := make([]*skills.SkillState, len(in))
	for i, s := range in {
		state := &skills.SkillState{
			Name:  s.Name,
			Path:  s.Path,
			State: skills.DiscoveryState(s.State),
		}
		if s.Error != "" {
			state.Err = errors.New(s.Error)
		}
		out[i] = state
	}
	return out
}

func apiTypeToMCPEventType(t apitypes.MCPEventType) mcp.EventType {
	switch t {
	case apitypes.MCPEventStateChanged:
		return mcp.EventStateChanged
	case apitypes.MCPEventToolsListChanged:
		return mcp.EventToolsListChanged
	case apitypes.MCPEventPromptsListChanged:
		return mcp.EventPromptsListChanged
	case apitypes.MCPEventResourcesListChanged:
		return mcp.EventResourcesListChanged
	default:
		return mcp.EventStateChanged
	}
}

func apiTypeToSession(s apitypes.Session) session.Session {
	return session.Session{
		ID:               s.ID,
		ParentSessionID:  s.ParentSessionID,
		Title:            s.Title,
		TitleSource:      s.TitleSource,
		SummaryMessageID: s.SummaryMessageID,
		MessageCount:     s.MessageCount,
		PromptTokens:     s.PromptTokens,
		CompletionTokens: s.CompletionTokens,
		Cost:             s.Cost,
		ProjectID:        s.ProjectID,
		Scope:            s.Scope,
		Todos:            apiTypeToTodos(s.Todos),
		CreatedAt:        s.CreatedAt,
		UpdatedAt:        s.UpdatedAt,
	}
}

func apiTypeToTodos(todos []apitypes.Todo) []session.Todo {
	if len(todos) == 0 {
		return nil
	}
	out := make([]session.Todo, len(todos))
	for i, t := range todos {
		out[i] = session.Todo{
			Content:    t.Content,
			Status:     session.TodoStatus(t.Status),
			ActiveForm: t.ActiveForm,
		}
	}
	return out
}

func apiTypeToFile(f apitypes.File) history.File {
	return history.File{
		ID:        f.ID,
		SessionID: f.SessionID,
		Path:      f.Path,
		Content:   f.Content,
		Version:   f.Version,
		CreatedAt: f.CreatedAt,
		UpdatedAt: f.UpdatedAt,
	}
}

func apiTypeToMessage(m apitypes.Message) message.Message {
	msg := message.Message{
		ID:        m.ID,
		SessionID: m.SessionID,
		Role:      message.MessageRole(m.Role),
		Model:     m.Model,
		Provider:  m.Provider,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}

	for _, p := range m.Parts {
		switch v := p.(type) {
		case apitypes.TextContent:
			msg.Parts = append(msg.Parts, message.TextContent{Text: v.Text})
		case apitypes.ReasoningContent:
			msg.Parts = append(msg.Parts, message.ReasoningContent{
				Thinking:   v.Thinking,
				Signature:  v.Signature,
				StartedAt:  v.StartedAt,
				FinishedAt: v.FinishedAt,
			})
		case apitypes.ToolCall:
			msg.Parts = append(msg.Parts, message.ToolCall{
				ID:       v.ID,
				Name:     v.Name,
				Input:    v.Input,
				Finished: v.Finished,
			})
		case apitypes.ToolResult:
			msg.Parts = append(msg.Parts, message.ToolResult{
				ToolCallID:       v.ToolCallID,
				Name:             v.Name,
				Content:          v.Content,
				Data:             v.Data,
				MIMEType:         v.MIMEType,
				Metadata:         v.Metadata,
				IsError:          v.IsError,
				DeliveredToModel: v.DeliveredToModel,
				DeliveredAtStep:  v.DeliveredAtStep,
				DeliveryReason:   v.DeliveryReason,
				StoredPath:       v.StoredPath,
				OriginalSize:     v.OriginalSize,
				TruncatedBy:      v.TruncatedBy,
			})
		case apitypes.Finish:
			msg.Parts = append(msg.Parts, message.Finish{
				Reason:  message.FinishReason(v.Reason),
				Time:    v.Time,
				Message: v.Message,
				Details: v.Details,
			})
		case apitypes.ImageURLContent:
			msg.Parts = append(msg.Parts, message.ImageURLContent{URL: v.URL, Detail: v.Detail})
		case apitypes.BinaryContent:
			msg.Parts = append(msg.Parts, message.BinaryContent{Path: v.Path, MIMEType: v.MIMEType, Data: v.Data})
		}
	}

	return msg
}

func apiTypeToMessages(msgs []apitypes.Message) []message.Message {
	out := make([]message.Message, len(msgs))
	for i, m := range msgs {
		out[i] = apiTypeToMessage(m)
	}
	return out
}

func apiTypeToFiles(files []apitypes.File) []history.File {
	out := make([]history.File, len(files))
	for i, f := range files {
		out[i] = apiTypeToFile(f)
	}
	return out
}

func sessionToAPIType(s session.Session) apitypes.Session {
	return apitypes.Session{
		ID:               s.ID,
		ParentSessionID:  s.ParentSessionID,
		Title:            s.Title,
		SummaryMessageID: s.SummaryMessageID,
		MessageCount:     s.MessageCount,
		PromptTokens:     s.PromptTokens,
		CompletionTokens: s.CompletionTokens,
		Cost:             s.Cost,
		ProjectID:        s.ProjectID,
		Scope:            s.Scope,
		Todos:            todosToAPIType(s.Todos),
		CreatedAt:        s.CreatedAt,
		UpdatedAt:        s.UpdatedAt,
	}
}

func todosToAPIType(todos []session.Todo) []apitypes.Todo {
	if len(todos) == 0 {
		return nil
	}
	out := make([]apitypes.Todo, len(todos))
	for i, t := range todos {
		out[i] = apitypes.Todo{
			Content:    t.Content,
			Status:     string(t.Status),
			ActiveForm: t.ActiveForm,
		}
	}
	return out
}
