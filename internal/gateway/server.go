package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"myclaw/internal/agent"
	"myclaw/internal/approval"
	"myclaw/internal/compaction"
	"myclaw/internal/llm"
	"myclaw/internal/orchestration"
	"myclaw/internal/permissions"
	protocolws "myclaw/internal/protocol/ws"
	"myclaw/internal/queryengine"
	"myclaw/internal/runtime"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	"myclaw/internal/workspace"
)

type Server struct {
	upgrader websocket.Upgrader
	logger   *log.Logger

	nextID                   atomic.Uint64
	sessionManager           *session.Manager
	runner                   *runtime.Runner
	coordinator              *orchestration.Coordinator
	orchestrator             orchestration.Hook
	queue                    *runtime.Queue
	fallbackPermissionHook   queryengine.PermissionHook
	permissionControlTimeout time.Duration
	mu                       sync.RWMutex
	clients                  map[string]*Client
}

type Options struct {
	PermissionPolicy          permissions.Policy
	Compactor                 *compaction.Service
	Orchestrator              orchestration.Hook
	Runner                    *runtime.Runner
	PermissionHook            queryengine.PermissionHook
	PreToolUseHook            queryengine.PreToolUseHook
	PostToolUseHook           queryengine.PostToolUseHook
	PostToolUseFailureHook    queryengine.PostToolUseFailureHook
	PermissionUpdatePersister queryengine.PermissionUpdatePersister
	PermissionControlTimeout  time.Duration
	MainLoopModel             string
	LLMProvider               string
	MCPClients                []tools.MCPConnection
	DisableMCPPromptSkills    bool
}

func shouldSuppressContinuationRunError(err error) bool {
	var approvalErr *queryengine.ApprovalRequiredError
	return errors.As(err, &approvalErr)
}

func NewServer(logger *log.Logger, sessionManager *session.Manager, llmClient llm.Client) *Server {
	return NewServerWithOptions(logger, sessionManager, llmClient, Options{
		PermissionPolicy: permissions.Policy{
			Mode:           permissions.ModeWorkspaceWrite,
			WorkspaceRoots: []string{defaultWorkspaceRoot()},
		},
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:         12,
			PreserveRecentTurns: 6,
			SummaryPrefix:       "Summary:",
		}),
	})
}

func NewServerWithOptions(logger *log.Logger, sessionManager *session.Manager, llmClient llm.Client, options Options) *Server {
	if logger == nil {
		logger = log.Default()
	}
	if sessionManager == nil {
		sessionManager = session.NewManager(nil)
	}
	if llmClient == nil {
		llmClient = llm.NewUnavailableClient("llm client is not configured")
	}

	coordinator := orchestration.NewCoordinator()
	hook := orchestration.Hook(coordinator)
	if options.Orchestrator != nil {
		hook = orchestration.Chain{coordinator, options.Orchestrator}
	}

	server := &Server{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool {
				return true
			},
		},
		logger:                   logger,
		sessionManager:           sessionManager,
		coordinator:              coordinator,
		orchestrator:             hook,
		fallbackPermissionHook:   options.PermissionHook,
		permissionControlTimeout: options.PermissionControlTimeout,
		clients:                  make(map[string]*Client),
	}
	if server.permissionControlTimeout == 0 {
		server.permissionControlTimeout = 30 * time.Second
	}

	runner := options.Runner
	if runner == nil {
		runner = runtime.NewRunnerWithOptions(sessionManager, llmClient, workspace.NewLoader(defaultWorkspaceRoot()), nil, runtime.Options{
			PermissionPolicy:          options.PermissionPolicy,
			Compactor:                 options.Compactor,
			Orchestrator:              hook,
			PermissionHook:            server,
			PreToolUseHook:            options.PreToolUseHook,
			PostToolUseHook:           options.PostToolUseHook,
			PostToolUseFailureHook:    options.PostToolUseFailureHook,
			PermissionUpdatePersister: options.PermissionUpdatePersister,
			MainLoopModel:             options.MainLoopModel,
			LLMProvider:               options.LLMProvider,
			MCPClients:                append([]tools.MCPConnection(nil), options.MCPClients...),
			DisableMCPPromptSkills:    options.DisableMCPPromptSkills,
			ReportToolProgress:        server.reportToolProgress,
		})
	} else {
		runner.SetReportToolProgress(server.reportToolProgress)
	}
	server.runner = runner
	server.queue = runtime.NewQueue(runner)
	return server
}

func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Printf("gateway upgrade failed: %v", err)
		return
	}

	client := NewClient(s.nextClientID(), conn)
	s.addClient(client)
	defer s.removeClient(client.ID())
	defer client.Close()

	s.logger.Printf("gateway connected: client=%s", client.ID())

	if err := s.handleClient(client); err != nil {
		s.logger.Printf("gateway client closed: client=%s err=%v", client.ID(), err)
	}
}

