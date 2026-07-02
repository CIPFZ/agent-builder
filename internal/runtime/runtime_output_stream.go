package runtime

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

// runtimeSessionOutputEventChannelCap bounds each per-session subscriber
// channel. If a subscriber falls behind we drop the whole tail into a
// snapshot_required signal rather than block the publisher.
const runtimeSessionOutputEventChannelCap = 256

// runtimeMessageUpdateCoalesceWindow is the minimum gap between two
// persisted message.updated events for the same message. Deltas are still
// emitted every tick — this just prevents a per-token write amplification
// into SQLite + the runtime event ring buffer.
const runtimeMessageUpdateCoalesceWindow = 250 * time.Millisecond

// runtimeSessionOutputDebounce is the tick at which persisted events are
// diffed via SessionOutputEvents for each subscribed session. Deltas ignore
// this and are fanned out immediately.
const runtimeSessionOutputDebounce = 50 * time.Millisecond

// deriveOutputTextDeltasLocked returns ephemeral output.text.delta events for
// any assistant message whose text or reasoning content has grown since the
// last observation. Callers hold r.mu. Non-assistant messages are ignored.
// If the message is finished, cursor state is retained so a follow-up
// message.completed for the same ID still computes the last delta.
func (r *runtimeService) deriveOutputTextDeltasLocked(msg apitypes.Message, turnID string, now time.Time) []RuntimeEvent {
	if r == nil || r.messageStream == nil || msg.ID == "" {
		return nil
	}
	if msg.Role != apitypes.Assistant {
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
		delta := text[cursor.lastTextLen:]
		if delta != "" {
			events = append(events, newOutputTextDeltaEvent(now, msg.SessionID, msg.ID, turnID, "text", delta, len(text)))
		}
		cursor.lastTextLen = len(text)
	}
	thinking := msg.ReasoningContent().Thinking
	if len(thinking) > cursor.lastReasoningLen {
		delta := thinking[cursor.lastReasoningLen:]
		if delta != "" {
			events = append(events, newOutputTextDeltaEvent(now, msg.SessionID, msg.ID, turnID, "reasoning", delta, len(thinking)))
		}
		cursor.lastReasoningLen = len(thinking)
	}
	if msg.IsFinished() {
		cursor.completed = true
	}
	return events
}

// recordMessageEventWithCoalesceLocked applies the 250ms coalesce window to
// persisted message.updated events. message.created and message.completed
// always go through. Coalesced events are dropped entirely — the delta
// channel already carries the intermediate visual state.
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
		// Skip this event; the delta channel keeps the UI live and the
		// next tick after the window closes will carry the accumulated
		// state.
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
	event.Payload = map[string]any{
		"part_type":   partType,
		"delta":       delta,
		"content_len": contentLen,
	}
	return event
}

// runtimeSessionOutputBroker fans persisted and ephemeral runtime events out
// to per-session subscribers. Deltas are forwarded live; persisted events
// are grouped by a 50ms debounce timer and materialized via the existing
// SessionOutputEvents diff so we do not reinvent projection logic here.
type runtimeSessionOutputBroker struct {
	mu       sync.Mutex
	sessions map[string][]*runtimeSessionOutputSubscriber
}

type runtimeSessionOutputSubscriber struct {
	sessionID string
	cursor    int64
	events    chan RuntimeOutputEvent
	// dirty is set whenever a persisted event arrives; the flush loop
	// consults it every debounce tick.
	dirty       chan struct{}
	cancel      chan struct{}
	rejectedOnce sync.Once
}

func newRuntimeSessionOutputBroker() *runtimeSessionOutputBroker {
	return &runtimeSessionOutputBroker{sessions: map[string][]*runtimeSessionOutputSubscriber{}}
}

// SubscribeSessionOutputEvents opens a per-session channel that receives
// both ephemeral text-delta events and materialized output events from
// SessionOutputEvents. The returned closer removes the subscriber. `after`
// is the last cursor the client already applied; the first flush picks up
// from there.
func (r *runtimeService) SubscribeSessionOutputEvents(ctx context.Context, sessionID string, after string) (<-chan RuntimeOutputEvent, func()) {
	r.mu.Lock()
	if r.sessionOutputStream == nil {
		r.sessionOutputStream = newRuntimeSessionOutputBroker()
	}
	broker := r.sessionOutputStream
	r.mu.Unlock()

	cursor, _ := strconv.ParseInt(strings.TrimSpace(after), 10, 64)
	sub := &runtimeSessionOutputSubscriber{
		sessionID: sessionID,
		cursor:    cursor,
		events:    make(chan RuntimeOutputEvent, runtimeSessionOutputEventChannelCap),
		dirty:     make(chan struct{}, 1),
		cancel:    make(chan struct{}),
	}
	broker.register(sub)
	go r.runSessionOutputFlushLoop(ctx, sub)

	closer := func() {
		close(sub.cancel)
		broker.unregister(sub)
	}
	return sub.events, closer
}

func (b *runtimeSessionOutputBroker) register(sub *runtimeSessionOutputSubscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sessions[sub.sessionID] = append(b.sessions[sub.sessionID], sub)
}

