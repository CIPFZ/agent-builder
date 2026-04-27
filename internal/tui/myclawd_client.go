package tui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gorilla/websocket"

	protocolws "myclaw/internal/protocol/ws"
)

type MyclawdClient struct {
	ctx      context.Context
	cancel   context.CancelFunc
	baseURL  string
	agentID  string
	clientID string
	store    *clientStore
	logger   protocolLogger

	mu         sync.RWMutex
	conn       *websocket.Conn
	sessionID  string
	sessionKey string
	send       func(tea.Msg)
	pending    map[string]chan protocolws.Message
	nextID     atomic.Uint64

	turnMu           sync.Mutex
	turnStartedAt    time.Time
	firstDeltaLogged bool
}

type protocolLogger interface {
	Log(level, component, event, message string, fields map[string]any)
}

func NewMyclawdClient(ctx context.Context, baseURL, agentID string, store *clientStore, logger protocolLogger) *MyclawdClient {
	if store == nil {
		store = newClientStore()
	}
	derived, cancel := context.WithCancel(ctx)
	return &MyclawdClient{
		ctx:      derived,
		cancel:   cancel,
		baseURL:  strings.TrimRight(baseURL, "/"),
		agentID:  strings.TrimSpace(agentID),
		clientID: "myclaw-tui",
		store:    store,
		logger:   logger,
		pending:  make(map[string]chan protocolws.Message),
	}
}

func (c *MyclawdClient) Attach(send func(tea.Msg)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.send = send
}

func (c *MyclawdClient) Start() error {
	if err := c.connect(); err != nil {
		return err
	}
	if err := c.refreshSnapshots(); err != nil {
		c.log("warn", "snapshot.refresh.error", err.Error(), nil)
	}
	return nil
}

func (c *MyclawdClient) Close() error {
	c.cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *MyclawdClient) SendUserMessage(input string) error {
	startedAt := time.Now()
	c.startTurnTiming(startedAt)
	c.log("info", "turn.send.start", "sending user message", nil)
	_, err := c.request(protocolws.MethodSendMessage, map[string]any{"content": input})
	if err != nil {
		c.log("error", "turn.send.error", err.Error(), map[string]any{"duration_ms": elapsedMillis(startedAt)})
		return err
	}
	c.log("info", "turn.send.accepted", "myclawd accepted user message", map[string]any{"duration_ms": elapsedMillis(startedAt)})
	return nil
}

func (c *MyclawdClient) Approve(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("missing approval id")
	}
	_, err := c.request(protocolws.MethodApprovalApprove, map[string]any{"approval_id": id})
	return err
}

func (c *MyclawdClient) Reject(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("missing approval id")
	}
	_, err := c.request(protocolws.MethodApprovalReject, map[string]any{"approval_id": id})
	return err
}

func (c *MyclawdClient) SetSessionModel(model string) error {
	_, err := c.request(protocolws.MethodSessionSetModel, map[string]any{"model": model})
	if err == nil {
		_ = c.refreshSessionStatus()
	}
	return err
}

func (c *MyclawdClient) ClearSessionModel() error {
	_, err := c.request(protocolws.MethodSessionSetModel, map[string]any{"model": "default"})
	if err == nil {
		_ = c.refreshSessionStatus()
	}
	return err
}

func (c *MyclawdClient) ContextSnapshot() contextSnapshot {
	return contextSnapshot{Model: "Unavailable in myclawd control plane"}
}

func (c *MyclawdClient) CompactionSnapshot() compactionSnapshot {
	return compactionSnapshot{
		LastCompactionReason: "Unavailable in myclawd control plane",
	}
}
func (c *MyclawdClient) CompactSession(string) (compactionActionResult, error) {
	return compactionActionResult{}, errors.New("compaction is not exposed by myclawd yet")
}
func (c *MyclawdClient) MicrocompactSession() (compactionActionResult, error) {
	return compactionActionResult{}, errors.New("microcompact is not exposed by myclawd yet")
}

func (c *MyclawdClient) PlatformStatusSnapshot() platformStatusSnapshot {
	return c.store.sessionSnapshot()
}

func (c *MyclawdClient) MCPSnapshot() mcpSnapshot {
	return c.store.mcpSnapshot()
}