func (s *Server) handleClient(client *Client) error {
	first, err := s.readMessage(client)
	if err != nil {
		return err
	}

	if first.Type != protocolws.TypeRequest || first.Method != protocolws.MethodConnect {
		_ = client.WriteJSON(protocolws.ErrorResponse(first.ID, "first message must be req/connect"))
		return nil
	}

	connectPayload, err := parseConnectPayload(first.Payload)
	if err != nil {
		_ = client.WriteJSON(protocolws.InvalidConnectResponse(first.ID, err.Error()))
		return nil
	}

	sess, err := s.sessionManager.Resolve(connectPayload.AgentID, connectPayload.SessionKey)
	if err != nil {
		_ = client.WriteJSON(protocolws.InvalidConnectResponse(first.ID, err.Error()))
		return nil
	}
	client.BindSession(sess.ID, sess.Key)
	client.SetSupportsPermissionControl(connectPayload.SupportsPermissionControl || connectPayload.Role == "sdk")

	if err := client.WriteJSON(protocolws.ConnectResponse(first.ID, sess.ID, sess.Key)); err != nil {
		return err
	}

	if err := client.WriteJSON(protocolws.EventMessage(protocolws.EventHello, map[string]any{
		"client_id":       client.ID(),
		"client_identity": connectPayload.ClientIdentity,
		"session_id":      sess.ID,
		"session_key":     sess.Key,
		"agent_id":        sess.AgentID,
	})); err != nil {
		return err
	}

	for {
		inbound, err := s.readMessage(client)
		if err != nil {
			return err
		}

		if inbound.Type == protocolws.TypeControlResponse {
			if !client.ResolveControlResponse(inbound) {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, "control response does not match a pending request")); err != nil {
					return err
				}
			}
			continue
		}
		if inbound.Type != protocolws.TypeRequest {
			if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, "only req messages are supported")); err != nil {
				return err
			}
			continue
		}

		switch inbound.Method {
		case protocolws.MethodSendMessage:
			sendPayload, err := parseSendMessagePayload(inbound.Payload)
			if err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}

			msg, err := s.sessionManager.AppendMessage(client.SessionID(), "user", sendPayload.Content)
			if err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}

			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"message_id": msg.ID,
					"status":     "accepted",
				},
			}); err != nil {
				return err
			}

			if err := client.WriteJSON(protocolws.EventMessage(protocolws.EventMessageCreated, map[string]any{
				"client_id":   client.ID(),
				"session_id":  client.SessionID(),
				"session_key": client.SessionKey(),
				"message": map[string]any{
					"id":         msg.ID,
					"role":       msg.Role,
					"content":    msg.Content,
					"created_at": msg.CreatedAt.Format(time.RFC3339Nano),
				},
			})); err != nil {
				return err
			}

			pending := s.queue.Enqueue(context.Background(), sess, msg, runtimeSink{
				client: client,
			})
			if pending > 1 {
				if err := client.WriteJSON(protocolws.EventMessage("queue.enqueued", map[string]any{
					"session_id":  client.SessionID(),
					"session_key": client.SessionKey(),
					"pending":     pending,
				})); err != nil {
					return err
				}
			}
		case protocolws.MethodSpawnSubagent:
			spawnPayload, err := parseSpawnSubagentPayload(inbound.Payload)
			if err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}

			run, err := s.runner.SpawnSubagentWithOptions(context.Background(), sess, spawnPayload.Label, spawnPayload.Prompt, runtime.SubagentOptions{
				AllowedTools: append([]string(nil), spawnPayload.AllowedTools...),
				Model:        strings.TrimSpace(spawnPayload.Model),
				Effort:       strings.TrimSpace(spawnPayload.Effort),
				AgentType:    strings.TrimSpace(spawnPayload.AgentType),
				Isolation:    strings.TrimSpace(spawnPayload.Isolation),
				UseFork:      spawnPayload.UseFork,
			})
			if err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}
			if err := client.WriteJSON(protocolws.Message{
				Type:    protocolws.TypeResponse,
				ID:      inbound.ID,
				OK:      true,
				Payload: subagentPayload(*run),
			}); err != nil {
				return err
			}
			s.watchSubagentCompletion(client, sess, run.ID)
		case protocolws.MethodSessionStatus:
			statusPayload, err := parseSessionStatusPayload(inbound.Payload)
			if err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}
			targetSession, resolveErr := s.resolveSessionForStatus(client, statusPayload)
			if resolveErr != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, resolveErr.Error())); err != nil {
					return err
				}
				continue
			}
			messages, _ := s.sessionManager.Messages(targetSession.ID)
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"session_id":                       targetSession.ID,
					"session_key":                      targetSession.Key,
					"agent_id":                         targetSession.AgentID,
					"is_main":                          targetSession.IsMain,
					"message_count":                    len(messages),
					"permission_mode":                  string(s.runner.PermissionPolicyForSession(targetSession.ID).Mode),
					"subagent_mode":                    string(s.runner.PermissionPolicyForSession(targetSession.ID).SubagentMode),
					"plan_mode":                        s.runner.PermissionPolicyForSession(targetSession.ID).PlanMode,
					"auto_mode":                        s.runner.PermissionPolicyForSession(targetSession.ID).AutoMode,
					"workspace_roots":                  toAnySlice(s.runner.PermissionPolicyForSession(targetSession.ID).WorkspaceRoots),
					"main_loop_model":                  s.runner.BaseMainLoopModelForSession(targetSession.ID),
					"session_main_loop_model_override": s.runner.SessionMainLoopModelOverride(targetSession.ID),
					"resolved_main_loop_model":         s.runner.ResolvedMainLoopModelForSession(targetSession.ID),
				},
			}); err != nil {
				return err
			}
		case protocolws.MethodSessionList:
			sessions := s.sessionManager.ListSessions()
			items := make([]map[string]any, 0, len(sessions))
			for _, sessionItem := range sessions {
				messages, _ := s.sessionManager.Messages(sessionItem.ID)
				items = append(items, sessionSummaryPayload(sessionItem, messages))
			}
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"sessions": items,
				},
			}); err != nil {
				return err
			}
		case protocolws.MethodSessionNew:
			newPayload, err := parseSessionNewPayload(inbound.Payload)
			if err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}
			agentID := strings.TrimSpace(newPayload.AgentID)
			if agentID == "" {
				agentID = sess.AgentID
			}
			if agentID == "" {
				agentID = "main"
			}
			newSession := s.sessionManager.CreateSession(agentID)
			sess = newSession
			client.BindSession(newSession.ID, newSession.Key)
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"session_id":  newSession.ID,
					"session_key": newSession.Key,
					"agent_id":    newSession.AgentID,
					"is_main":     newSession.IsMain,
					"status":      "created",
				},
			}); err != nil {
				return err
			}
			if err := client.WriteJSON(protocolws.EventMessage(protocolws.EventHello, map[string]any{
				"client_id":   client.ID(),
				"session_id":  newSession.ID,
				"session_key": newSession.Key,
				"agent_id":    newSession.AgentID,
			})); err != nil {
				return err
			}
		case protocolws.MethodSessionMessages:
			messagesPayload, err := parseSessionMessagesPayload(inbound.Payload)
			if err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}
			targetSession, resolveErr := s.resolveSessionForStatus(client, protocolws.SessionStatusPayload{
				SessionID:  messagesPayload.SessionID,
				SessionKey: messagesPayload.SessionKey,
			})
			if resolveErr != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, resolveErr.Error())); err != nil {
					return err
				}
				continue
			}
			messages, _ := s.sessionManager.Messages(targetSession.ID)
			items := make([]map[string]any, 0, len(messages))
			for _, message := range messages {
				items = append(items, sessionMessagePayload(message))
			}
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"session_id":  targetSession.ID,
					"session_key": targetSession.Key,
					"messages":    items,
				},
			}); err != nil {
				return err
			}
		case protocolws.MethodSessionDelete:
			deletePayload, err := parseSessionDeletePayload(inbound.Payload)
			if err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}
			targetSession, resolveErr := s.resolveSessionForStatus(client, protocolws.SessionStatusPayload{
				SessionID:  deletePayload.SessionID,
				SessionKey: deletePayload.SessionKey,
			})
			if resolveErr != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, resolveErr.Error())); err != nil {
					return err
				}
				continue
			}
				activeSession := session.Session{}
				deletedActiveSession := false
				if targetSession.ID == client.SessionID() {
					if targetSession.IsMain {
						if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, "main session cannot be deleted")); err != nil {
							return err
						}
						continue
					}
					deletedActiveSession = true
					activeSession = s.sessionManager.GetOrCreateMain(targetSession.AgentID)
					sess = activeSession
					client.BindSession(activeSession.ID, activeSession.Key)
				} else if current, ok := s.sessionManager.GetByID(client.SessionID()); ok {
					activeSession = current
				}
				if err := s.sessionManager.DeleteSession(targetSession.ID); err != nil {
					if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
						return err
					}
					continue
				}
				payload := map[string]any{
					"session_id":  targetSession.ID,
					"session_key": targetSession.Key,
					"status":      "deleted",
				}
				if activeSession.ID != "" {
					payload["active_session_id"] = activeSession.ID
					payload["active_session_key"] = activeSession.Key
				}
				if err := client.WriteJSON(protocolws.Message{
					Type: protocolws.TypeResponse,
					ID:   inbound.ID,
					OK:   true,
					Payload: payload,
				}); err != nil {
					return err
				}
				if deletedActiveSession && activeSession.ID != "" {
					if err := client.WriteJSON(protocolws.EventMessage(protocolws.EventHello, map[string]any{
						"client_id":   client.ID(),
						"session_id":  activeSession.ID,
						"session_key": activeSession.Key,
						"agent_id":    activeSession.AgentID,
					})); err != nil {
						return err
					}
				}
		case protocolws.MethodMCPStatus:
			statusPayload, err := parseMCPStatusPayload(inbound.Payload)
			if err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}
			servers := s.runner.MCPServers()
			if serverName := strings.TrimSpace(statusPayload.Server); serverName != "" {
				filtered, ok := filterMCPServerSnapshots(servers, serverName)
				if !ok {
					if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, "MCP server not found")); err != nil {
						return err
					}
					continue
				}
				servers = filtered
			}
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"inventory": mcpInventoryPayload(s.runner.MCPInventory()),
					"servers":   mcpServerPayloads(servers),
				},
			}); err != nil {
				return err
			}
		case protocolws.MethodMCPReconnect:
			actionPayload, err := parseMCPActionPayload(inbound.Payload, "mcp_reconnect")
			if err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}
			serverSnapshot, err := s.runner.ReconnectMCP(context.Background(), actionPayload.Server)
			if err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"status":    "reconnected",
					"inventory": mcpInventoryPayload(s.runner.MCPInventory()),
					"server":    mcpServerPayload(serverSnapshot),
				},
			}); err != nil {
				return err
			}
		case protocolws.MethodMCPAuthenticate:
			actionPayload, err := parseMCPActionPayload(inbound.Payload, "mcp_authenticate")
			if err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}
			authResult, serverSnapshot, err := s.runner.AuthenticateMCP(context.Background(), actionPayload.Server)
			if err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"status":    strings.TrimSpace(authResult.Status),
					"inventory": mcpInventoryPayload(s.runner.MCPInventory()),
					"server":    mcpServerPayload(serverSnapshot),
					"auth":      mcpAuthStartPayload(authResult),
				},
			}); err != nil {
				return err
			}
		case protocolws.MethodOrchestrationStatus:
			targetSession, resolveErr := s.resolveSessionForStatus(client, protocolws.SessionStatusPayload{})
			if resolveErr != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, resolveErr.Error())); err != nil {
					return err
				}
				continue
			}
			runs := s.coordinator.ListBySession(targetSession.ID)
			items := make([]map[string]any, 0, len(runs))
			for _, run := range runs {
				items = append(items, orchestrationPayload(run))
			}
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"runs": items,
				},
			}); err != nil {
				return err
			}
		case protocolws.MethodOrchestrationHistory:
			historyPayload := protocolws.OrchestrationHistoryPayload{}
			if raw, err := json.Marshal(inbound.Payload); err == nil {
				_ = json.Unmarshal(raw, &historyPayload)
			}
			targetSession, resolveErr := s.resolveSessionForStatus(client, protocolws.SessionStatusPayload{
				SessionID:  historyPayload.SessionID,
				SessionKey: historyPayload.SessionKey,
			})
			if resolveErr != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, resolveErr.Error())); err != nil {
					return err
				}
				continue
			}
			if strings.TrimSpace(historyPayload.RunID) == "" {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, "orchestration_history payload requires run_id")); err != nil {
					return err
				}
				continue
			}
			run, ok := s.coordinator.GetRun(historyPayload.RunID)
			if !ok || run.SessionID != targetSession.ID {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, "run not found")); err != nil {
					return err
				}
				continue
			}
			history := s.coordinator.FilteredHistoryByRun(historyPayload.RunID, orchestration.HistoryFilter{
				Status:           historyPayload.Status,
				DecisionPriority: historyPayload.DecisionPriority,
			})
			items := make([]map[string]any, 0, len(history))
			for _, record := range history {
				items = append(items, orchestrationHistoryPayload(record))
			}
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"run_id":   historyPayload.RunID,
					"history":  items,
					"status":   run.Status,
					"decision": run.DecisionType,
					"summary": map[string]any{
						"record_count":      len(items),
						"status_filter":     historyPayload.Status,
						"decision_priority": historyPayload.DecisionPriority,
						"current_status":    run.Status,
						"current_decision":  run.DecisionType,
						"current_priority":  run.DecisionPriority,
					},
				},
			}); err != nil {
				return err
			}
		case protocolws.MethodOrchestrationSummary:
			targetSession, resolveErr := s.resolveSessionForStatus(client, protocolws.SessionStatusPayload{})
			if resolveErr != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, resolveErr.Error())); err != nil {
					return err
				}
				continue
			}
			summary := s.coordinator.SummaryBySession(targetSession.ID)
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"session_id":                targetSession.ID,
					"run_count":                 summary.RunCount,
					"status_counts":             toAnyMap(summary.StatusCounts),
					"priority_counts":           toAnyMap(summary.PriorityCounts),
					"recommended_action_counts": toAnyMap(summary.RecommendedActionCounts),
					"top_priority":              summary.TopPriority,
				},
			}); err != nil {
				return err
			}
		case protocolws.MethodOrchestrationEvaluate:
			evaluatePayload := protocolws.OrchestrationEvaluatePayload{}
			if raw, err := json.Marshal(inbound.Payload); err == nil {
				_ = json.Unmarshal(raw, &evaluatePayload)
			}
			targetSession, resolveErr := s.resolveSessionForStatus(client, protocolws.SessionStatusPayload{
				SessionID:  evaluatePayload.SessionID,
				SessionKey: evaluatePayload.SessionKey,
			})
			if resolveErr != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, resolveErr.Error())); err != nil {
					return err
				}
				continue
			}
			suggestions := s.coordinator.EvaluateSessionWithFilter(targetSession.ID, orchestration.SuggestionFilter{
				Category:     evaluatePayload.Category,
				Priority:     evaluatePayload.Priority,
				BlockingOnly: evaluatePayload.BlockingOnly,
			})
			items := make([]map[string]any, 0, len(suggestions))
			for _, suggestion := range suggestions {
				items = append(items, orchestrationSuggestionPayload(suggestion))
			}
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"session_id":  targetSession.ID,
					"suggestions": items,
					"summary": map[string]any{
						"suggestion_count": len(items),
						"blocking_count":   countBlockingSuggestions(suggestions),
						"category":         evaluatePayload.Category,
						"priority":         evaluatePayload.Priority,
						"blocking_only":    evaluatePayload.BlockingOnly,
					},
				},
			}); err != nil {
				return err
			}
		case protocolws.MethodOrchestrationPlan:
			planPayload := protocolws.OrchestrationEvaluatePayload{}
			if raw, err := json.Marshal(inbound.Payload); err == nil {
				_ = json.Unmarshal(raw, &planPayload)
			}
			targetSession, resolveErr := s.resolveSessionForStatus(client, protocolws.SessionStatusPayload{
				SessionID:  planPayload.SessionID,
				SessionKey: planPayload.SessionKey,
			})
			if resolveErr != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, resolveErr.Error())); err != nil {
					return err
				}
				continue
			}
			plan := s.coordinator.PlanSession(targetSession.ID, orchestration.SuggestionFilter{
				Category:     planPayload.Category,
				Priority:     planPayload.Priority,
				BlockingOnly: planPayload.BlockingOnly,
			})
			steps := make([]map[string]any, 0, len(plan.Steps))
			for _, step := range plan.Steps {
				steps = append(steps, orchestrationPlanStepPayload(step))
			}
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"session_id":        targetSession.ID,
					"summary":           plan.Summary,
					"groups":            toAnyMap(plan.Groups),
					"priority_sections": toAnyMap(plan.PrioritySections),
					"phase_sections":    toAnyMap(plan.PhaseSections),
					"steps":             steps,
				},
			}); err != nil {
				return err
			}
		case protocolws.MethodOrchestrationPlanOverview:
			overviewPayload := protocolws.OrchestrationEvaluatePayload{}
			if raw, err := json.Marshal(inbound.Payload); err == nil {
				_ = json.Unmarshal(raw, &overviewPayload)
			}
			targetSession, resolveErr := s.resolveSessionForStatus(client, protocolws.SessionStatusPayload{
				SessionID:  overviewPayload.SessionID,
				SessionKey: overviewPayload.SessionKey,
			})
			if resolveErr != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, resolveErr.Error())); err != nil {
					return err
				}
				continue
			}
			overview := s.coordinator.PlanExecutionOverview(targetSession.ID, orchestration.SuggestionFilter{
				Category:     overviewPayload.Category,
				Priority:     overviewPayload.Priority,
				BlockingOnly: overviewPayload.BlockingOnly,
			})
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"session_id":                targetSession.ID,
					"total_steps":               overview.TotalSteps,
					"completed_steps":           overview.CompletedSteps,
					"failed_steps":              overview.FailedSteps,
					"ready_steps":               overview.ReadySteps,
					"pending_steps":             overview.PendingSteps,
					"in_progress_steps":         overview.InProgressSteps,
					"blocking_steps":            overview.BlockingSteps,
					"active_steps":              overview.ActiveSteps,
					"terminal_steps":            overview.TerminalSteps,
					"progress_percent":          overview.ProgressPercent,
					"has_blocked_steps":         overview.HasBlockedSteps,
					"state_counts":              toAnyMap(overview.StateCounts),
					"latest_active_action":      overview.LatestActiveAction,
					"latest_ready_action":       overview.LatestReadyAction,
					"latest_in_progress_action": overview.LatestInProgressAction,
					"latest_pending_action":     overview.LatestPendingAction,
					"latest_terminal_action":    overview.LatestTerminalAction,
					"latest_completed_action":   overview.LatestCompletedAction,
					"latest_failed_action":      overview.LatestFailedAction,
					"latest_blocked_action":     overview.LatestBlockedAction,
					"last_updated_at":           overview.LastUpdatedAt.Format(time.RFC3339Nano),
				},
			}); err != nil {
				return err
			}
		case protocolws.MethodOrchestrationPlanExecutionHistory:
			historyPayload := protocolws.OrchestrationPlanExecutionHistoryPayload{}
			if raw, err := json.Marshal(inbound.Payload); err == nil {
				_ = json.Unmarshal(raw, &historyPayload)
			}
			targetSession, resolveErr := s.resolveSessionForStatus(client, protocolws.SessionStatusPayload{
				SessionID:  historyPayload.SessionID,
				SessionKey: historyPayload.SessionKey,
			})
			if resolveErr != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, resolveErr.Error())); err != nil {
					return err
				}
				continue
			}
			var since time.Time
			if value := strings.TrimSpace(historyPayload.Since); value != "" {
				parsed, err := time.Parse(time.RFC3339Nano, value)
				if err != nil {
					if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, "invalid since timestamp")); err != nil {
						return err
					}
					continue
				}
				since = parsed
			}
			var until time.Time
			if value := strings.TrimSpace(historyPayload.Until); value != "" {
				parsed, err := time.Parse(time.RFC3339Nano, value)
				if err != nil {
					if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, "invalid until timestamp")); err != nil {
						return err
					}
					continue
				}
				until = parsed
			}
			history := s.coordinator.FilteredSessionPlanExecutionHistory(
				targetSession.ID,
				strings.TrimSpace(historyPayload.State),
				strings.TrimSpace(historyPayload.ActionID),
				since,
				until,
			)
			items := make([]map[string]any, 0, len(history))
			for _, record := range history {
				items = append(items, map[string]any{
					"session_id":  record.SessionID,
					"action_id":   record.ActionID,
					"state":       record.State,
					"result":      record.Result,
					"recorded_at": record.RecordedAt.Format(time.RFC3339Nano),
				})
			}
			summary := orchestration.SummarizeSessionPlanExecutionHistory(history)
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"session_id": targetSession.ID,
					"history":    items,
					"summary": map[string]any{
						"record_count":              summary.RecordCount,
						"state_counts":              toAnyMap(summary.StateCounts),
						"action_counts":             toAnyMap(summary.ActionCounts),
						"latest_recorded_action":    summary.LatestRecordedAction,
						"latest_recorded_state":     summary.LatestRecordedState,
						"latest_recorded_result":    summary.LatestRecordedResult,
						"latest_active_action":      summary.LatestActiveAction,
						"latest_active_state":       summary.LatestActiveState,
						"latest_active_at":          formatOptionalTime(summary.LatestActiveAt),
						"latest_active_result":      summary.LatestActiveResult,
						"latest_ready_action":       summary.LatestReadyAction,
						"latest_ready_at":           formatOptionalTime(summary.LatestReadyAt),
						"latest_ready_result":       summary.LatestReadyResult,
						"latest_ready_state":        summary.LatestReadyState,
						"latest_in_progress_action": summary.LatestInProgressAction,
						"latest_in_progress_at":     formatOptionalTime(summary.LatestInProgressAt),
						"latest_in_progress_result": summary.LatestInProgressResult,
						"latest_in_progress_state":  summary.LatestInProgressState,
						"latest_pending_action":     summary.LatestPendingAction,
						"latest_pending_at":         formatOptionalTime(summary.LatestPendingAt),
						"latest_pending_result":     summary.LatestPendingResult,
						"latest_pending_state":      summary.LatestPendingState,
						"latest_terminal_action":    summary.LatestTerminalAction,
						"latest_terminal_state":     summary.LatestTerminalState,
						"latest_terminal_result":    summary.LatestTerminalResult,
						"latest_terminal_at":        formatOptionalTime(summary.LatestTerminalAt),
						"latest_completed_action":   summary.LatestCompletedAction,
						"latest_completed_state":    summary.LatestCompletedState,
						"latest_completed_result":   summary.LatestCompletedResult,
						"latest_completed_at":       formatOptionalTime(summary.LatestCompletedAt),
						"latest_failed_action":      summary.LatestFailedAction,
						"latest_failed_state":       summary.LatestFailedState,
						"latest_failed_result":      summary.LatestFailedResult,
						"latest_failed_at":          formatOptionalTime(summary.LatestFailedAt),
						"state":                     strings.TrimSpace(historyPayload.State),
						"action_id":                 strings.TrimSpace(historyPayload.ActionID),
						"last_recorded_at":          summary.LastRecordedAt.Format(time.RFC3339Nano),
						"since":                     strings.TrimSpace(historyPayload.Since),
						"until":                     strings.TrimSpace(historyPayload.Until),
					},
				},
			}); err != nil {
				return err
			}
		case protocolws.MethodOrchestrationPlanStepUpdate:
			updatePayload := protocolws.OrchestrationPlanStepUpdatePayload{}
			if raw, err := json.Marshal(inbound.Payload); err == nil {
				_ = json.Unmarshal(raw, &updatePayload)
			}
			targetSession, resolveErr := s.resolveSessionForStatus(client, protocolws.SessionStatusPayload{
				SessionID:  updatePayload.SessionID,
				SessionKey: updatePayload.SessionKey,
			})
			if resolveErr != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, resolveErr.Error())); err != nil {
					return err
				}
				continue
			}
			step, err := s.coordinator.UpdatePlanStep(targetSession.ID, strings.TrimSpace(updatePayload.ActionID), strings.TrimSpace(updatePayload.State), updatePayload.Result)
			if err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"session_id": targetSession.ID,
					"step":       orchestrationPlanStepPayload(step),
				},
			}); err != nil {
				return err
			}
			if err := client.WriteJSON(protocolws.EventMessage(protocolws.EventOrchestrationPlanStepUpdated, map[string]any{
				"session_id": targetSession.ID,
				"step":       orchestrationPlanStepPayload(step),
			})); err != nil {
				return err
			}
		case protocolws.MethodOrchestrationPlanStepHistory:
			historyPayload := protocolws.OrchestrationPlanStepHistoryPayload{}
			if raw, err := json.Marshal(inbound.Payload); err == nil {
				_ = json.Unmarshal(raw, &historyPayload)
			}
			targetSession, resolveErr := s.resolveSessionForStatus(client, protocolws.SessionStatusPayload{
				SessionID:  historyPayload.SessionID,
				SessionKey: historyPayload.SessionKey,
			})
			if resolveErr != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, resolveErr.Error())); err != nil {
					return err
				}
				continue
			}
			if strings.TrimSpace(historyPayload.ActionID) == "" {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, "orchestration_plan_step_history payload requires action_id")); err != nil {
					return err
				}
				continue
			}
			history := s.coordinator.FilteredPlanStepHistory(targetSession.ID, strings.TrimSpace(historyPayload.ActionID), strings.TrimSpace(historyPayload.State))
			summary := orchestration.SummarizePlanStepHistory(history)
			items := make([]map[string]any, 0, len(history))
			for _, record := range history {
				items = append(items, map[string]any{
					"session_id":  record.SessionID,
					"action_id":   record.ActionID,
					"state":       record.State,
					"result":      record.Result,
					"recorded_at": record.RecordedAt.Format(time.RFC3339Nano),
				})
			}
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"session_id": targetSession.ID,
					"action_id":  strings.TrimSpace(historyPayload.ActionID),
					"history":    items,
					"summary": map[string]any{
						"record_count": summary.RecordCount,
						"state_counts": toAnyMap(summary.StateCounts),
						"latest_state": summary.LatestState,
						"state":        strings.TrimSpace(historyPayload.State),
					},
				},
			}); err != nil {
				return err
			}
		case protocolws.MethodSessionSetPermission:
			setPayload, err := parseSessionSetPermissionPayload(inbound.Payload)
			if err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}
			targetSession, resolveErr := s.resolveSessionForStatus(client, protocolws.SessionStatusPayload{
				SessionID:  setPayload.SessionID,
				SessionKey: setPayload.SessionKey,
			})
			if resolveErr != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, resolveErr.Error())); err != nil {
					return err
				}
				continue
			}
			currentPolicy := s.runner.PermissionPolicyForSession(targetSession.ID)
			updatedPolicy := currentPolicy
			updatedPolicy.Mode = permissions.Mode(setPayload.Mode)
			updatedPolicy.SubagentMode = ""
			if setPayload.SubagentMode != "" {
				updatedPolicy.SubagentMode = permissions.Mode(setPayload.SubagentMode)
			}
			if setPayload.PlanMode != nil {
				updatedPolicy.PlanMode = *setPayload.PlanMode
			}
			if setPayload.AutoMode != nil {
				updatedPolicy.AutoMode = *setPayload.AutoMode
			}
			if len(setPayload.WorkspaceRoots) > 0 {
				updatedPolicy.WorkspaceRoots = append([]string(nil), setPayload.WorkspaceRoots...)
			}
			updatedPolicy, err = permissions.SetupPolicy(updatedPolicy)
			if err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}
			s.runner.SetSessionPermissionPolicy(targetSession.ID, updatedPolicy, setPayload.CascadeSubagents)
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"session_id":        targetSession.ID,
					"session_key":       targetSession.Key,
					"permission_mode":   string(updatedPolicy.Mode),
					"subagent_mode":     string(updatedPolicy.SubagentMode),
					"plan_mode":         updatedPolicy.PlanMode,
					"auto_mode":         updatedPolicy.AutoMode,
					"workspace_roots":   toAnySlice(updatedPolicy.WorkspaceRoots),
					"cascade_subagents": setPayload.CascadeSubagents,
				},
			}); err != nil {
				return err
			}
		case protocolws.MethodSessionSetModel:
			setPayload, err := parseSessionSetModelPayload(inbound.Payload)
			if err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}
			targetSession, resolveErr := s.resolveSessionForStatus(client, protocolws.SessionStatusPayload{
				SessionID:  setPayload.SessionID,
				SessionKey: setPayload.SessionKey,
			})
			if resolveErr != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, resolveErr.Error())); err != nil {
					return err
				}
				continue
			}
			if strings.EqualFold(strings.TrimSpace(setPayload.Model), "default") {
				if err := s.runner.ClearSessionMainLoopModelOverride(targetSession.ID); err != nil {
					if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
						return err
					}
					continue
				}
			} else {
				if err := s.runner.SetSessionMainLoopModelOverride(targetSession.ID, setPayload.Model); err != nil {
					if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
						return err
					}
					continue
				}
			}
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"session_id":                       targetSession.ID,
					"session_key":                      targetSession.Key,
					"main_loop_model":                  s.runner.BaseMainLoopModelForSession(targetSession.ID),
					"session_main_loop_model_override": s.runner.SessionMainLoopModelOverride(targetSession.ID),
					"resolved_main_loop_model":         s.runner.ResolvedMainLoopModelForSession(targetSession.ID),
				},
			}); err != nil {
				return err
			}
		case protocolws.MethodTasksList, protocolws.MethodSubagentList:
			runs := s.runner.AgentManager().List()
			items := make([]map[string]any, 0, len(runs))
			for _, run := range runs {
				items = append(items, subagentPayload(run))
			}
			key := "tasks"
			if inbound.Method == protocolws.MethodSubagentList {
				key = "subagents"
			}
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					key: items,
				},
			}); err != nil {
				return err
			}
		case protocolws.MethodSubagentStatus:
			runID, _ := inbound.Payload["run_id"].(string)
			if strings.TrimSpace(runID) == "" {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, "subagent_status payload requires run_id")); err != nil {
					return err
				}
				continue
			}
			run, ok := s.runner.AgentManager().Get(runID)
			if !ok {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, "run not found")); err != nil {
					return err
				}
				continue
			}
			if err := client.WriteJSON(protocolws.Message{
				Type:    protocolws.TypeResponse,
				ID:      inbound.ID,
				OK:      true,
				Payload: subagentPayload(run),
			}); err != nil {
				return err
			}
		case protocolws.MethodSubagentStop:
			runID, _ := inbound.Payload["run_id"].(string)
			if strings.TrimSpace(runID) == "" {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, "subagent_stop payload requires run_id")); err != nil {
					return err
				}
				continue
			}
			if err := s.runner.AgentManager().Stop(runID); err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}
			updated, _ := s.runner.AgentManager().Get(runID)
			if err := client.WriteJSON(protocolws.Message{
				Type:    protocolws.TypeResponse,
				ID:      inbound.ID,
				OK:      true,
				Payload: subagentPayload(updated),
			}); err != nil {
				return err
			}
			if err := client.WriteJSON(protocolws.EventMessage(protocolws.EventSubagentUpdated, subagentPayload(updated))); err != nil {
				return err
			}
			if err := s.emitOrchestratorEvent(context.Background(), orchestration.Event{Type: protocolws.EventSubagentUpdated, SessionID: client.SessionID(), RunID: runID, Status: string(updated.Status), Action: string(updated.LastAction)}); err != nil {
				return err
			}
			if err := s.emitOrchestrationUpdated(client, runID); err != nil {
				return err
			}
		case protocolws.MethodMemoryList:
			memories := s.runner.MemoryService().List(client.SessionID())
			items := make([]map[string]any, 0, len(memories))
			for _, memory := range memories {
				items = append(items, map[string]any{
					"id":         memory.ID,
					"type":       string(memory.Type),
					"session_id": memory.SessionID,
					"agent_id":   memory.AgentID,
					"content":    memory.Content,
				})
			}
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"memories": items,
				},
			}); err != nil {
				return err
			}
		case protocolws.MethodApprovalList:
			approvalListPayload := protocolws.ApprovalListPayload{}
			if raw, err := json.Marshal(inbound.Payload); err == nil {
				_ = json.Unmarshal(raw, &approvalListPayload)
			}
			approvals := s.runner.ApprovalManager().ListBySessionAndStatus(client.SessionID(), approval.Status(approvalListPayload.Status))
			items := make([]map[string]any, 0, len(approvals))
			for _, approval := range approvals {
				items = append(items, map[string]any{
					"id":                approval.ID,
					"session_id":        approval.SessionID,
					"run_id":            approval.RunID,
					"tool_name":         approval.ToolName,
					"tool_input":        approval.ToolInput,
					"tool_input_object": approval.ToolInputObject,
					"reason":            approval.Reason,
					"decision_reason":   approval.DecisionReason,
					"accept_feedback":   approval.AcceptFeedback,
					"content_blocks":    approval.ContentBlocks,
					"status":            string(approval.Status),
				})
			}
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"approvals": items,
				},
			}); err != nil {
				return err
			}
		case protocolws.MethodApprovalApprove, protocolws.MethodApprovalReject:
			approvalID, _ := inbound.Payload["approval_id"].(string)
			if strings.TrimSpace(approvalID) == "" {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, "approval decision payload requires approval_id")); err != nil {
					return err
				}
				continue
			}
			status := approval.StatusApproved
			if inbound.Method == protocolws.MethodApprovalReject {
				status = approval.StatusRejected
			}
			contentBlocks, err := decodeContentBlockMaps(inbound.Payload["content_blocks"])
			if err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}
			if inbound.Method == protocolws.MethodApprovalApprove {
				acceptFeedback, _ := inbound.Payload["accept_feedback"].(string)
				if strings.TrimSpace(acceptFeedback) != "" || contentBlocks != nil {
					if _, err := s.runner.UpdateApprovalPromptMetadata(approvalID, strings.TrimSpace(acceptFeedback), contentBlocks); err != nil {
						if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
							return err
						}
						continue
					}
				}
			}
			rejectFeedback, _ := inbound.Payload["reject_feedback"].(string)
			if rejectFeedback == "" {
				rejectFeedback, _ = inbound.Payload["feedback"].(string)
			}
			updated, err := s.runner.UpdateApprovalStatus(approvalID, status)
			if err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"approval_id": updated.ID,
					"status":      string(updated.Status),
				},
			}); err != nil {
				return err
			}
			if err := client.WriteJSON(protocolws.EventMessage("approval.updated", map[string]any{
				"approval_id": updated.ID,
				"session_id":  updated.SessionID,
				"run_id":      updated.RunID,
				"status":      string(updated.Status),
			})); err != nil {
				return err
			}
			if inbound.Method == protocolws.MethodApprovalApprove {
				go func() {
					if err := s.runner.ApproveAndContinue(context.Background(), approvalID, runtimeSink{client: client}); err != nil {
						if shouldSuppressContinuationRunError(err) {
							return
						}
						_ = client.WriteJSON(protocolws.EventMessage("run.error", map[string]any{
							"run_id":     updated.RunID,
							"session_id": updated.SessionID,
							"message":    err.Error(),
						}))
					}
				}()
			} else if strings.TrimSpace(rejectFeedback) != "" || contentBlocks != nil {
				go func() {
					if err := s.runner.RejectAndContinue(context.Background(), approvalID, rejectFeedback, contentBlocks, runtimeSink{client: client}); err != nil {
						if shouldSuppressContinuationRunError(err) {
							return
						}
						_ = client.WriteJSON(protocolws.EventMessage("run.error", map[string]any{
							"run_id":     updated.RunID,
							"session_id": updated.SessionID,
							"message":    err.Error(),
						}))
					}
				}()
			}
		case protocolws.MethodApprovalClear:
			clearPayload := protocolws.ApprovalClearPayload{}
			if raw, err := json.Marshal(inbound.Payload); err == nil {
				_ = json.Unmarshal(raw, &clearPayload)
			}
			cleared := s.runner.ApprovalManager().ClearBySessionAndStatus(client.SessionID(), approval.Status(clearPayload.Status))
			if err := client.WriteJSON(protocolws.Message{
				Type: protocolws.TypeResponse,
				ID:   inbound.ID,
				OK:   true,
				Payload: map[string]any{
					"cleared": cleared,
				},
			}); err != nil {
				return err
			}
			if err := client.WriteJSON(protocolws.EventMessage("approval.cleared", map[string]any{
				"session_id": client.SessionID(),
				"status":     clearPayload.Status,
				"cleared":    cleared,
			})); err != nil {
				return err
			}
		case protocolws.MethodSubagentSteer:
			runID, _ := inbound.Payload["run_id"].(string)
			message, _ := inbound.Payload["message"].(string)
			if strings.TrimSpace(runID) == "" || strings.TrimSpace(message) == "" {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, "subagent_steer payload requires run_id and message")); err != nil {
					return err
				}
				continue
			}
			if err := s.runner.AgentManager().Steer(runID, message); err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}
			updated, _ := s.runner.AgentManager().Get(runID)
			if err := client.WriteJSON(protocolws.Message{
				Type:    protocolws.TypeResponse,
				ID:      inbound.ID,
				OK:      true,
				Payload: withSubagentMessage(subagentPayload(updated), message),
			}); err != nil {
				return err
			}
			if err := client.WriteJSON(protocolws.EventMessage(protocolws.EventSubagentUpdated, withSubagentMessage(subagentPayload(updated), message))); err != nil {
				return err
			}
			if err := s.emitOrchestratorEvent(context.Background(), orchestration.Event{Type: protocolws.EventSubagentUpdated, SessionID: client.SessionID(), RunID: runID, Status: string(updated.Status), Action: string(updated.LastAction), Message: message}); err != nil {
				return err
			}
			if err := s.emitOrchestrationUpdated(client, runID); err != nil {
				return err
			}
		case protocolws.MethodSubagentResume:
			runID, _ := inbound.Payload["run_id"].(string)
			promptText, _ := inbound.Payload["prompt"].(string)
			label, _ := inbound.Payload["label"].(string)
			if strings.TrimSpace(runID) == "" || strings.TrimSpace(promptText) == "" {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, "subagent_resume payload requires run_id and prompt")); err != nil {
					return err
				}
				continue
			}
			run, err := s.runner.ResumeSubagent(context.Background(), runID, label, promptText)
			if err != nil {
				if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, err.Error())); err != nil {
					return err
				}
				continue
			}
			if err := client.WriteJSON(protocolws.Message{
				Type:    protocolws.TypeResponse,
				ID:      inbound.ID,
				OK:      true,
				Payload: subagentPayload(*run),
			}); err != nil {
				return err
			}
			if err := client.WriteJSON(protocolws.EventMessage(protocolws.EventSubagentUpdated, subagentPayload(*run))); err != nil {
				return err
			}
			if err := s.emitOrchestratorEvent(context.Background(), orchestration.Event{Type: protocolws.EventSubagentUpdated, SessionID: client.SessionID(), RunID: run.ID, Status: string(run.Status), Action: string(run.LastAction)}); err != nil {
				return err
			}
			s.watchSubagentCompletion(client, sess, run.ID)
			if err := s.emitOrchestrationUpdated(client, run.ID); err != nil {
				return err
			}
		default:
			if err := client.WriteJSON(protocolws.ErrorResponse(inbound.ID, "unsupported method")); err != nil {
				return err
			}
		}
	}
}

