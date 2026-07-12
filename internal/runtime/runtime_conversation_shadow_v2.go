package runtime

import (
	"context"
	"fmt"
	"strings"
)

const (
	runtimeConversationModeLegacy    = "legacy"
	runtimeConversationModeShadow    = "canonical_v2_shadow"
	runtimeConversationModeCanonical = "canonical_v2"
)

func compareCanonicalConversationV2(c RuntimeCanonicalConversationSnapshot, l RuntimeOutputSnapshot) []RuntimeConversationV2Mismatch {
	out := []RuntimeConversationV2Mismatch{}
	add := func(kind, id, field, a, b string) {
		if a != b {
			out = append(out, RuntimeConversationV2Mismatch{SessionID: c.SessionID, Cursor: c.Cursor, EntityType: kind, EntityID: id, Field: field, Legacy: a, Canonical: b})
		}
	}
	legacyTurns := map[string]RuntimeTurn{}
	for _, v := range l.Turns {
		legacyTurns[v.ID] = v
	}
	for _, v := range c.Turns {
		old, ok := legacyTurns[v.ID]
		if !ok {
			add("turn", v.ID, "presence", "missing", "present")
			continue
		}
		add("turn", v.ID, "status", old.Status, v.Status)
		add("turn", v.ID, "finalMessageId", old.LatestAssistantMessageID, v.FinalMessageID)
	}
	legacyMessages := map[string]RuntimeMessage{}
	legacyMessagePhase := map[string]string{}
	for _, item := range l.Items {
		if item.MessageID != "" && item.Phase != "" {
			legacyMessagePhase[item.MessageID] = item.Phase
		}
	}
	for _, v := range l.Messages {
		legacyMessages[v.ID] = v
	}
	for _, v := range c.Messages {
		old, ok := legacyMessages[v.ID]
		if !ok {
			add("message", v.ID, "presence", "missing", "present")
			continue
		}
		add("message", v.ID, "content", old.Content, v.Content)
		add("message", v.ID, "turnId", runtimeOutputMessageTurnID(l.Items, v.ID), v.TurnID)
		if phase := legacyMessagePhase[v.ID]; phase != "" {
			add("message", v.ID, "phase", phase, v.Phase)
		}
	}
	legacyCalls := map[string]RuntimeToolCall{}
	for _, v := range l.ToolCalls {
		legacyCalls[v.ID] = v
	}
	for _, v := range c.ToolCalls {
		old, ok := legacyCalls[v.ID]
		if !ok {
			add("toolCall", v.ID, "presence", "missing", "present")
			continue
		}
		add("toolCall", v.ID, "status", old.Status, v.Status)
		add("toolCall", v.ID, "turnId", old.TurnID, v.TurnID)
		add("toolCall", v.ID, "resultIds", strings.Join(old.ResultIDs, ","), strings.Join(v.ResultIDs, ","))
	}
	legacySteps := map[string]RuntimeAssistantStep{}
	for _, v := range l.AssistantSteps {
		legacySteps[v.ID] = v
	}
	for _, v := range c.AssistantSteps {
		old, ok := legacySteps[v.ID]
		if !ok {
			add("assistantStep", v.ID, "presence", "missing", "present")
			continue
		}
		add("assistantStep", v.ID, "status", old.Status, v.Status)
	}
	legacyResults := map[string]RuntimeToolResult{}
	for _, v := range l.ToolResults {
		legacyResults[v.ID] = v
	}
	for _, v := range c.ToolResults {
		old, ok := legacyResults[v.ID]
		if !ok {
			add("toolResult", v.ID, "presence", "missing", "present")
			continue
		}
		add("toolResult", v.ID, "status", old.Status, v.Status)
	}
	legacyPermissions := map[string]RuntimePermissionRequest{}
	for _, v := range l.Permissions {
		legacyPermissions[v.ID] = v
	}
	for _, v := range c.Permissions {
		old, ok := legacyPermissions[v.ID]
		if !ok {
			add("permission", v.ID, "presence", "missing", "present")
			continue
		}
		add("permission", v.ID, "status", old.Status, v.Status)
	}
	legacyTasks := map[string]RuntimeAgentTask{}
	for _, v := range l.AgentTasks {
		legacyTasks[v.ID] = v
	}
	for _, v := range c.AgentTasks {
		old, ok := legacyTasks[v.ID]
		if !ok {
			add("agentTask", v.ID, "presence", "missing", "present")
			continue
		}
		add("agentTask", v.ID, "status", old.Status, v.Status)
	}
	if len(c.TodoPlans) > 0 {
		if l.Todos == nil {
			add("todoPlan", c.TodoPlans[0].ID, "presence", "missing", "present")
		} else {
			add("todoPlan", c.TodoPlans[0].ID, "itemCount", fmt.Sprint(len(l.Todos.Todos)), fmt.Sprint(len(c.TodoPlans[0].Items)))
		}
	}
	canonicalPresence := map[string]map[string]bool{"turn": {}, "message": {}, "toolCall": {}, "assistantStep": {}, "toolResult": {}, "permission": {}, "agentTask": {}}
	for _, v := range c.Turns {
		canonicalPresence["turn"][v.ID] = true
	}
	for _, v := range c.Messages {
		canonicalPresence["message"][v.ID] = true
	}
	for _, v := range c.ToolCalls {
		canonicalPresence["toolCall"][v.ID] = true
	}
	for _, v := range c.AssistantSteps {
		canonicalPresence["assistantStep"][v.ID] = true
	}
	for _, v := range c.ToolResults {
		canonicalPresence["toolResult"][v.ID] = true
	}
	for _, v := range c.Permissions {
		canonicalPresence["permission"][v.ID] = true
	}
	for _, v := range c.AgentTasks {
		canonicalPresence["agentTask"][v.ID] = true
	}
	for id := range legacyTurns {
		if !canonicalPresence["turn"][id] {
			add("turn", id, "presence", "present", "missing")
		}
	}
	for id := range legacyMessages {
		if !canonicalPresence["message"][id] {
			add("message", id, "presence", "present", "missing")
		}
	}
	for id := range legacyCalls {
		if !canonicalPresence["toolCall"][id] {
			add("toolCall", id, "presence", "present", "missing")
		}
	}
	for id := range legacySteps {
		if !canonicalPresence["assistantStep"][id] {
			add("assistantStep", id, "presence", "present", "missing")
		}
	}
	for id := range legacyResults {
		if !canonicalPresence["toolResult"][id] {
			add("toolResult", id, "presence", "present", "missing")
		}
	}
	for id := range legacyPermissions {
		if !canonicalPresence["permission"][id] {
			add("permission", id, "presence", "present", "missing")
		}
	}
	for id := range legacyTasks {
		if !canonicalPresence["agentTask"][id] {
			add("agentTask", id, "presence", "present", "missing")
		}
	}
	return out
}

