package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	tools := map[int]*ToolRequest{}
	wireMessages := make([]map[string]any, 0, len(request.Messages)+1)
	if request.Settings.System != "" {
		wireMessages = append(wireMessages, map[string]any{"role": "system", "content": request.Settings.System})
	}
	for _, message := range request.Messages {
		role := "assistant"
		if message.Role == User {
			role = "user"
		}
		wire := map[string]any{"role": role, "content": message.Content}
		var calls []map[string]any
		for _, part := range message.Parts {
			if part.Kind != ToolPart {
				continue
			}
			if part.Tool.Arguments != "" {
				calls = append(calls, map[string]any{"id": part.Tool.ID, "type": "function", "function": map[string]any{"name": part.Tool.Name, "arguments": part.Tool.Arguments}})
			}
			if part.Tool.Result != "" || part.Tool.Error != "" {
				result := part.Tool.Result
				if part.Tool.Error != "" {
					result = part.Tool.Error
				}
				wireMessages = append(wireMessages, map[string]any{"role": "tool", "tool_call_id": part.Tool.ID, "content": result})
			}
		}
		if len(calls) > 0 {
			wire["tool_calls"] = calls
		}
		wireMessages = append(wireMessages, wire)
	}
	result := c.service.ChatStreamWith(nativeagent.ChatStreamRequest{
		RequestID: requestID,
		BaseURL:   request.Settings.BaseURL,
		APIKey:    request.Settings.APIKey,
		Body: map[string]any{
			"model": request.Settings.Model, "stream": true, "messages": wireMessages, "tools": request.Tools,
		},
	}, nativeagent.ChatSink{Chunk: func(chunk string) {
		for _, line := range strings.Split(chunk, "\n") {
			line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "data:"))
			if line == "" || line == "[DONE]" {
				continue
			}
			var payload struct {
				Choices []struct {
					Delta struct {
						Content   string `json:"content"`
						Reasoning string `json:"reasoning_content"`
						Tools     []struct {
							Index    int    `json:"index"`
							ID       string `json:"id"`
							Function struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							} `json:"function"`
						} `json:"tool_calls"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if json.Unmarshal([]byte(line), &payload) != nil || len(payload.Choices) == 0 {
				continue
			}
			delta := payload.Choices[0].Delta
			if delta.Content != "" || delta.Reasoning != "" {
				onDelta(StreamEvent{Text: delta.Content, Thought: delta.Reasoning})
			}
			for _, part := range delta.Tools {
				call := tools[part.Index]
				if call == nil {
					call = &ToolRequest{}
					tools[part.Index] = call
				}
				call.ID += part.ID
				call.Name += part.Function.Name
				call.Arguments += part.Function.Arguments
			}
		}
	}})
	c.active.Store("")
	if result.Aborted {
		return StreamResult{Aborted: true}
	}
	if !result.Ok {
		return StreamResult{Err: errors.New(result.Message)}
	}
	ordered := make([]ToolRequest, 0, len(tools))
	for index := 0; index < len(tools); index++ {
		if call := tools[index]; call != nil {
			ordered = append(ordered, *call)
		}
	}
	return StreamResult{Tools: ordered}
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
