package runtime

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

const runtimeConversationV2SubscriberCap = 128

func (r *runtimeService) SessionConversationEventsV2(ctx context.Context, sessionID string, req RuntimeCanonicalConversationEventsRequestV2) (RuntimeCanonicalConversationEventsResponseV2, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return RuntimeCanonicalConversationEventsResponseV2{}, errors.New("session id is required")
	}
	if req.After == "" {
		req.After = "0"
	}
	after, err := strconv.ParseInt(req.After, 10, 64)
	if err != nil || after < 0 {
		return RuntimeCanonicalConversationEventsResponseV2{}, errors.New("canonical conversation after cursor must be a non-negative decimal int64")
	}
	if req.LimitRawEvents < 0 {
		return RuntimeCanonicalConversationEventsResponseV2{}, errors.New("canonical conversation raw event limit cannot be negative")
	}
	r.conversationV2Mu.Lock()
	defer r.conversationV2Mu.Unlock()
	store := newRuntimeConversationEventStoreV2(r.eventStore.db)
	checkpoint, reason, exists, err := store.checkpoint(ctx, sessionID)
	if err != nil {
		return RuntimeCanonicalConversationEventsResponseV2{}, err
	}
	resp := RuntimeCanonicalConversationEventsResponseV2{SchemaVersion: RuntimeConversationSchemaVersion, SessionID: sessionID, AfterCursor: req.After, Cursor: req.After, Events: []RuntimeConversationEntityEventV2{}}
	if !exists {
		resp.SnapshotRequired = true
		resp.Reason = "projector_uninitialized"
		return resp, nil
	}
	resp.Cursor = strconv.FormatInt(checkpoint, 10)
	if reason != "" {
		resp.SnapshotRequired = true
		resp.Reason = reason
		return resp, nil
	}
	if after > checkpoint {
		resp.SnapshotRequired = true
		resp.Reason = "invalid_cursor"
		return resp, nil
	}
	cursors, err := store.batchCursorsAfter(ctx, sessionID, after, req.LimitRawEvents)
	if err != nil {
		return RuntimeCanonicalConversationEventsResponseV2{}, err
	}
	if len(cursors) == 0 {
		if after != checkpoint {
			resp.SnapshotRequired = true
			resp.Reason = "projector_gap"
		}
		return resp, nil
	}
	expected := after
	for _, cursor := range cursors {
		if cursor.previous != expected {
			resp.SnapshotRequired = true
			resp.Reason = "projector_gap"
			resp.Cursor = strconv.FormatInt(expected, 10)
			return resp, nil
		}
		expected = cursor.sequence
	}
	through := cursors[len(cursors)-1].sequence
	events, err := store.listRange(ctx, sessionID, after, through)
	if err != nil {
		return RuntimeCanonicalConversationEventsResponseV2{}, err
	}
	resp.Events = events
	resp.Cursor = strconv.FormatInt(through, 10)
	return resp, nil
}

func (r *runtimeService) SubscribeSessionConversationEventsV2(ctx context.Context, sessionID, after string) (<-chan RuntimeCanonicalConversationEventBatchV2, func()) {
	streamCtx, cancel := context.WithCancel(ctx)
	out := make(chan RuntimeCanonicalConversationEventBatchV2, runtimeConversationV2SubscriberCap)
	rawEvents, unsubscribe := r.SubscribeEvents(streamCtx, parseCursorOrZero(after))
	done := func() { cancel(); unsubscribe() }
	go func() {
		defer close(out)
		cursor := after
		if cursor == "" {
			cursor = "0"
		}
		send := func(resp RuntimeCanonicalConversationEventsResponseV2) bool {
			batch := RuntimeCanonicalConversationEventBatchV2{SchemaVersion: resp.SchemaVersion, SessionID: resp.SessionID, AfterCursor: resp.AfterCursor, Cursor: resp.Cursor, Events: resp.Events, SnapshotRequired: resp.SnapshotRequired, Reason: resp.Reason}
			return sendCanonicalConversationBatchV2(streamCtx, out, batch, sessionID, cursor)
		}
		catchup, err := r.SessionConversationEventsV2(streamCtx, sessionID, RuntimeCanonicalConversationEventsRequestV2{After: cursor})
		if err != nil {
			return
		}
		if !send(catchup) {
			return
		}
		cursor = catchup.Cursor
		for {
			select {
			case raw, ok := <-rawEvents:
				if !ok {
					return
				}
				if raw.Type == "snapshot_required" {
					send(RuntimeCanonicalConversationEventsResponseV2{SchemaVersion: RuntimeConversationSchemaVersion, SessionID: sessionID, AfterCursor: cursor, Cursor: cursor, Events: []RuntimeConversationEntityEventV2{}, SnapshotRequired: true, Reason: "retention"})
					return
				}
				if raw.SessionID != sessionID {
					continue
				}
				resp, readErr := r.SessionConversationEventsV2(streamCtx, sessionID, RuntimeCanonicalConversationEventsRequestV2{After: cursor})
				if readErr != nil {
					return
				}
				if resp.Cursor == cursor && len(resp.Events) == 0 {
					continue
				}
				if !send(resp) {
					return
				}
				cursor = resp.Cursor
			case <-streamCtx.Done():
				return
			}
		}
	}()
	return out, done
}

func sendCanonicalConversationBatchV2(ctx context.Context, out chan RuntimeCanonicalConversationEventBatchV2, batch RuntimeCanonicalConversationEventBatchV2, sessionID, cursor string) bool {
	select {
	case out <- batch:
		return !batch.SnapshotRequired
	case <-ctx.Done():
		return false
	default:
		overflow := RuntimeCanonicalConversationEventBatchV2{SchemaVersion: RuntimeConversationSchemaVersion, SessionID: sessionID, AfterCursor: cursor, Cursor: cursor, Events: []RuntimeConversationEntityEventV2{}, SnapshotRequired: true, Reason: "overflow"}
		select {
		case <-out:
		default:
		}
		out <- overflow
		return false
	}
}

func parseCursorOrZero(v string) int64 { parsed, _ := strconv.ParseInt(v, 10, 64); return parsed }