func (c *MyclawdClient) TaskPanelSnapshot() taskPanelSnapshot {
	return c.store.taskSnapshot()
}

func (c *MyclawdClient) connect() error {
	wsURL, err := websocketURL(c.baseURL)
	if err != nil {
		return err
	}
	conn, _, err := websocket.DefaultDialer.DialContext(c.ctx, wsURL, nil)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	go c.readLoop()

	id := c.nextRequestID()
	responseCh := c.registerPending(id)
	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     id,
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":                        "tui",
			"client_identity":             c.clientID,
			"agent_id":                    valueOrDefault(c.agentID, "main"),
			"supports_permission_control": true,
		},
	}); err != nil {
		c.unregisterPending(id)
		return err
	}

	response, err := c.awaitResponse(id, responseCh)
	if err != nil {
		return err
	}
	if !response.OK {
		return responseError(response)
	}
	c.sessionID = stringValue(response.Payload, "session_id")
	c.sessionKey = stringValue(response.Payload, "session_key")
	c.store.setSession(platformStatusSnapshot{
		SessionID:  c.sessionID,
		SessionKey: c.sessionKey,
		AgentID:    valueOrDefault(c.agentID, "main"),
		IsMain:     true,
	})
	return nil
}

func (c *MyclawdClient) refreshSnapshots() error {
	if err := c.refreshSessionStatus(); err != nil {
		return err
	}
	if err := c.refreshTasks(); err != nil {
		return err
	}
	if err := c.refreshMCP(); err != nil {
		return err
	}
	if err := c.refreshApprovals(); err != nil {
		return err
	}
	return nil
}

func (c *MyclawdClient) refreshSessionStatus() error {
	msg, err := c.request(protocolws.MethodSessionStatus, map[string]any{})
	if err != nil {
		return err
	}
	c.store.setSession(platformStatusSnapshot{
		SessionID:      stringValue(msg.Payload, "session_id"),
		SessionKey:     stringValue(msg.Payload, "session_key"),
		AgentID:        stringValue(msg.Payload, "agent_id"),
		IsMain:         boolValue(msg.Payload, "is_main"),
		WorkspaceRoots: stringSliceValue(msg.Payload["workspace_roots"]),
		BaseModel:      stringValue(msg.Payload, "main_loop_model"),
		ModelOverride:  stringValue(msg.Payload, "session_main_loop_model_override"),
		ResolvedModel:  stringValue(msg.Payload, "resolved_main_loop_model"),
	})
	return nil
}

func (c *MyclawdClient) refreshMCP() error {
	msg, err := c.request(protocolws.MethodMCPStatus, map[string]any{})
	if err != nil {
		return err
	}
	servers := parseMCPServers(msg.Payload["servers"])
	c.store.setMCP(mcpSnapshot{Servers: servers})
	return nil
}

func (c *MyclawdClient) refreshTasks() error {
	msg, err := c.request(protocolws.MethodSubagentList, map[string]any{})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not supported") {
			c.store.setTasks(taskPanelSnapshot{
				SessionID: c.sessionID,
				Tasks: []taskSnapshot{{
					RunID:   "unsupported",
					Label:   "Subagent inventory unavailable",
					Status:  "unsupported",
					Message: err.Error(),
				}},
			})
			return nil
		}
		return err
	}
	snapshot := taskPanelSnapshot{SessionID: c.sessionID}
	items, _ := msg.Payload["runs"].([]any)
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		task := taskSnapshot{
			RunID:           stringValue(entry, "run_id"),
			Label:           stringValue(entry, "label"),
			Status:          stringValue(entry, "status"),
			ParentSessionID: stringValue(entry, "parent_session_id"),
			ChildSessionID:  stringValue(entry, "child_session_id"),
			ChildSessionKey: stringValue(entry, "child_session_key"),
			Output:          stringValue(entry, "output"),
			Error:           stringValue(entry, "error"),
		}
		switch task.Status {
		case "running":
			snapshot.RunningCount++
		case "completed":
			snapshot.CompletedCount++
		case "failed":
			snapshot.FailedCount++
		case "stopped":
			snapshot.StoppedCount++
		}
		snapshot.Tasks = append(snapshot.Tasks, task)
	}
	c.store.setTasks(snapshot)
	return nil
}

