package agent

import (
	"errors"
	"fmt"
	"sync/atomic"

	nativeagent "muse/agent"
)

// ProviderClient adapts the feature-owned AI HTTP service to the application
// stream port. It contains no transcript, approval, or UI state.
type ProviderClient struct {
	service *nativeagent.AI
	serial  atomic.Uint64
	active  atomic.Value // string request id
}

func NewProviderClient(service *nativeagent.AI) *ProviderClient {
	return &ProviderClient{service: service}
}

func (c *ProviderClient) Stream(request Request, onDelta func(StreamEvent)) StreamResult {
	if c == nil || c.service == nil {
		return StreamResult{Err: errors.New("assistant provider is unavailable")}
	}
	requestID := fmt.Sprintf("assistant-%d", c.serial.Add(1))
	c.active.Store(requestID)
	wireMessages := make([]map[string]any, 0, len(request.Messages)+1)
	if request.Settings.System != "" {
		wireMessages = append(wireMessages, map[string]any{"role": "system", "content": request.Settings.System})
	}
	for _, message := range request.Messages {
		role := "assistant"
		if message.Role == User {
			role = "user"
		}
		wireMessages = append(wireMessages, map[string]any{"role": role, "content": message.Content})
	}
	result := c.service.ChatStreamWith(nativeagent.ChatStreamRequest{
		RequestID: requestID,
		BaseURL:   request.Settings.BaseURL,
		APIKey:    request.Settings.APIKey,
		Body: map[string]any{
			"model": request.Settings.Model, "stream": true, "messages": wireMessages,
		},
	}, nativeagent.ChatSink{Chunk: func(chunk string) { onDelta(StreamEvent{Text: chunk}) }})
	c.active.Store("")
	if result.Aborted {
		return StreamResult{Aborted: true}
	}
	if !result.Ok {
		return StreamResult{Err: errors.New(result.Message)}
	}
	return StreamResult{}
}

func (c *ProviderClient) Abort() {
	if c == nil || c.service == nil {
		return
	}
	id, _ := c.active.Load().(string)
	if id != "" {
		c.service.ChatAbort(struct {
			RequestID string `json:"requestId"`
		}{RequestID: id})
	}
}