func (s *Server) CheckPermission(ctx context.Context, request queryengine.PermissionHookRequest) (permissions.Decision, bool, error) {
	if client := s.permissionControlClient(request.Session.ID); client != nil {
		decision, decided, err := s.requestCanUseTool(ctx, client, request, s.fallbackPermissionHook)
		if err != nil {
			return decision, decided, err
		}
		if decided {
			return decision, true, nil
		}
	}

	if s.fallbackPermissionHook != nil {
		return s.fallbackPermissionHook.CheckPermission(ctx, request)
	}
	return permissions.Decision{}, false, nil
}

type permissionHookResult struct {
	decision permissions.Decision
	decided  bool
	err      error
}

func (s *Server) requestCanUseTool(ctx context.Context, client *Client, request queryengine.PermissionHookRequest, fallback queryengine.PermissionHook) (permissions.Decision, bool, error) {
	requestID := "control-" + formatID(s.nextID.Add(1))
	responseCh := client.RegisterControlRequest(requestID)
	defer client.CancelControlRequest(requestID)

	input := any(request.ToolInput)
	if request.ToolInputObject != nil {
		input = request.ToolInputObject
	}
	controlRequest := map[string]any{
		"subtype":     "can_use_tool",
		"tool_name":   request.ToolName,
		"input":       input,
		"tool_use_id": request.ToolUseID,
		"agent_id":    request.Session.AgentID,
	}
	if serializedReason := request.Decision.SerializedDecisionReason(); serializedReason != "" {
		controlRequest["decision_reason"] = serializedReason
	}
	if structuredReason := request.Decision.DecisionReason.Structured(); structuredReason != nil {
		controlRequest["decision_reason_details"] = structuredReason
	}
	payload := map[string]any{
		"request": controlRequest,
	}
	if err := client.WriteJSON(protocolws.Message{
		Type:    protocolws.TypeControlRequest,
		ID:      requestID,
		Payload: payload,
	}); err != nil {
		return permissions.Decision{}, false, err
	}

	var hookCh <-chan permissionHookResult
	if fallback != nil {
		ch := make(chan permissionHookResult, 1)
		hookCh = ch
		go func() {
			decision, decided, err := fallback.CheckPermission(ctx, request)
			ch <- permissionHookResult{decision: decision, decided: decided, err: err}
		}()
	}

	timeout := time.NewTimer(s.permissionControlTimeout)
	defer timeout.Stop()
	for {
		select {
		case <-ctx.Done():
			return permissions.Decision{}, false, ctx.Err()
		case <-timeout.C:
			return permissions.Decision{}, false, nil
		case result := <-hookCh:
			hookCh = nil
			if result.err != nil {
				return result.decision, result.decided, result.err
			}
			if result.decided {
				return result.decision, true, nil
			}
		case response := <-responseCh:
			return permissionDecisionFromControlResponse(response)
		}
	}
}