func (c *MyclawdClient) refreshApprovals() error {
	msg, err := c.request(protocolws.MethodApprovalList, map[string]any{})
	if err != nil {
		return err
	}
	items, _ := msg.Payload["approvals"].([]any)
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		c.store.applyApproval(approvalView{
			ID:        stringValue(entry, "id"),
			ToolName:  stringValue(entry, "tool_name"),
			ToolInput: stringValue(entry, "tool_input"),
			Status:    stringValue(entry, "status"),
			Reason:    stringValue(entry, "reason"),
			SessionID: stringValue(entry, "session_id"),
			RunID:     stringValue(entry, "run_id"),
		})
	}
	return nil
}

func (c *MyclawdClient) readLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}
		msg, err := c.readMessage()
		if err != nil {
			c.failPending()
			c.dispatch(BridgeErrMsg{Err: err})
			return
		}
		if msg.Type == protocolws.TypeResponse && c.resolvePending(msg) {
			continue
		}
		if event, ok := parseClientEventMessage(wsMessageLike{Type: msg.Type, Event: msg.Event, Payload: msg.Payload}); ok {
			c.store.applyEvent(event)
			c.logTurnEvent(event)
			if event.Type == "permission.required" && event.Tool != nil && event.Tool.Approval != nil {
				c.store.applyApproval(approvalView{
					ID:         event.Tool.Approval.ID,
					ToolName:   event.Tool.Approval.ToolName,
					ToolInput:  event.Tool.Approval.ToolInput,
					Status:     event.Tool.Approval.Status,
					Reason:     event.Tool.Approval.Reason,
					SessionID:  event.Tool.Approval.SessionID,
					RunID:      event.Tool.Approval.RunID,
					Category:   event.Tool.Approval.Category,
					RuleSource: event.Tool.Approval.RuleSource,
				})
			}
			c.dispatch(RuntimeEventMsg{Event: event})
		}
		if msg.Type == protocolws.TypeEvent {
			switch msg.Event {
			case protocolws.EventSubagentUpdated, protocolws.EventSubagentCompleted:
				_ = c.refreshTasks()
			case "approval.updated":
				_ = c.refreshApprovals()
			}
		}
	}
}

func (c *MyclawdClient) request(method string, payload map[string]any) (protocolws.Message, error) {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return protocolws.Message{}, errors.New("not connected")
	}
	id := c.nextRequestID()
	responseCh := c.registerPending(id)
	if err := conn.WriteJSON(protocolws.Message{
		Type:    protocolws.TypeRequest,
		ID:      id,
		Method:  method,
		Payload: payload,
	}); err != nil {
		c.unregisterPending(id)
		return protocolws.Message{}, err
	}
	msg, err := c.awaitResponse(id, responseCh)
	if err != nil {
		return protocolws.Message{}, err
	}
	if !msg.OK {
		return protocolws.Message{}, responseError(msg)
	}
	return msg, nil
}

func (c *MyclawdClient) readMessage() (protocolws.Message, error) {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return protocolws.Message{}, errors.New("not connected")
	}
	var msg protocolws.Message
	if err := conn.ReadJSON(&msg); err != nil {
		return protocolws.Message{}, err
	}
	if msg.Payload == nil {
		msg.Payload = map[string]any{}
	}
	return msg, nil
}

func (c *MyclawdClient) dispatch(msg tea.Msg) {
	c.mu.RLock()
	send := c.send
	c.mu.RUnlock()
	if send != nil {
		send(msg)
	}
}

func (c *MyclawdClient) nextRequestID() string {
	return fmt.Sprintf("tui-%d", c.nextID.Add(1))
}

func (c *MyclawdClient) registerPending(id string) chan protocolws.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan protocolws.Message, 1)
	c.pending[id] = ch
	return ch
}

func (c *MyclawdClient) unregisterPending(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, id)
}

func (c *MyclawdClient) resolvePending(msg protocolws.Message) bool {
	c.mu.Lock()
	ch, ok := c.pending[msg.ID]
	if ok {
		delete(c.pending, msg.ID)
	}
	c.mu.Unlock()
	if !ok {
		return false
	}
	ch <- msg
	close(ch)
	return true
}

