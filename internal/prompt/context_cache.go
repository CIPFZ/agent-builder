package prompt

import (
	"crypto/sha256"
	"encoding/hex"
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
	writePart := func(value string) {
		h.Write([]byte(value))
		h.Write([]byte{0})
	}
	writePart(input.Session.ID)
	writePart(input.UserMessage.ID)
	writePart(input.UserMessage.Content)
	writePart(input.WorkspaceContext.Fingerprint)
	for _, file := range input.WorkspaceContext.Files {
		writePart(file.Fingerprint)
	}
	for _, msg := range ProjectHistory(input.History, ProjectionOptions{CurrentUserMessageID: input.UserMessage.ID}) {
		writePart(msg.ID)
		writePart(msg.Role)
		writePart(msg.Content)
		for _, block := range msg.Blocks {
			writePart(string(block.Type))
			writePart(block.ID)
			writePart(block.ToolUseID)
			writePart(block.Name)
			writePart(block.Content)
		}
	}
	for _, value := range input.SessionMemories {
		writePart(value)
	}
	for _, item := range input.SessionMemoryItems {
		writePart(string(item.Type))
		writePart(item.Content)
	}
	for _, tool := range input.Tools {
		writePart(tool.Name)
		writePart(tool.Description)
		writePart(tool.Source)
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
