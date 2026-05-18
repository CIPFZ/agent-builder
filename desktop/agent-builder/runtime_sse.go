package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

type runtimeSSEServer struct {
	mu          sync.Mutex
	server      *http.Server
	listener    net.Listener
	url         string
	token       string
	subscribers map[chan RuntimeEvent]struct{}
}

func newRuntimeSSEServer() *runtimeSSEServer {
	return &runtimeSSEServer{
		token:       newStreamToken(),
		subscribers: make(map[chan RuntimeEvent]struct{}),
	}
}

func (s *runtimeSSEServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.url != "" {
		return nil
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to listen for desktop runtime SSE: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/events", s.handleEvents)
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	s.listener = listener
	s.server = server
	s.url = fmt.Sprintf("http://%s/events?token=%s", listener.Addr().String(), s.token)

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			slog.Error("Desktop runtime SSE server stopped", "error", err)
		}
	}()

	return nil
}

func (s *runtimeSSEServer) Close(ctx context.Context) error {
	s.mu.Lock()
	server := s.server
	s.server = nil
	s.listener = nil
	s.url = ""
	s.mu.Unlock()

	if server == nil {
		return nil
	}
	return server.Shutdown(ctx)
}

func (s *runtimeSSEServer) URL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.url
}

func (s *runtimeSSEServer) Publish(event RuntimeEvent) {
	s.mu.Lock()
	subscribers := make([]chan RuntimeEvent, 0, len(s.subscribers))
	for subscriber := range s.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	s.mu.Unlock()

	for _, subscriber := range subscribers {
		select {
		case subscriber <- event:
		default:
			slog.Warn("Dropping desktop runtime SSE event for slow subscriber", "type", event.Type)
		}
	}
}

func (s *runtimeSSEServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Query().Get("token") != s.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	events := make(chan RuntimeEvent, 64)
	s.addSubscriber(events)
	defer s.removeSubscriber(events)

	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				slog.Error("Failed to encode desktop runtime SSE event", "error", err)
				continue
			}
			fmt.Fprintf(w, "event: runtime-event\ndata: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *runtimeSSEServer) addSubscriber(events chan RuntimeEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscribers[events] = struct{}{}
}

func (s *runtimeSSEServer) removeSubscriber(events chan RuntimeEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subscribers[events]; ok {
		delete(s.subscribers, events)
	}
}

func newStreamToken() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(data[:])
}