func (c *MyclawdClient) failPending() {
	c.mu.Lock()
	channels := make([]chan protocolws.Message, 0, len(c.pending))
	for id, ch := range c.pending {
		channels = append(channels, ch)
		delete(c.pending, id)
	}
	c.mu.Unlock()
	for _, ch := range channels {
		close(ch)
	}
}

func (c *MyclawdClient) awaitResponse(id string, ch chan protocolws.Message) (protocolws.Message, error) {
	select {
	case <-c.ctx.Done():
		c.unregisterPending(id)
		return protocolws.Message{}, c.ctx.Err()
	case msg, ok := <-ch:
		if !ok {
			return protocolws.Message{}, errors.New("connection closed")
		}
		return msg, nil
	case <-time.After(10 * time.Second):
		c.unregisterPending(id)
		return protocolws.Message{}, errors.New("myclawd request timed out")
	}
}

func (c *MyclawdClient) log(level, event, message string, fields map[string]any) {
	if c.logger != nil {
		c.logger.Log(level, "tui.client", event, message, fields)
	}
}

func (c *MyclawdClient) startTurnTiming(startedAt time.Time) {
	c.turnMu.Lock()
	defer c.turnMu.Unlock()
	c.turnStartedAt = startedAt
	c.firstDeltaLogged = false
}

func (c *MyclawdClient) logTurnEvent(event clientEvent) {
	c.turnMu.Lock()
	startedAt := c.turnStartedAt
	firstDeltaLogged := c.firstDeltaLogged
	if event.Type == "assistant.delta" && !firstDeltaLogged {
		c.firstDeltaLogged = true
	}
	c.turnMu.Unlock()

	if startedAt.IsZero() {
		return
	}
	fields := map[string]any{"since_send_ms": elapsedMillis(startedAt)}
	switch event.Type {
	case "agent.lifecycle.start":
		c.log("info", "turn.lifecycle.start", "myclawd started processing turn", fields)
	case "model.request.start":
		c.log("info", "turn.model_request.start", "model request started", fields)
	case "assistant.delta":
		if !firstDeltaLogged {
			c.log("info", "turn.first_delta", "first assistant delta received", fields)
		}
	case "message.created":
		if event.Message != nil && event.Message.Role == "assistant" {
			c.log("info", "turn.assistant.final", "assistant final message received", fields)
		}
	case "model.request.end":
		c.log("info", "turn.model_request.end", "model request completed", fields)
	case "run.error":
		c.log("error", "turn.error", event.Error, fields)
	}
}

func elapsedMillis(startedAt time.Time) int64 {
	if startedAt.IsZero() {
		return 0
	}
	return time.Since(startedAt).Milliseconds()
}

func websocketURL(base string) (string, error) {
	if strings.TrimSpace(base) == "" {
		base = "http://127.0.0.1:8080/ws"
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	if u.Scheme == "ws" || u.Scheme == "wss" {
		return u.String(), nil
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported myclawd url scheme %q", u.Scheme)
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/ws"
	}
	return u.String(), nil
}

func responseError(msg protocolws.Message) error {
	if msg.Error != nil && strings.TrimSpace(msg.Error.Message) != "" {
		return errors.New(msg.Error.Message)
	}
	return errors.New("myclawd request failed")
}

func parseMCPServers(raw any) []mcpServerSnapshot {
	items, _ := raw.([]any)
	out := make([]mcpServerSnapshot, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, mcpServerSnapshot{
			Name:          stringValue(entry, "name"),
			TransportType: stringValue(entry, "transport_type"),
			Endpoint:      stringValue(entry, "endpoint"),
			Enabled:       boolValue(entry, "enabled"),
			Tools:         stringSliceValue(entry["tools"]),
			Prompts:       stringSliceValue(entry["prompts"]),
			Resources:     stringSliceValue(entry["resources"]),
		})
	}
	return out
}

func stringSliceValue(raw any) []string {
	items, _ := raw.([]any)
	if len(items) == 0 {
		return nil
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			values = append(values, text)
		}
	}
	return values
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