func permissionDecisionFromControlResponse(response protocolws.Message) (permissions.Decision, bool, error) {
	payload := response.Payload
	if nested, ok := payload["response"].(map[string]any); ok {
		payload = nested
	}
	behavior, _ := payload["behavior"].(string)
	switch strings.ToLower(strings.TrimSpace(behavior)) {
	case "allow":
		decision := permissions.Decision{
			Allowed: true,
			DecisionReason: permissions.DecisionReason{
				Type:     permissions.DecisionReasonHook,
				HookName: "PermissionRequest",
				Reason:   controlString(payload, "message", "reason"),
			},
		}
		if updatedInput, ok := payload["updated_input"]; ok {
			applyControlUpdatedInput(&decision, updatedInput)
		} else if updatedInput, ok := payload["updatedInput"]; ok {
			applyControlUpdatedInput(&decision, updatedInput)
		}
		if updates, ok := payload["updated_permissions"]; ok {
			decoded, err := decodePermissionUpdates(updates)
			if err != nil {
				return permissions.Decision{}, false, err
			}
			decision.UpdatedPermissions = decoded
		} else if updates, ok := payload["updatedPermissions"]; ok {
			decoded, err := decodePermissionUpdates(updates)
			if err != nil {
				return permissions.Decision{}, false, err
			}
			decision.UpdatedPermissions = decoded
		}
		return decision, true, nil
	case "deny":
		reason := controlString(payload, "message", "reason")
		if strings.TrimSpace(reason) == "" {
			reason = "Permission denied by can_use_tool host"
		}
		return permissions.Decision{
			Reason: reason,
			DecisionReason: permissions.DecisionReason{
				Type:     permissions.DecisionReasonHook,
				HookName: "PermissionRequest",
				Reason:   reason,
			},
		}, true, nil
	default:
		return permissions.Decision{}, false, nil
	}
}

