package runtime

import (
	"charm.land/fantasy"
)

// compactKeepTailMessages is the minimum number of trailing fantasy messages
// preserved verbatim when a full compact replaces the projection prefix with
// the compact summary.
const compactKeepTailMessages = 6

// pairSafeTailStart returns the index in messages where a pairing-safe tail
// begins. The tail keeps at least minTail messages from the end (ignoring
// trailing dangling tool calls) and is expanded backwards until every
// tool_result inside the tail has its originating tool_use message inside the
// tail as well. System messages are never used as a cut point: the returned
// index never points at a system message unless it is 0. Leading system
// messages (prompt prefix / system prompt) are never counted as tail
// candidates; if the tail would need to start inside the leading system
// block, the function degenerates to 0 (keep everything).
func pairSafeTailStart(messages []fantasy.Message, minTail int) int {
	if len(messages) == 0 {
		return 0
	}
	if minTail <= 0 {
		minTail = 1
	}
	floor := leadingSystemMessageCount(messages)
	if floor >= len(messages) {
		return 0
	}
	// Trailing assistant messages whose tool calls have no matching results
	// anywhere in the list do not count toward the minimum tail quota.
	effectiveEnd := len(messages)
	resultIDs := map[string]struct{}{}
	for _, msg := range messages {
		for _, id := range toolResultIDsOfMessage(msg) {
			resultIDs[id] = struct{}{}
		}
	}
	for effectiveEnd > floor {
		calls := toolCallIDsOfMessage(messages[effectiveEnd-1])
		if len(calls) == 0 {
			break
		}
		dangling := false
		for _, id := range calls {
			if _, ok := resultIDs[id]; !ok {
				dangling = true
				break
			}
		}
		if !dangling {
			break
		}
		effectiveEnd--
	}

	start := effectiveEnd - minTail
	if start < floor {
		start = floor
	}

	// Index of the message that introduced each tool call id.
	callIndex := map[string]int{}
	for i, msg := range messages {
		for _, id := range toolCallIDsOfMessage(msg) {
			if _, ok := callIndex[id]; !ok {
				callIndex[id] = i
			}
		}
	}

	for {
		adjusted := start
		// Every tool_result in the retained region must have its tool_use in
		// the retained region too. Orphan results (call never seen) are kept
		// as-is: there is nothing to pair them with.
		for i := adjusted; i < len(messages); i++ {
			for _, id := range toolResultIDsOfMessage(messages[i]) {
				if callAt, ok := callIndex[id]; ok && callAt < adjusted {
					adjusted = callAt
				}
			}
		}
		// Never cut directly at a system (or synthetic-system) message.
		for adjusted > floor && messages[adjusted].Role == fantasy.MessageRoleSystem {
			adjusted--
		}
		if adjusted == start {
			break
		}
		start = adjusted
	}
	if start <= floor {
		return 0
	}
	return start
}

// trimTrailingDanglingToolCalls drops trailing assistant messages whose tool
// calls have no matching tool results anywhere in the slice.
func trimTrailingDanglingToolCalls(messages []fantasy.Message) []fantasy.Message {
	if len(messages) == 0 {
		return messages
	}
	resultIDs := map[string]struct{}{}
	for _, msg := range messages {
		for _, id := range toolResultIDsOfMessage(msg) {
			resultIDs[id] = struct{}{}
		}
	}
	end := len(messages)
	for end > 0 {
		calls := toolCallIDsOfMessage(messages[end-1])
		if len(calls) == 0 {
			break
		}
		dangling := false
		for _, id := range calls {
			if _, ok := resultIDs[id]; !ok {
				dangling = true
				break
			}
		}
		if !dangling {
			break
		}
		end--
	}
	return messages[:end]
}

// leadingSystemMessageCount returns the number of consecutive system messages
// at the head of the list (prompt prefix, system prompt, memory injection).
func leadingSystemMessageCount(messages []fantasy.Message) int {
	count := 0
	for _, msg := range messages {
		if msg.Role != fantasy.MessageRoleSystem {
			break
		}
		count++
	}
	return count
}

func toolCallIDsOfMessage(msg fantasy.Message) []string {
	var ids []string
	for _, part := range msg.Content {
		if call, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part); ok && call.ToolCallID != "" {
			ids = append(ids, call.ToolCallID)
		}
	}
	return ids
}

func toolResultIDsOfMessage(msg fantasy.Message) []string {
	var ids []string
	for _, part := range msg.Content {
		if result, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part); ok && result.ToolCallID != "" {
			ids = append(ids, result.ToolCallID)
		}
	}
	return ids
}