func (r *runtimeService) runCanonicalConversationShadowV2(ctx context.Context, s RuntimeCanonicalConversationSnapshot) {
	if r.conversationMode != runtimeConversationModeShadow {
		return
	}
	legacy, err := r.SessionOutput(ctx, s.SessionID, RuntimeOutputRequest{Snapshot: true})
	if err != nil {
		r.conversationV2Mismatches = append(r.conversationV2Mismatches, RuntimeConversationV2Mismatch{SessionID: s.SessionID, Cursor: s.Cursor, EntityType: "snapshot", Field: "legacy_error", Legacy: err.Error(), Canonical: "available"})
		return
	}
	r.conversationV2Mismatches = append(r.conversationV2Mismatches, compareCanonicalConversationV2(s, legacy)...)
}

func (r *runtimeService) ConversationV2Diagnostics(ctx context.Context, sessionID string) (RuntimeConversationV2DiagnosticsResponse, error) {
	r.conversationV2Mu.Lock()
	defer r.conversationV2Mu.Unlock()
	items := []RuntimeConversationV2Mismatch{}
	for _, item := range r.conversationV2Mismatches {
		if sessionID == "" || item.SessionID == sessionID {
			items = append(items, item)
		}
	}
	return RuntimeConversationV2DiagnosticsResponse{Mode: r.conversationMode, Mismatches: items}, nil
}

func runtimeOutputMessageTurnID(items []RuntimeConversationItem, messageID string) string {
	for _, item := range items {
		if item.MessageID == messageID && item.TurnID != "" {
			return item.TurnID
		}
	}
	return ""
}