func applyControlUpdatedInput(decision *permissions.Decision, updatedInput any) {
	switch typed := updatedInput.(type) {
	case string:
		decision.UpdatedInput = typed
	case map[string]any:
		decision.UpdatedInputObject = typed
	}
}

func decodePermissionUpdates(value any) ([]permissions.PermissionUpdate, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var updates []permissions.PermissionUpdate
	if err := json.Unmarshal(raw, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func decodeContentBlockMaps(value any) ([]map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, err
	}
	return cloneAnyMaps(blocks), nil
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func cloneAnyMaps(input []map[string]any) []map[string]any {
	if input == nil {
		return nil
	}
	cloned := make([]map[string]any, 0, len(input))
	for _, item := range input {
		cloned = append(cloned, cloneAnyMap(item))
	}
	return cloned
}

func controlString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, _ := payload[key].(string); strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

type runtimeSink struct {
	client *Client
}

func (s runtimeSink) Emit(event runtime.RuntimeEvent) error {
	switch event.Type {
	case "model.request.start", "model.request.end":
		return s.client.WriteJSON(protocolws.EventMessage(event.Type, map[string]any{
			"run_id":      event.RunID,
			"session_id":  event.Session.ID,
			"session_key": event.Session.Key,
			"agent_id":    event.Session.AgentID,
		}))
	case "agent.lifecycle.start", "agent.lifecycle.end":
		return s.client.WriteJSON(protocolws.EventMessage(event.Type, map[string]any{
			"run_id":      event.RunID,
			"session_id":  event.Session.ID,
			"session_key": event.Session.Key,
			"agent_id":    event.Session.AgentID,
		}))
	case "message.created":
		if event.Message == nil {
			return nil
		}
		return s.client.WriteJSON(protocolws.EventMessage(protocolws.EventMessageCreated, map[string]any{
			"session_id":  event.Session.ID,
			"session_key": event.Session.Key,
			"message": map[string]any{
				"id":         event.Message.ID,
				"role":       event.Message.Role,
				"content":    event.Message.Content,
				"created_at": event.Message.CreatedAt.Format(time.RFC3339Nano),
			},
		}))
	case "assistant.delta":
		return s.client.WriteJSON(protocolws.EventMessage("assistant.delta", map[string]any{
			"run_id":      event.RunID,
			"session_id":  event.Session.ID,
			"session_key": event.Session.Key,
			"delta":       event.Delta,
		}))
	case "tool.called":
		return s.client.WriteJSON(protocolws.EventMessage("tool.called", map[string]any{
			"run_id":              event.RunID,
			"session_id":          event.Session.ID,
			"session_key":         event.Session.Key,
			"tool_use_id":         event.ToolUseID,
			"provider_message_id": event.ProviderMessageID,
			"tool_name":           event.ToolName,
			"tool_input":          event.ToolInput,
			"tool_input_object":   event.ToolInputObject,
		}))
	case "tool.progress":
		if event.Progress == nil {
			return nil
		}
		return s.client.WriteJSON(protocolws.EventMessage("tool.progress", map[string]any{
			"run_id":              event.RunID,
			"session_id":          event.Session.ID,
			"session_key":         event.Session.Key,
			"tool_name":           event.ToolName,
			"tool_use_id":         event.Progress.ToolUseID,
			"provider_message_id": event.ProviderMessageID,
			"type":                event.Progress.Type,
			"message":             event.Progress.Message,
			"data":                event.Progress.Data,
		}))
	case "tool.result":
		if event.Message == nil {
			return nil
		}
		payload := map[string]any{
			"run_id":              event.RunID,
			"session_id":          event.Session.ID,
			"session_key":         event.Session.Key,
			"tool_name":           event.ToolName,
			"tool_use_id":         event.ToolUseID,
			"provider_message_id": event.ProviderMessageID,
			"tool_input":          event.ToolInput,
			"tool_input_object":   event.ToolInputObject,
			"message": map[string]any{
				"id":         event.Message.ID,
				"role":       event.Message.Role,
				"content":    event.Message.Content,
				"created_at": event.Message.CreatedAt.Format(time.RFC3339Nano),
			},
		}
		if event.StructuredContent != nil {
			payload["structured_content"] = event.StructuredContent
		}
		if event.Meta != nil {
			payload["meta"] = event.Meta
		}
		return s.client.WriteJSON(protocolws.EventMessage("tool.result", payload))
	case "run.error":
		return s.client.WriteJSON(protocolws.EventMessage("run.error", map[string]any{
			"run_id":      event.RunID,
			"session_id":  event.Session.ID,
			"session_key": event.Session.Key,
			"message":     event.Error,
		}))
	case "permission.required":
		payload := map[string]any{
			"run_id":            event.RunID,
			"session_id":        event.Session.ID,
			"session_key":       event.Session.Key,
			"tool_name":         event.ToolName,
			"tool_input":        event.ToolInput,
			"tool_input_object": event.ToolInputObject,
		}
		if event.Approval != nil {
			payload["approval_id"] = event.Approval.ID
			payload["reason"] = event.Approval.Reason
			payload["status"] = string(event.Approval.Status)
			if event.AcceptFeedback == "" {
				event.AcceptFeedback = event.Approval.AcceptFeedback
			}
			if event.ContentBlocks == nil {
				event.ContentBlocks = event.Approval.ContentBlocks
			}
		}
		if event.DecisionReason != "" {
			payload["decision_reason"] = event.DecisionReason
		}
		if event.DecisionReasonDetails != nil {
			payload["decision_reason_details"] = event.DecisionReasonDetails
		}
		if event.AcceptFeedback != "" {
			payload["accept_feedback"] = event.AcceptFeedback
		}
		if event.ContentBlocks != nil {
			payload["content_blocks"] = event.ContentBlocks
		}
		return s.client.WriteJSON(protocolws.EventMessage("permission.required", payload))
	default:
		return nil
	}
}

func parseConnectPayload(payload map[string]any) (protocolws.ConnectPayload, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return protocolws.ConnectPayload{}, err
	}

	var connectPayload protocolws.ConnectPayload
	if err := json.Unmarshal(raw, &connectPayload); err != nil {
		return protocolws.ConnectPayload{}, err
	}
	if connectPayload.Role == "" {
		return protocolws.ConnectPayload{}, &connectError{message: "connect payload requires role"}
	}
	if connectPayload.ClientIdentity == "" {
		return protocolws.ConnectPayload{}, &connectError{message: "connect payload requires client_identity"}
	}
	if connectPayload.AgentID == "" {
		connectPayload.AgentID = "main"
	}

	return connectPayload, nil
}

func parseSendMessagePayload(payload map[string]any) (protocolws.SendMessagePayload, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return protocolws.SendMessagePayload{}, err
	}

	var sendPayload protocolws.SendMessagePayload
	if err := json.Unmarshal(raw, &sendPayload); err != nil {
		return protocolws.SendMessagePayload{}, err
	}
	if sendPayload.Content == "" {
		return protocolws.SendMessagePayload{}, &connectError{message: "send_message payload requires content"}
	}

	return sendPayload, nil
}

