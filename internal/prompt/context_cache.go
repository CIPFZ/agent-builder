package prompt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	"myclaw/internal/memory"
	"myclaw/internal/model"
	"myclaw/internal/session"
)

type ContextCache struct {
	mu      sync.Mutex
	entries map[string]Context
}

type CacheState struct {
	Key string
	Hit bool
}

func NewContextCache() *ContextCache {
	return &ContextCache{entries: make(map[string]Context)}
}

func (c *ContextCache) Build(input BuildInput) (Context, CacheState) {
	key := ContextCacheKey(input)
	if c == nil {
		return Build(input), CacheState{Key: key}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if cached, ok := c.entries[key]; ok {
		return cloneContext(cached), CacheState{Key: key, Hit: true}
	}
	built := Build(input)
	c.entries[key] = cloneContext(built)
	return built, CacheState{Key: key}
}

func ContextCacheKey(input BuildInput) string {
	h := sha256.New()
	writeField := func(name, value string) {
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write([]byte(strconv.Itoa(len(value))))
		h.Write([]byte{':'})
		h.Write([]byte(value))
		h.Write([]byte{0})
	}
	writeBool := func(name string, value bool) {
		writeField(name, strconv.FormatBool(value))
	}
	writeStringSlice := func(name string, values []string) {
		writeField(name+".len", strconv.Itoa(len(values)))
		for i, value := range values {
			writeField(name+"."+strconv.Itoa(i), value)
		}
	}
	writeAny := func(name string, value any) {
		if value == nil {
			writeField(name, "")
			return
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			writeField(name, "")
			return
		}
		writeField(name, string(encoded))
	}

	writeField("session.id", input.Session.ID)
	writeField("session.key", input.Session.Key)
	writeField("session.agent_id", input.Session.AgentID)
	writeBool("session.is_main", input.Session.IsMain)
	writeField("user_message.id", input.UserMessage.ID)
	writeField("user_message.role", input.UserMessage.Role)
	writeField("user_message.content", input.UserMessage.Content)

	writeField("workspace.root", input.WorkspaceContext.Root)
	writeField("workspace.fingerprint", input.WorkspaceContext.Fingerprint)
	writeField("workspace.files.len", strconv.Itoa(len(input.WorkspaceContext.Files)))
	for i, file := range input.WorkspaceContext.Files {
		prefix := "workspace.files." + strconv.Itoa(i)
		writeField(prefix+".name", file.Name)
		writeField(prefix+".path", file.Path)
		writeField(prefix+".type", file.Type)
		writeField(prefix+".content", file.Content)
		writeField(prefix+".hash", file.Hash)
		writeField(prefix+".size", strconv.FormatInt(file.Size, 10))
		writeField(prefix+".mod_time", file.ModTime.UTC().Format("2006-01-02T15:04:05.000000000Z07:00"))
		writeField(prefix+".fingerprint", file.Fingerprint)
	}

	projected := ProjectHistory(input.History, ProjectionOptions{CurrentUserMessageID: input.UserMessage.ID})
	writeField("history.projected.len", strconv.Itoa(len(projected)))
	for i, msg := range projected {
		prefix := "history.projected." + strconv.Itoa(i)
		writeField(prefix+".id", msg.ID)
		writeField(prefix+".role", msg.Role)
		writeField(prefix+".subtype", msg.Subtype)
		writeField(prefix+".content", msg.Content)
		writeField(prefix+".provider_message_id", msg.ProviderMessageID)
		writeField(prefix+".blocks.len", strconv.Itoa(len(msg.Blocks)))
		for j, block := range msg.Blocks {
			blockPrefix := prefix + ".blocks." + strconv.Itoa(j)
			writeField(blockPrefix+".type", string(block.Type))
			writeField(blockPrefix+".id", block.ID)
			writeField(blockPrefix+".tool_use_id", block.ToolUseID)
			writeField(blockPrefix+".text", block.Text)
			writeField(blockPrefix+".name", block.Name)
			writeField(blockPrefix+".input", block.Input)
			writeAny(blockPrefix+".input_object", block.InputObject)
			writeField(blockPrefix+".content", block.Content)
			writeBool(blockPrefix+".is_error", block.IsError)
		}
	}

	writeStringSlice("session_memories", input.SessionMemories)
	writeField("session_memory_items.len", strconv.Itoa(len(input.SessionMemoryItems)))
	for i, item := range input.SessionMemoryItems {
		prefix := "session_memory_items." + strconv.Itoa(i)
		writeField(prefix+".type", string(item.Type))
		writeField(prefix+".content", item.Content)
	}

	writeStringSlice("default_system_prompt", input.DefaultSystemPrompt)
	writeField("custom_system_prompt", input.CustomSystemPrompt)
	writeField("agent_system_prompt", input.AgentSystemPrompt)
	writeField("coordinator_system_prompt", input.CoordinatorSystemPrompt)
	writeBool("proactive_agent_prompt", input.ProactiveAgentPrompt)
	writeField("append_system_prompt", input.AppendSystemPrompt)
	writeField("override_system_prompt", input.OverrideSystemPrompt)
	writeStringSlice("user_context_lines", input.UserContextLines)
	writeStringSlice("system_context_lines", input.SystemContextLines)

	writeField("tools.len", strconv.Itoa(len(input.Tools)))
	for i, tool := range input.Tools {
		prefix := "tools." + strconv.Itoa(i)
		writeField(prefix+".name", tool.Name)
		writeField(prefix+".description", tool.Description)
		writeField(prefix+".source", tool.Source)
		writeField(prefix+".search_hint", tool.SearchHint)
		writeBool(prefix+".should_defer", tool.ShouldDefer)
		writeBool(prefix+".always_load", tool.AlwaysLoad)
	}

	return hex.EncodeToString(h.Sum(nil))
}

type ProjectionOptions struct {
	CurrentUserMessageID string
	LastSummaryID        string
}

func ProjectHistory(history []session.Message, options ProjectionOptions) []session.Message {
	start := 0
	if strings.TrimSpace(options.LastSummaryID) != "" {
		for i, msg := range history {
			if msg.ID == options.LastSummaryID {
				start = i
				break
			}
		}
	} else {
		for i := len(history) - 1; i >= 0; i-- {
			if history[i].Role == "summary" || history[i].IsCompactSummary {
				start = i
				break
			}
		}
	}

	projected := make([]session.Message, 0, len(history)-start)
	toolUseIDs := make(map[string]struct{})
	for _, msg := range history[start:] {
		if msg.ID == options.CurrentUserMessageID {
			continue
		}
		switch msg.Role {
		case "system", "attachment":
			continue
		case "tool":
			if len(msg.Blocks) > 0 {
				kept := false
				for _, block := range msg.Blocks {
					if _, ok := toolUseIDs[block.ToolUseID]; ok && block.ToolUseID != "" {
						kept = true
						break
					}
				}
				if !kept {
					continue
				}
			}
		}
		cloned := msg
		cloned.Blocks = append([]model.MessageBlock(nil), msg.Blocks...)
		projected = append(projected, cloned)
		for _, block := range msg.Blocks {
			if block.Type == model.MessageBlockToolUse && block.ID != "" {
				toolUseIDs[block.ID] = struct{}{}
			}
		}
	}
	return projected
}

func cloneContext(src Context) Context {
	out := src
	out.HistoryLines = append([]string(nil), src.HistoryLines...)
	out.UserContextLines = append([]string(nil), src.UserContextLines...)
	out.SystemContextLines = append([]string(nil), src.SystemContextLines...)
	out.WorkspaceLines = append([]string(nil), src.WorkspaceLines...)
	out.ToolLines = append([]string(nil), src.ToolLines...)
	out.MemoryLines = append([]string(nil), src.MemoryLines...)
	if src.MemoryByType != nil {
		out.MemoryByType = make(map[memory.Type][]string, len(src.MemoryByType))
		for key, values := range src.MemoryByType {
			out.MemoryByType[key] = append([]string(nil), values...)
		}
	}
	return out
}
