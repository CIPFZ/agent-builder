package runtime

import (
	"time"

	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

// runtimeMessageUpdateCoalesceWindow limits persisted message.updated write
// amplification. Ephemeral deltas still keep the canonical stream live.
const runtimeMessageUpdateCoalesceWindow = 250 * time.Millisecond

func (r *runtimeService) deriveOutputTextDeltasLocked(msg apitypes.Message, turnID string, now time.Time) []RuntimeEvent {
	if r == nil || r.messageStream == nil || msg.ID == "" || msg.Role != apitypes.Assistant {
		return nil
	}
	cursor, ok := r.messageStream[msg.ID]
	if !ok {
		cursor = &messageStreamCursor{sessionID: msg.SessionID}
		r.messageStream[msg.ID] = cursor
	}
	var events []RuntimeEvent
	text := msg.Content().Text
	if len(text) > cursor.lastTextLen {
		if delta := text[cursor.lastTextLen:]; delta != "" {
			events = append(events, newOutputTextDeltaEvent(now, msg.SessionID, msg.ID, turnID, "text", delta, len(text)))
		}
		cursor.lastTextLen = len(text)
	}
	thinking := msg.ReasoningContent().Thinking
	if len(thinking) > cursor.lastReasoningLen {
		if delta := thinking[cursor.lastReasoningLen:]; delta != "" {
			events = append(events, newOutputTextDeltaEvent(now, msg.SessionID, msg.ID, turnID, "reasoning", delta, len(thinking)))
		}
		cursor.lastReasoningLen = len(thinking)
	}
	if msg.IsFinished() {
		cursor.completed = true
	}
	return events
}

func (r *runtimeService) recordMessageEventWithCoalesceLocked(event RuntimeEvent, now time.Time) *RuntimeEvent {
	if event.Type != runtimeapi.EventMessageUpdated {
		if event.Type == runtimeapi.EventMessageCompleted || event.Type == runtimeapi.EventMessageCreated {
			if cursor, ok := r.messageStream[event.MessageID]; ok && cursor != nil {
				cursor.lastUpdateEmitted = now.UnixMilli()
				if event.Type == runtimeapi.EventMessageCompleted {
					cursor.completed = true
				}
			}
		}
		emitted := r.appendRuntimeEventLocked(event)
		return &emitted
	}
	cursor, ok := r.messageStream[event.MessageID]
	if !ok {
		cursor = &messageStreamCursor{sessionID: event.SessionID}
		r.messageStream[event.MessageID] = cursor
	}
	nowMs := now.UnixMilli()
	if cursor.lastUpdateEmitted != 0 && nowMs-cursor.lastUpdateEmitted < runtimeMessageUpdateCoalesceWindow.Milliseconds() {
		return nil
	}
	cursor.lastUpdateEmitted = nowMs
	emitted := r.appendRuntimeEventLocked(event)
	return &emitted
}

func newOutputTextDeltaEvent(createdAt time.Time, sessionID, messageID, turnID, partType, delta string, contentLen int) RuntimeEvent {
	event := runtimeapi.NewEvent(newRuntimeEventID(), runtimeapi.EventOutputTextDelta, createdAt)
	event.SessionID = sessionID
	event.MessageID = messageID
	event.TurnID = turnID
	event.Payload = map[string]any{"part_type": partType, "delta": delta, "content_len": contentLen}
	return event
}