func parseSpawnSubagentPayload(payload map[string]any) (protocolws.SpawnSubagentPayload, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return protocolws.SpawnSubagentPayload{}, err
	}

	var spawnPayload protocolws.SpawnSubagentPayload
	if err := json.Unmarshal(raw, &spawnPayload); err != nil {
		return protocolws.SpawnSubagentPayload{}, err
	}
	if strings.TrimSpace(spawnPayload.AgentType) == "" {
		if legacy, _ := payload["subagent_type"].(string); strings.TrimSpace(legacy) != "" {
			spawnPayload.AgentType = strings.TrimSpace(legacy)
		}
	}
	if spawnPayload.Prompt == "" {
		return protocolws.SpawnSubagentPayload{}, &connectError{message: "spawn_subagent payload requires prompt"}
	}
	return spawnPayload, nil
}

func parseSessionStatusPayload(payload map[string]any) (protocolws.SessionStatusPayload, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return protocolws.SessionStatusPayload{}, err
	}
	var statusPayload protocolws.SessionStatusPayload
	if err := json.Unmarshal(raw, &statusPayload); err != nil {
		return protocolws.SessionStatusPayload{}, err
	}
	return statusPayload, nil
}

func parseSessionNewPayload(payload map[string]any) (protocolws.SessionNewPayload, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return protocolws.SessionNewPayload{}, err
	}
	var newPayload protocolws.SessionNewPayload
	if err := json.Unmarshal(raw, &newPayload); err != nil {
		return protocolws.SessionNewPayload{}, err
	}
	return newPayload, nil
}

