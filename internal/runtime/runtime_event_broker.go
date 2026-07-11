package runtime

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type runtimeEventBroker struct {
	mu          sync.Mutex
	subscribers map[chan RuntimeEvent]struct{}
}

func newRuntimeEventBroker() *runtimeEventBroker {
	return &runtimeEventBroker{subscribers: make(map[chan RuntimeEvent]struct{})}
}

func (b *runtimeEventBroker) Publish(event RuntimeEvent) {
	b.mu.Lock()
	subscribers := make([]chan RuntimeEvent, 0, len(b.subscribers))
	for subscriber := range b.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	b.mu.Unlock()
	for _, subscriber := range subscribers {
		select {
		case subscriber <- event:
		default:
			slog.Warn("Dropping runtime event for slow Wails subscriber", "type", event.Type)
		}
	}
}

func (b *runtimeEventBroker) addSubscriber(events chan RuntimeEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[events] = struct{}{}
}

func (b *runtimeEventBroker) removeSubscriber(events chan RuntimeEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subscribers, events)
}

func newStreamToken() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(data[:])
}