func (b *runtimeSessionOutputBroker) unregister(sub *runtimeSessionOutputSubscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subs := b.sessions[sub.sessionID]
	kept := subs[:0]
	for _, candidate := range subs {
		if candidate == sub {
			continue
		}
		kept = append(kept, candidate)
	}
	if len(kept) == 0 {
		delete(b.sessions, sub.sessionID)
	} else {
		b.sessions[sub.sessionID] = kept
	}
}

func (b *runtimeSessionOutputBroker) subscribersFor(sessionID string) []*runtimeSessionOutputSubscriber {
	b.mu.Lock()
	defer b.mu.Unlock()
	subs := b.sessions[sessionID]
	out := make([]*runtimeSessionOutputSubscriber, len(subs))
	copy(out, subs)
	return out
}

// PublishSessionOutputEvent is called from runtime_events for persisted or
// ephemeral events (see publishRuntimeEvent). Deltas are converted to a
// RuntimeOutputEvent and pushed directly; persisted events just wake the
// flush loop so it will call SessionOutputEvents at the next tick.
func (r *runtimeService) publishSessionOutputEventFromRuntime(event RuntimeEvent) {
	if r.sessionOutputStream == nil || event.SessionID == "" {
		return
	}
	subs := r.sessionOutputStream.subscribersFor(event.SessionID)
	if len(subs) == 0 {
		return
	}
	if event.Type == runtimeapi.EventOutputTextDelta {
		delta := textDeltaFromPayload(event)
		if delta == nil {
			return
		}
		output := RuntimeOutputEvent{
			ID:        event.ID,
			SessionID: event.SessionID,
			TurnID:    event.TurnID,
			Kind:      runtimeapi.EventOutputTextDelta,
			EntityID:  event.MessageID,
			Operation: "delta",
			CreatedAt: runtimeOutputEventTime(event),
			TextDelta: delta,
		}
		for _, sub := range subs {
			sendOrOverflow(sub, output)
		}
		return
	}
	for _, sub := range subs {
		select {
		case sub.dirty <- struct{}{}:
		default:
		}
	}
}

func textDeltaFromPayload(event RuntimeEvent) *RuntimeOutputTextDelta {
	partType, _ := event.Payload["part_type"].(string)
	delta, _ := event.Payload["delta"].(string)
	if partType == "" || delta == "" {
		return nil
	}
	var contentLen int
	switch v := event.Payload["content_len"].(type) {
	case int:
		contentLen = v
	case int64:
		contentLen = int(v)
	case float64:
		contentLen = int(v)
	}
	return &RuntimeOutputTextDelta{
		MessageID:  event.MessageID,
		TurnID:     event.TurnID,
		PartType:   partType,
		Delta:      delta,
		ContentLen: contentLen,
	}
}

// runSessionOutputFlushLoop debounces persisted events per subscriber and
// calls SessionOutputEvents to materialize them into typed output events.
// It reuses the projection diff so we do not maintain a second increment
// engine here.
func (r *runtimeService) runSessionOutputFlushLoop(ctx context.Context, sub *runtimeSessionOutputSubscriber) {
	timer := time.NewTimer(runtimeSessionOutputDebounce)
	defer timer.Stop()
	pending := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-sub.cancel:
			return
		case <-sub.dirty:
			pending = true
			// Reset the debounce window; we want to bundle bursts.
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(runtimeSessionOutputDebounce)
		case <-timer.C:
			if !pending {
				timer.Reset(runtimeSessionOutputDebounce)
				continue
			}
			pending = false
			cursor := strconv.FormatInt(sub.cursor, 10)
			resp, err := r.SessionOutputEvents(ctx, sub.sessionID, cursor)
			if err == nil {
				for _, ev := range resp.Events {
					sendOrOverflow(sub, ev)
					if ev.Sequence > sub.cursor {
						sub.cursor = ev.Sequence
					}
				}
				if parsed, err := strconv.ParseInt(resp.Cursor, 10, 64); err == nil && parsed > sub.cursor {
					sub.cursor = parsed
				}
			}
			timer.Reset(runtimeSessionOutputDebounce)
		}
	}
}

func sendOrOverflow(sub *runtimeSessionOutputSubscriber, event RuntimeOutputEvent) {
	select {
	case sub.events <- event:
	default:
		// Downstream consumer fell behind; send a single snapshot_required
		// signal so it can rehydrate from a full snapshot. Suppress any
		// further sends until the closer runs. The signal is prioritized
		// over the tail: we drain one pending event to make room, because
		// once the client hits snapshot_required it will re-hydrate from
		// scratch anyway and any lost tail is redundant.
		sub.rejectedOnce.Do(func() {
			signal := RuntimeOutputEvent{
				ID:        newRuntimeEventID(),
				SessionID: sub.sessionID,
				Kind:      runtimeapi.EventSnapshotRequired,
				Operation: "reset",
				CreatedAt: time.Now().UnixMilli(),
			}
			select {
			case <-sub.events:
			default:
			}
			select {
			case sub.events <- signal:
			default:
			}
		})
	}
}
