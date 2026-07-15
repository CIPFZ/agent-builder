package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"charm.land/fantasy"
)

type anthropicUsageContextKey struct{}

type anthropicUsageTracker struct {
	mu    sync.Mutex
	usage fantasy.Usage
}

func (t *anthropicUsageTracker) update(usage fantasy.Usage) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.usage.InputTokens = max(t.usage.InputTokens, usage.InputTokens)
	t.usage.OutputTokens = max(t.usage.OutputTokens, usage.OutputTokens)
	t.usage.CacheReadTokens = max(t.usage.CacheReadTokens, usage.CacheReadTokens)
	t.usage.CacheCreationTokens = max(t.usage.CacheCreationTokens, usage.CacheCreationTokens)
}

func (t *anthropicUsageTracker) snapshot() fantasy.Usage {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.usage
}

type anthropicUsageTransport struct {
	base http.RoundTripper
}

func (t *anthropicUsageTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}
	tracker, _ := req.Context().Value(anthropicUsageContextKey{}).(*anthropicUsageTracker)
	if tracker != nil {
		resp.Body = &anthropicUsageBody{ReadCloser: resp.Body, tracker: tracker}
	}
	return resp, nil
}

type anthropicUsageBody struct {
	io.ReadCloser
	tracker *anthropicUsageTracker
	pending []byte
}

func (b *anthropicUsageBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.consume(p[:n], false)
	}
	if err == io.EOF {
		b.consume(nil, true)
	}
	return n, err
}

func (b *anthropicUsageBody) Close() error {
	b.consume(nil, true)
	return b.ReadCloser.Close()
}

func (b *anthropicUsageBody) consume(chunk []byte, flush bool) {
	b.pending = append(b.pending, chunk...)
	for {
		index := bytes.IndexByte(b.pending, '\n')
		if index < 0 {
			break
		}
		b.captureLine(b.pending[:index])
		b.pending = b.pending[index+1:]
	}
	if flush && len(b.pending) > 0 {
		b.captureLine(b.pending)
		b.pending = nil
	}
}

func (b *anthropicUsageBody) captureLine(line []byte) {
	line = bytes.TrimSpace(line)
	line = bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
	if len(line) == 0 || bytes.Equal(line, []byte("[DONE]")) {
		return
	}
	var payload struct {
		Usage   anthropicWireUsage `json:"usage"`
		Message struct {
			Usage anthropicWireUsage `json:"usage"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &payload) != nil {
		return
	}
	b.tracker.update(payload.Usage.fantasyUsage())
	b.tracker.update(payload.Message.Usage.fantasyUsage())
}

type anthropicWireUsage struct {
	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	CacheReadTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_input_tokens"`
}

func (u anthropicWireUsage) fantasyUsage() fantasy.Usage {
	return fantasy.Usage{
		InputTokens:         u.InputTokens,
		OutputTokens:        u.OutputTokens,
		CacheReadTokens:     u.CacheReadTokens,
		CacheCreationTokens: u.CacheCreationTokens,
	}
}

func newAnthropicUsageHTTPClient(base *http.Client) *http.Client {
	if base == nil {
		base = http.DefaultClient
	}
	client := *base
	transport := base.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client.Transport = &anthropicUsageTransport{base: transport}
	return &client
}

type anthropicUsageProvider struct {
	fantasy.Provider
}

func (p anthropicUsageProvider) LanguageModel(ctx context.Context, modelID string) (fantasy.LanguageModel, error) {
	model, err := p.Provider.LanguageModel(ctx, modelID)
	if err != nil {
		return nil, err
	}
	return anthropicUsageModel{LanguageModel: model}, nil
}

type anthropicUsageModel struct {
	fantasy.LanguageModel
}

func (m anthropicUsageModel) Generate(ctx context.Context, call fantasy.Call) (*fantasy.Response, error) {
	tracker := &anthropicUsageTracker{}
	resp, err := m.LanguageModel.Generate(context.WithValue(ctx, anthropicUsageContextKey{}, tracker), call)
	if resp != nil {
		resp.Usage = mergeAnthropicUsage(resp.Usage, tracker.snapshot())
	}
	return resp, err
}

func (m anthropicUsageModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	tracker := &anthropicUsageTracker{}
	stream, err := m.LanguageModel.Stream(context.WithValue(ctx, anthropicUsageContextKey{}, tracker), call)
	if err != nil {
		return nil, err
	}
	return func(yield func(fantasy.StreamPart) bool) {
		for part := range stream {
			if part.Type == fantasy.StreamPartTypeFinish {
				part.Usage = mergeAnthropicUsage(part.Usage, tracker.snapshot())
			}
			if !yield(part) {
				return
			}
		}
	}, nil
}

func mergeAnthropicUsage(usage, captured fantasy.Usage) fantasy.Usage {
	usage.InputTokens = max(usage.InputTokens, captured.InputTokens)
	usage.OutputTokens = max(usage.OutputTokens, captured.OutputTokens)
	usage.CacheReadTokens = max(usage.CacheReadTokens, captured.CacheReadTokens)
	usage.CacheCreationTokens = max(usage.CacheCreationTokens, captured.CacheCreationTokens)
	usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	return usage
}
