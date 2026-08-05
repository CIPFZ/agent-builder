package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

const (
	// A queued item is independently capped at 2 MiB below, so the channel
	// capacity also imposes a 16 MiB hard upper bound on resident payloads.
	runtimeConversationV2SubscriberCap        = 8
	runtimeConversationV2DefaultRawEventLimit = 128
	runtimeConversationV2MaxRawEventLimit     = 256
	runtimeConversationV2MaxEntityCount       = 256
	runtimeConversationV2MaxEncodedBatchBytes = 2 * 1024 * 1024
	// Reserve room for the response and Wails stream envelopes. Stored event
	// JSON accounts for nearly all payload bytes, including UTF-8 content.
	runtimeConversationV2EnvelopeReserveBytes = 8 * 1024
	runtimeConversationV2DeltaFlushInterval   = time.Second
	runtimeConversationV2MaxPendingDeltas     = 128
	runtimeConversationV2MaxPendingDeltaBytes = 32 * 1024
)

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
	if req.LimitRawEvents == 0 {
		req.LimitRawEvents = runtimeConversationV2DefaultRawEventLimit
	} else if req.LimitRawEvents > runtimeConversationV2MaxRawEventLimit {
		req.LimitRawEvents = runtimeConversationV2MaxRawEventLimit
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
	floor, hasFloor, err := store.retentionFloor(ctx, sessionID)
	if err != nil {
		return RuntimeCanonicalConversationEventsResponseV2{}, err
	}
	if hasFloor && after < floor {
		resp.Cursor = req.After
		resp.SnapshotRequired = true
		resp.Reason = "retention"
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
	selected := make([]runtimeConversationBatchCursorV2, 0, len(cursors))
	entityCount := 0
	encodedBytes := int64(0)
	for _, cursor := range cursors {
		if cursor.previous != expected {
			resp.SnapshotRequired = true
			resp.Reason = "projector_gap"
			resp.Cursor = strconv.FormatInt(expected, 10)
			return resp, nil
		}
		if cursor.entityCount > runtimeConversationV2MaxEntityCount || cursor.encodedBytes > runtimeConversationV2MaxEncodedBatchBytes-runtimeConversationV2EnvelopeReserveBytes {
			if len(selected) == 0 {
				resp.SnapshotRequired = true
				resp.Reason = "batch_too_large"
				resp.Cursor = strconv.FormatInt(expected, 10)
				return resp, nil
			}
			break
		}
		if entityCount+cursor.entityCount > runtimeConversationV2MaxEntityCount || encodedBytes+cursor.encodedBytes > runtimeConversationV2MaxEncodedBatchBytes-runtimeConversationV2EnvelopeReserveBytes {
			break
		}
		selected = append(selected, cursor)
		entityCount += cursor.entityCount
		encodedBytes += cursor.encodedBytes
		expected = cursor.sequence
	}
	through := selected[len(selected)-1].sequence
	events, err := store.listRange(ctx, sessionID, after, through)
	if err != nil {
		return RuntimeCanonicalConversationEventsResponseV2{}, err
	}
	resp.Events = events
	resp.Cursor = strconv.FormatInt(through, 10)
	if encoded, marshalErr := json.Marshal(resp); marshalErr != nil || len(events) > runtimeConversationV2MaxEntityCount || len(encoded) > runtimeConversationV2MaxEncodedBatchBytes {
		resp.Events = []RuntimeConversationEntityEventV2{}
		resp.Cursor = req.After
		resp.SnapshotRequired = true
		resp.Reason = "batch_too_large"
	}
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
		firstCatchup := true
		for {
			catchup, err := r.SessionConversationEventsV2(streamCtx, sessionID, RuntimeCanonicalConversationEventsRequestV2{After: cursor})
			if err != nil {
				return
			}
			advanced := catchup.Cursor != cursor
			if firstCatchup || advanced || len(catchup.Events) > 0 || catchup.SnapshotRequired {
				if !send(catchup) {
					return
				}
			}
			firstCatchup = false
			cursor = catchup.Cursor
			if !advanced {
				break
			}
		}
		var pendingDeltas []RuntimeConversationTextDeltaV2
		pendingDeltaBytes := 0
		var deltaTimer *time.Timer
		var deltaTimerC <-chan time.Time
		stopDeltaTimer := func() {
			if deltaTimer != nil && !deltaTimer.Stop() {
				select {
				case <-deltaTimer.C:
				default:
				}
			}
			deltaTimer = nil
			deltaTimerC = nil
		}
		flushDeltas := func() bool {
			stopDeltaTimer()
			if len(pendingDeltas) == 0 {
				return true
			}
			batch := RuntimeCanonicalConversationEventBatchV2{SchemaVersion: RuntimeConversationSchemaVersion, SessionID: sessionID, AfterCursor: cursor, Cursor: cursor, Events: []RuntimeConversationEntityEventV2{}, Deltas: pendingDeltas}
			pendingDeltas = nil
			pendingDeltaBytes = 0
			return sendCanonicalConversationBatchV2(streamCtx, out, batch, sessionID, cursor)
		}
		for {
			select {
			case <-deltaTimerC:
				if !flushDeltas() {
					return
				}
			case raw, ok := <-rawEvents:
				if !ok {
					return
				}
				if raw.Type == "snapshot_required" {
					if !flushDeltas() {
						return
					}
					send(RuntimeCanonicalConversationEventsResponseV2{SchemaVersion: RuntimeConversationSchemaVersion, SessionID: sessionID, AfterCursor: cursor, Cursor: cursor, Events: []RuntimeConversationEntityEventV2{}, SnapshotRequired: true, Reason: "retention"})
					return
				}
				if raw.SessionID != sessionID {
					continue
				}
				if raw.Type == runtimeapi.EventOutputTextDelta {
					delta := RuntimeConversationTextDeltaV2{
						MessageID:     raw.MessageID,
						TurnID:        raw.TurnID,
						PartType:      stringFromMap(raw.Payload, "part_type"),
						Delta:         stringFromMap(raw.Payload, "delta"),
						ContentLength: intFromMap(raw.Payload, "content_len"),
						CreatedAt:     parseRuntimeEventMillis(raw.CreatedAt),
					}
					if delta.MessageID != "" && delta.Delta != "" && (delta.PartType == "text" || delta.PartType == "reasoning") {
						pendingDeltas = appendCoalescedRuntimeConversationDelta(pendingDeltas, delta)
						pendingDeltaBytes += len(delta.Delta)
						if deltaTimer == nil {
							deltaTimer = time.NewTimer(runtimeConversationV2DeltaFlushInterval)
							deltaTimerC = deltaTimer.C
						}
						if len(pendingDeltas) >= runtimeConversationV2MaxPendingDeltas || pendingDeltaBytes >= runtimeConversationV2MaxPendingDeltaBytes {
							if !flushDeltas() {
								return
							}
						}
					}
					continue
				}
				if !flushDeltas() {
					return
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
				stopDeltaTimer()
				return
			}
		}
	}()
	return out, done
}

func appendCoalescedRuntimeConversationDelta(pending []RuntimeConversationTextDeltaV2, delta RuntimeConversationTextDeltaV2) []RuntimeConversationTextDeltaV2 {
	for index := len(pending) - 1; index >= 0; index-- {
		previous := &pending[index]
		if previous.MessageID != delta.MessageID || previous.PartType != delta.PartType {
			continue
		}
		if previous.ContentLength+len(delta.Delta) != delta.ContentLength {
			break
		}
		previous.Delta += delta.Delta
		previous.ContentLength = delta.ContentLength
		if delta.CreatedAt > previous.CreatedAt {
			previous.CreatedAt = delta.CreatedAt
		}
		return pending
	}
	return append(pending, delta)
}

func sendCanonicalConversationBatchV2(ctx context.Context, out chan RuntimeCanonicalConversationEventBatchV2, batch RuntimeCanonicalConversationEventBatchV2, sessionID, cursor string) bool {
	if encoded, err := json.Marshal(batch); err != nil || len(encoded) > runtimeConversationV2MaxEncodedBatchBytes {
		if len(batch.Deltas) > 0 && len(batch.Events) == 0 {
			// Advisory deltas may be dropped; the next durable message revision
			// repairs the live suffix without forcing a snapshot.
			return true
		}
		batch = RuntimeCanonicalConversationEventBatchV2{SchemaVersion: RuntimeConversationSchemaVersion, SessionID: sessionID, AfterCursor: cursor, Cursor: cursor, Events: []RuntimeConversationEntityEventV2{}, SnapshotRequired: true, Reason: "batch_too_large"}
	}
	if len(batch.Deltas) > 0 && len(batch.Events) == 0 {
		select {
		case out <- batch:
		case <-ctx.Done():
			return false
		default:
			// Live suffixes are advisory. Dropping one under backpressure is
			// safer than evicting a durable batch or forcing snapshot recovery;
			// the next canonical message revision reconciles the full content.
		}
		return true
	}
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