func parseSessionMessagesPayload(payload map[string]any) (protocolws.SessionMessagesPayload, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return protocolws.SessionMessagesPayload{}, err
	}
	var messagesPayload protocolws.SessionMessagesPayload
	if err := json.Unmarshal(raw, &messagesPayload); err != nil {
		return protocolws.SessionMessagesPayload{}, err
	}
	return messagesPayload, nil
}

func parseSessionDeletePayload(payload map[string]any) (protocolws.SessionDeletePayload, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return protocolws.SessionDeletePayload{}, err
	}
	var deletePayload protocolws.SessionDeletePayload
	if err := json.Unmarshal(raw, &deletePayload); err != nil {
		return protocolws.SessionDeletePayload{}, err
	}
	if strings.TrimSpace(deletePayload.SessionID) == "" && strings.TrimSpace(deletePayload.SessionKey) == "" {
		return protocolws.SessionDeletePayload{}, &connectError{message: "session_delete payload requires session_id or session_key"}
	}
	return deletePayload, nil
}

func parseMCPStatusPayload(payload map[string]any) (protocolws.MCPStatusPayload, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return protocolws.MCPStatusPayload{}, err
	}
	var statusPayload protocolws.MCPStatusPayload
	if err := json.Unmarshal(raw, &statusPayload); err != nil {
		return protocolws.MCPStatusPayload{}, err
	}
	return statusPayload, nil
}

func parseMCPActionPayload(payload map[string]any, method string) (protocolws.MCPActionPayload, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return protocolws.MCPActionPayload{}, err
	}
	var actionPayload protocolws.MCPActionPayload
	if err := json.Unmarshal(raw, &actionPayload); err != nil {
		return protocolws.MCPActionPayload{}, err
	}
	if strings.TrimSpace(actionPayload.Server) == "" {
		return protocolws.MCPActionPayload{}, &connectError{message: method + " payload requires server"}
	}
	return actionPayload, nil
}

func parseSessionSetPermissionPayload(payload map[string]any) (protocolws.SessionSetPermissionPayload, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return protocolws.SessionSetPermissionPayload{}, err
	}
	var setPayload protocolws.SessionSetPermissionPayload
	if err := json.Unmarshal(raw, &setPayload); err != nil {
		return protocolws.SessionSetPermissionPayload{}, err
	}
	if strings.TrimSpace(setPayload.Mode) == "" {
		return protocolws.SessionSetPermissionPayload{}, &connectError{message: "session_set_permission payload requires mode"}
	}
	return setPayload, nil
}

func parseSessionSetModelPayload(payload map[string]any) (protocolws.SessionSetModelPayload, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return protocolws.SessionSetModelPayload{}, err
	}
	var setPayload protocolws.SessionSetModelPayload
	if err := json.Unmarshal(raw, &setPayload); err != nil {
		return protocolws.SessionSetModelPayload{}, err
	}
	if strings.TrimSpace(setPayload.Model) == "" {
		return protocolws.SessionSetModelPayload{}, &connectError{message: "session_set_model payload requires model"}
	}
	return setPayload, nil
}

func mcpInventoryPayload(inventory runtime.MCPInventory) map[string]any {
	return map[string]any{
		"server_count":   inventory.ServerCount,
		"tool_count":     inventory.ToolCount,
		"prompt_count":   inventory.PromptCount,
		"resource_count": inventory.ResourceCount,
		"skill_count":    inventory.SkillCount,
	}
}

func sessionSummaryPayload(sess session.Session, messages []session.Message) map[string]any {
	lastActivity := sess.Metadata.LastActivityAt
	lastUserMessage := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if lastActivity.IsZero() || messages[i].CreatedAt.After(lastActivity) {
			lastActivity = messages[i].CreatedAt
		}
		if lastUserMessage == "" && messages[i].Role == "user" {
			lastUserMessage = messages[i].Content
		}
	}
	title := strings.TrimSpace(lastUserMessage)
	if title == "" {
		if sess.IsMain {
			title = "Main session"
		} else {
			title = "New chat"
		}
	}
	if titleRunes := []rune(title); len(titleRunes) > 64 {
		title = strings.TrimSpace(string(titleRunes[:64])) + "..."
	}
	payload := map[string]any{
		"session_id":        sess.ID,
		"session_key":       sess.Key,
		"agent_id":          sess.AgentID,
		"is_main":           sess.IsMain,
		"message_count":     len(messages),
		"last_user_message": lastUserMessage,
		"title":             title,
	}
	if !lastActivity.IsZero() {
		payload["last_activity_at"] = lastActivity.Format(time.RFC3339Nano)
	}
	return payload
}

func sessionMessagePayload(message session.Message) map[string]any {
	return map[string]any{
		"id":         message.ID,
		"role":       message.Role,
		"content":    message.Content,
		"created_at": message.CreatedAt.Format(time.RFC3339Nano),
	}
}

func mcpServerPayloads(servers []runtime.MCPServerSnapshot) []map[string]any {
	items := make([]map[string]any, 0, len(servers))
	for _, server := range servers {
		items = append(items, mcpServerPayload(server))
	}
	return items
}

func mcpServerPayload(server runtime.MCPServerSnapshot) map[string]any {
	payload := map[string]any{
		"name":           server.Name,
		"transport_type": server.TransportType,
		"endpoint":       server.Endpoint,
		"enabled":        server.Enabled,
		"status":         server.Status,
		"tools":          toAnySlice(server.Tools),
		"prompts":        toAnySlice(server.Prompts),
		"resources":      toAnySlice(server.Resources),
		"skills":         toAnySlice(server.Skills),
	}
	if strings.TrimSpace(server.AuthURL) != "" {
		payload["auth_url"] = server.AuthURL
	}
	if strings.TrimSpace(server.AuthMessage) != "" {
		payload["auth_message"] = server.AuthMessage
	}
	if strings.TrimSpace(server.AuthScope) != "" {
		payload["auth_scope"] = server.AuthScope
	}
	if strings.TrimSpace(server.AuthResourceMetadataURL) != "" {
		payload["auth_resource_metadata_url"] = server.AuthResourceMetadataURL
	}
	if len(server.AuthChallenge) > 0 {
		payload["auth_challenge"] = server.AuthChallenge
	}
	if strings.TrimSpace(server.Error) != "" {
		payload["error"] = server.Error
	}
	return payload
}

func mcpAuthStartPayload(result tools.MCPAuthStartResult) map[string]any {
	payload := map[string]any{
		"status": result.Status,
	}
	if strings.TrimSpace(result.AuthURL) != "" {
		payload["auth_url"] = result.AuthURL
	}
	if strings.TrimSpace(result.Message) != "" {
		payload["message"] = result.Message
	}
	if strings.TrimSpace(result.Scope) != "" {
		payload["scope"] = result.Scope
	}
	if strings.TrimSpace(result.ResourceMetadataURL) != "" {
		payload["resource_metadata_url"] = result.ResourceMetadataURL
	}
	if len(result.Challenge) > 0 {
		payload["challenge"] = result.Challenge
	}
	return payload
}

func filterMCPServerSnapshots(servers []runtime.MCPServerSnapshot, name string) ([]runtime.MCPServerSnapshot, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return servers, true
	}
	for _, server := range servers {
		if server.Name == name {
			return []runtime.MCPServerSnapshot{server}, true
		}
	}
	return nil, false
}

