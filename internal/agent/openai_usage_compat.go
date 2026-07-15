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

type openAIUsageContextKey struct{}

type openAIUsageTracker struct {
	mu    sync.Mutex
	usage fantasy.Usage
}

func (t *openAIUsageTracker) update(usage fantasy.Usage) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.usage.InputTokens = max(t.usage.InputTokens, usage.InputTokens)
	t.usage.OutputTokens = max(t.usage.OutputTokens, usage.OutputTokens)
	t.usage.TotalTokens = max(t.usage.TotalTokens, usage.TotalTokens)
	t.usage.ReasoningTokens = max(t.usage.ReasoningTokens, usage.ReasoningTokens)
	t.usage.CacheReadTokens = max(t.usage.CacheReadTokens, usage.CacheReadTokens)
}

func (t *openAIUsageTracker) snapshot() fantasy.Usage {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.usage
}

type openAIUsageTransport struct {
	base http.RoundTripper
}

func (t *openAIUsageTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}
	tracker, _ := req.Context().Value(openAIUsageContextKey{}).(*openAIUsageTracker)
	if tracker != nil {
		resp.Body = &openAIUsageBody{ReadCloser: resp.Body, tracker: tracker}
	}
	return resp, nil
}

type openAIUsageBody struct {
	io.ReadCloser
	tracker *openAIUsageTracker
	pending []byte
}

func (b *openAIUsageBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.consume(p[:n], false)
	}
	if err == io.EOF {
		b.consume(nil, true)
	}
	return n, err
}

func (b *openAIUsageBody) Close() error {
	b.consume(nil, true)
	return b.ReadCloser.Close()
}

func (b *openAIUsageBody) consume(chunk []byte, flush bool) {
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

func (b *openAIUsageBody) captureLine(line []byte) {
	line = bytes.TrimSpace(line)
	line = bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
	if len(line) == 0 || bytes.Equal(line, []byte("[DONE]")) {
		return
	}
	var payload struct {
		Usage    openAIWireUsage `json:"usage"`
		Response struct {
			Usage openAIWireUsage `json:"usage"`
		} `json:"response"`
	}
	if json.Unmarshal(line, &payload) != nil {
		return
	}
	b.tracker.update(payload.Usage.fantasyUsage())
	b.tracker.update(payload.Response.Usage.fantasyUsage())
}

type openAIWireUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	PromptDetails    struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	InputDetails struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	CompletionDetails struct {
		ReasoningTokens int64 `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
	OutputDetails struct {
		ReasoningTokens int64 `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

func (u openAIWireUsage) fantasyUsage() fantasy.Usage {
	promptTokens := max(u.PromptTokens, u.InputTokens)
	outputTokens := max(u.CompletionTokens, u.OutputTokens)
	cacheReadTokens := max(u.PromptDetails.CachedTokens, u.InputDetails.CachedTokens)
	if promptTokens == 0 && outputTokens > 0 && u.TotalTokens >= outputTokens {
		promptTokens = u.TotalTokens - outputTokens
	}
	inputTokens := max(promptTokens-cacheReadTokens, 0)
	totalTokens := u.TotalTokens
	if totalTokens == 0 {
		totalTokens = promptTokens + outputTokens
	}
	return fantasy.Usage{
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		TotalTokens:     totalTokens,
		ReasoningTokens: max(u.CompletionDetails.ReasoningTokens, u.OutputDetails.ReasoningTokens),
		CacheReadTokens: cacheReadTokens,
	}
}

func newOpenAIUsageHTTPClient(base *http.Client) *http.Client {
	if base == nil {
		base = http.DefaultClient
	}
	client := *base
	transport := base.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client.Transport = &openAIUsageTransport{base: transport}
	return &client
}

type openAIUsageProvider struct {
	fantasy.Provider
}

func (p openAIUsageProvider) LanguageModel(ctx context.Context, modelID string) (fantasy.LanguageModel, error) {
	model, err := p.Provider.LanguageModel(ctx, modelID)
	if err != nil {
		return nil, err
	}
	return openAIUsageModel{LanguageModel: model}, nil
}

type openAIUsageModel struct {
	fantasy.LanguageModel
}

func (m openAIUsageModel) Generate(ctx context.Context, call fantasy.Call) (*fantasy.Response, error) {
	tracker := &openAIUsageTracker{}
	resp, err := m.LanguageModel.Generate(context.WithValue(ctx, openAIUsageContextKey{}, tracker), call)
	if resp != nil {
		resp.Usage = mergeOpenAIUsage(resp.Usage, tracker.snapshot())
	}
	return resp, err
}

func (m openAIUsageModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	tracker := &openAIUsageTracker{}
	stream, err := m.LanguageModel.Stream(context.WithValue(ctx, openAIUsageContextKey{}, tracker), call)
	if err != nil {
		return nil, err
	}
	return func(yield func(fantasy.StreamPart) bool) {
		for part := range stream {
			if part.Type == fantasy.StreamPartTypeFinish {
				part.Usage = mergeOpenAIUsage(part.Usage, tracker.snapshot())
			}
			if !yield(part) {
				return
			}
		}
	}, nil
}

func mergeOpenAIUsage(usage, captured fantasy.Usage) fantasy.Usage {
	usage.InputTokens = max(usage.InputTokens, captured.InputTokens)
	usage.OutputTokens = max(usage.OutputTokens, captured.OutputTokens)
	usage.TotalTokens = max(usage.TotalTokens, captured.TotalTokens)
	usage.ReasoningTokens = max(usage.ReasoningTokens, captured.ReasoningTokens)
	usage.CacheReadTokens = max(usage.CacheReadTokens, captured.CacheReadTokens)
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens + usage.CacheReadTokens
	}
	return usage
}