func (s *Server) resolveSessionForStatus(client *Client, payload protocolws.SessionStatusPayload) (session.Session, error) {
	if payload.SessionID != "" {
		if sess, ok := s.sessionManager.GetByID(payload.SessionID); ok {
			return sess, nil
		}
		return session.Session{}, &connectError{message: "session not found"}
	}
	if payload.SessionKey != "" {
		if sess, ok := s.sessionManager.GetByKey(payload.SessionKey); ok {
			return sess, nil
		}
		return session.Session{}, &connectError{message: "session not found"}
	}
	if client.SessionID() == "" {
		return session.Session{}, &connectError{message: "session id or session key is required"}
	}
	sess, ok := s.sessionManager.GetByID(client.SessionID())
	if !ok {
		return session.Session{}, &connectError{message: "session not found"}
	}
	return sess, nil
}

type connectError struct {
	message string
}

func (e *connectError) Error() string {
	return e.message
}

func (s *Server) readMessage(client *Client) (protocolws.Message, error) {
	var msg protocolws.Message
	if err := client.conn.ReadJSON(&msg); err != nil {
		return protocolws.Message{}, err
	}
	if msg.Payload == nil {
		msg.Payload = map[string]any{}
	}
	return msg, nil
}

func (s *Server) nextClientID() string {
	id := s.nextID.Add(1)
	return "conn-" + formatID(id)
}

func (s *Server) addClient(client *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.clients[client.ID()] = client
}

func (s *Server) permissionControlClient(sessionID string) *Client {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, client := range s.clients {
		if client.SessionID() == sessionID && client.SupportsPermissionControl() {
			return client
		}
	}
	return nil
}

func (s *Server) removeClient(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.clients, id)
}

func (s *Server) ActiveClients() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.clients)
}

func (s *Server) SessionManager() *session.Manager {
	return s.sessionManager
}

func toAnySlice(items []string) []any {
	values := make([]any, 0, len(items))
	for _, item := range items {
		values = append(values, item)
	}
	return values
}

func toAnyMap(items map[string]int) map[string]any {
	values := make(map[string]any, len(items))
	for key, value := range items {
		values[key] = value
	}
	return values
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339Nano)
}

func subagentPayload(run agent.Run) map[string]any {
	payload := map[string]any{
		"run_id":            run.ID,
		"parent_session_id": run.ParentSessionID,
		"label":             run.Label,
		"status":            string(run.Status),
		"child_session_id":  run.ChildSessionID,
		"child_session_key": run.ChildSessionKey,
		"attempt":           run.Attempt,
	}
	if run.LastAction != "" {
		payload["last_action"] = string(run.LastAction)
	}
	if run.Output != "" {
		payload["output"] = run.Output
	}
	if run.OutputFile != "" {
		payload["output_file"] = run.OutputFile
	}
	if run.ErrorSummary != "" {
		payload["error"] = run.ErrorSummary
	}
	if len(run.ControlMessages) > 0 {
		payload["control_messages"] = toAnySlice(run.ControlMessages)
	}
	if !run.CreatedAt.IsZero() {
		payload["created_at"] = run.CreatedAt.Format(time.RFC3339Nano)
	}
	if !run.StartedAt.IsZero() {
		payload["started_at"] = run.StartedAt.Format(time.RFC3339Nano)
	}
	if !run.UpdatedAt.IsZero() {
		payload["updated_at"] = run.UpdatedAt.Format(time.RFC3339Nano)
	}
	if !run.CompletedAt.IsZero() {
		payload["completed_at"] = run.CompletedAt.Format(time.RFC3339Nano)
	}
	if !run.LastActionAt.IsZero() {
		payload["last_action_at"] = run.LastActionAt.Format(time.RFC3339Nano)
	}
	return payload
}

func withSubagentMessage(payload map[string]any, message string) map[string]any {
	if strings.TrimSpace(message) == "" {
		return payload
	}
	withMessage := make(map[string]any, len(payload)+1)
	for key, value := range payload {
		withMessage[key] = value
	}
	withMessage["message"] = message
	return withMessage
}

func orchestrationPayload(run orchestration.RunState) map[string]any {
	return map[string]any{
		"run_id":             run.RunID,
		"session_id":         run.SessionID,
		"agent_id":           run.AgentID,
		"status":             run.Status,
		"last_action":        run.LastAction,
		"last_event":         run.LastEvent,
		"tool_name":          run.ToolName,
		"message":            run.Message,
		"dispatcher_state":   run.DispatcherState,
		"reviewer_state":     run.ReviewerState,
		"executor_state":     run.ExecutorState,
		"next_action":        run.NextAction,
		"recommended_role":   run.RecommendedRole,
		"recommended_action": run.RecommendedAction,
		"decision_type":      run.DecisionType,
		"decision_reason":    run.DecisionReason,
		"decision_priority":  run.DecisionPriority,
		"auto_executable":    run.AutoExecutable,
	}
}

func orchestrationHistoryPayload(record orchestration.DecisionRecord) map[string]any {
	return map[string]any{
		"run_id":             record.RunID,
		"session_id":         record.SessionID,
		"event_type":         record.EventType,
		"status":             record.Status,
		"last_action":        record.LastAction,
		"tool_name":          record.ToolName,
		"message":            record.Message,
		"recommended_role":   record.RecommendedRole,
		"recommended_action": record.RecommendedAction,
		"decision_type":      record.DecisionType,
		"decision_reason":    record.DecisionReason,
		"decision_priority":  record.DecisionPriority,
		"auto_executable":    record.AutoExecutable,
		"recorded_at":        record.RecordedAt.Format(time.RFC3339Nano),
	}
}

func orchestrationSuggestionPayload(suggestion orchestration.Suggestion) map[string]any {
	return map[string]any{
		"run_id":             suggestion.RunID,
		"session_id":         suggestion.SessionID,
		"category":           suggestion.Category,
		"suggested_action":   suggestion.SuggestedAction,
		"reason":             suggestion.Reason,
		"priority":           suggestion.Priority,
		"blocking":           suggestion.Blocking,
		"auto_executable":    suggestion.AutoExecutable,
		"recommended_role":   suggestion.RecommendedRole,
		"recommended_action": suggestion.RecommendedAction,
	}
}

func orchestrationPlanStepPayload(step orchestration.PlanStep) map[string]any {
	return map[string]any{
		"run_id":           step.RunID,
		"action_id":        step.ActionID,
		"title":            step.Title,
		"description":      step.Description,
		"action_kind":      step.ActionKind,
		"phase":            step.Phase,
		"depends_on":       step.DependsOn,
		"state":            step.State,
		"result":           step.Result,
		"updated_at":       step.UpdatedAt.Format(time.RFC3339Nano),
		"suggested_action": step.SuggestedAction,
		"priority":         step.Priority,
		"blocking":         step.Blocking,
		"recommended_role": step.RecommendedRole,
	}
}

func countBlockingSuggestions(suggestions []orchestration.Suggestion) int {
	total := 0
	for _, suggestion := range suggestions {
		if suggestion.Blocking {
			total++
		}
	}
	return total
}

func (s *Server) emitOrchestrationUpdated(client *Client, runID string) error {
	if client == nil || strings.TrimSpace(runID) == "" || s.coordinator == nil {
		return nil
	}
	state, ok := s.coordinator.GetRun(runID)
	if !ok {
		return nil
	}
	return client.WriteJSON(protocolws.EventMessage(protocolws.EventOrchestrationUpdated, orchestrationPayload(state)))
}

func (s *Server) emitOrchestratorEvent(ctx context.Context, event orchestration.Event) error {
	if s.orchestrator == nil {
		return nil
	}
	return s.orchestrator.Handle(ctx, event)
}

func (s *Server) watchSubagentCompletion(client *Client, sess session.Session, runID string) {
	if client == nil || strings.TrimSpace(runID) == "" {
		return
	}
	go func() {
		result, waitErr := s.runner.AgentManager().Wait(context.Background(), runID, 0)
		if waitErr != nil {
			_ = client.WriteJSON(protocolws.EventMessage(protocolws.EventSubagentCompleted, map[string]any{
				"run_id": runID,
				"status": string(agent.StatusFailed),
				"error":  waitErr.Error(),
			}))
			_ = s.emitOrchestratorEvent(context.Background(), orchestration.Event{
				Type:       protocolws.EventSubagentCompleted,
				SessionID:  sess.ID,
				SessionKey: sess.Key,
				AgentID:    sess.AgentID,
				RunID:      runID,
				Status:     string(agent.StatusFailed),
				Message:    waitErr.Error(),
			})
			return
		}
		_ = client.WriteJSON(protocolws.EventMessage(protocolws.EventSubagentCompleted, subagentPayload(result)))
		_ = s.emitOrchestratorEvent(context.Background(), orchestration.Event{
			Type:       protocolws.EventSubagentCompleted,
			SessionID:  sess.ID,
			SessionKey: sess.Key,
			AgentID:    sess.AgentID,
			RunID:      result.ID,
			Status:     string(result.Status),
			Action:     string(result.LastAction),
			Message:    result.Output,
		})
	}()
}

func (s *Server) reportToolProgress(progress tools.ToolProgress) {}

func defaultWorkspaceRoot() string {
	candidates := []string{
		filepath.Join("configs", "workspace"),
		filepath.Join("..", "..", "configs", "workspace"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return filepath.Join("configs", "workspace")
}
