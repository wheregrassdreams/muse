// AI 服务：对应 Electron 版 electron/services/ai.ts。
// 密钥只进 Go 进程网络层，不暴露给渲染进程。
//
//	AI.TestConnection  请求 { baseUrl, apiKey } -> { ok, status?, message }
//	AI.ChatStream      发起 OpenAI 兼容流式对话，SSE 块经事件转发给渲染进程
//	AI.ChatAbort       中止指定 requestId 的上游请求
//	AI.WebSearch       Brave Search API 联网搜索 -> { ok, results?, message? }
//
// 事件（Go -> 渲染进程）：ai:chat-chunk / ai:chat-end / ai:chat-error
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"muse/locales"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const aiTimeout = 10 * time.Second

// TestConnResult 是连接测试的返回。
type TestConnResult struct {
	Ok      bool   `json:"ok"`
	Status  int    `json:"status,omitempty"`
	Message string `json:"message"`
}

// TestConnRequest 是连接测试的请求。
type TestConnRequest struct {
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
}

type ProviderModel struct {
	ID            string   `json:"id"`
	Name          string   `json:"name,omitempty"`
	Reasoning     bool     `json:"reasoning,omitempty"`
	Input         []string `json:"input,omitempty"`
	ContextWindow int      `json:"contextWindow,omitempty"`
	MaxTokens     int      `json:"maxTokens,omitempty"`
}

type ModelsResult struct {
	TestConnResult
	Models []ProviderModel `json:"models,omitempty"`
}

// ChatStreamRequest 是流式对话的请求。
type ChatStreamRequest struct {
	RequestID string         `json:"requestId"`
	BaseURL   string         `json:"baseUrl"`
	APIKey    string         `json:"apiKey"`
	Body      map[string]any `json:"body"`
}

// ChatResult 是流式对话的最终返回（块经事件先行转发）。
type ChatResult struct {
	Ok      bool   `json:"ok"`
	Aborted bool   `json:"aborted,omitempty"`
	Message string `json:"message,omitempty"`
}

// ChatSink receives a stream directly, for callers that are not a web page
// (the native UI). Any callback may be nil.
type ChatSink struct {
	Chunk func(string)
	End   func()
	Error func(string)
}

// WebSearchItem 是一条搜索结果。
type WebSearchItem struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

// WebSearchResponse 是联网搜索的返回。
type WebSearchResponse struct {
	Ok      bool            `json:"ok"`
	Results []WebSearchItem `json:"results,omitempty"`
	Status  int             `json:"status,omitempty"`
	Message string          `json:"message,omitempty"`
}

// AI 是 AI 服务对象。
type AI struct {
	mu       sync.Mutex
	aborters map[string]context.CancelFunc
	client   *http.Client
}

// NewAI 初始化 AI 服务。
func NewAI() *AI {
	return &AI{aborters: map[string]context.CancelFunc{}, client: &http.Client{}}
}

func normalizeBaseURL(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(base)
	if err == nil && u.Host != "" && u.Path == "" {
		u.Path = "/v1"
		return u.String()
	}
	return base
}

func describeHTTPError(status int, raw string) string {
	if status == 401 || status == 403 {
		return locales.AiServiceAuthFailure()
	}
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(raw), &parsed) == nil && parsed.Error.Message != "" {
		return parsed.Error.Message
	}
	return locales.AiServiceRequestFailed(status)
}

func describeNetworkError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) || osIsTimeout(err) {
		return locales.AiServiceTimeout(int(aiTimeout / time.Second))
	}
	var uerr *url.Error
	if errors.As(err, &uerr) && uerr.Err != nil {
		err = uerr.Err
	}
	msg := err.Error()
	for _, code := range []string{"ENOTFOUND", "ECONNREFUSED", "EHOSTUNREACH", "ECONNRESET", "ETIMEDOUT",
		"no such host", "connection refused", "network is unreachable"} {
		if strings.Contains(msg, code) {
			return locales.AiServiceUnreachable()
		}
	}
	return msg
}

func osIsTimeout(err error) bool {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return uerr.Timeout()
	}
	return false
}

// TestConnection 测试与 OpenAI 兼容端点的连通性。
func (a *AI) TestConnection(req TestConnRequest) TestConnResult {
	return a.ListModels(req).TestConnResult
}

// ListModels discovers models through the same API prefix used for chat requests.
func (a *AI) ListModels(req TestConnRequest) ModelsResult {
	failure := func(status int, message string) ModelsResult {
		return ModelsResult{TestConnResult: TestConnResult{Status: status, Message: message}}
	}
	baseURL := normalizeBaseURL(req.BaseURL)
	apiKey := strings.TrimSpace(req.APIKey)
	if baseURL == "" {
		return failure(0, locales.AiServiceFillBaseURL())
	}

	ctx, cancel := context.WithTimeout(context.Background(), aiTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return failure(0, err.Error())
	}
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}
	httpReq.Header.Set("Accept", "application/json")
	res, err := a.client.Do(httpReq)
	if err != nil {
		return failure(0, describeNetworkError(err))
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return failure(res.StatusCode, locales.AiServiceReadModelsFailed())
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return failure(res.StatusCode, describeHTTPError(res.StatusCode, string(body)))
	}
	var data struct {
		Data []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			DisplayName   string `json:"display_name"`
			Reasoning     bool   `json:"reasoning"`
			ContextWindow int    `json:"context_window"`
			ContextLength int    `json:"context_length"`
			MaxTokens     int    `json:"max_output_tokens"`
			Modalities    struct {
				Input []string `json:"input"`
			} `json:"modalities"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &data) != nil || data.Data == nil {
		return failure(res.StatusCode, locales.AiServiceInvalidModelList())
	}
	models := make([]ProviderModel, 0, len(data.Data))
	seen := map[string]bool{}
	for _, m := range data.Data {
		id := strings.TrimSpace(m.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		name := m.DisplayName
		if name == "" {
			name = m.Name
		}
		contextWindow := m.ContextWindow
		if contextWindow == 0 {
			contextWindow = m.ContextLength
		}
		models = append(models, ProviderModel{ID: id, Name: name, Reasoning: m.Reasoning,
			Input: m.Modalities.Input, ContextWindow: contextWindow, MaxTokens: m.MaxTokens})
	}
	return ModelsResult{TestConnResult: TestConnResult{Ok: true, Status: res.StatusCode,
		Message: locales.AiServiceModelsLoaded(len(models))}, Models: models}
}

// ChatStream 发起流式对话：POST {baseUrl}/chat/completions（stream: true），
// 按 SSE 事件边界（\n\n）切块后经 ai:chat-chunk 转发；渲染进程负责拼回流。
// 方法与 Electron 版语义一致：调用在整个流结束后才返回。
func (a *AI) ChatStream(req ChatStreamRequest) ChatResult {
	return a.ChatStreamWith(req, ChatSink{
		Chunk: func(chunk string) {
			_ = EvChatChunk.Broadcast(map[string]any{"requestId": req.RequestID, "chunk": chunk})
		},
		End: func() {
			_ = EvChatEnd.Broadcast(map[string]any{"requestId": req.RequestID})
		},
		Error: func(message string) {
			_ = EvChatError.Broadcast(map[string]any{"requestId": req.RequestID, "message": message})
		},
	})
}

// ChatStreamWith is ChatStream without the web event bus: the caller gets the
// raw SSE chunks, end and error through sink.
func (a *AI) ChatStreamWith(req ChatStreamRequest, sink ChatSink) ChatResult {
	requestID := req.RequestID
	baseURL := normalizeBaseURL(req.BaseURL)
	apiKey := strings.TrimSpace(req.APIKey)
	if requestID == "" || baseURL == "" || req.Body == nil {
		return ChatResult{Ok: false, Message: locales.AiServiceRequestIncomplete()}
	}

	payload, err := json.Marshal(req.Body)
	if err != nil {
		return ChatResult{Ok: false, Message: err.Error()}
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.aborters[requestID] = cancel
	a.mu.Unlock()
	defer func() {
		cancel()
		a.mu.Lock()
		delete(a.aborters, requestID)
		a.mu.Unlock()
	}()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return ChatResult{Ok: false, Message: err.Error()}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	res, err := a.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return ChatResult{Ok: false, Aborted: true, Message: locales.AiServiceAborted()}
		}
		message := describeNetworkError(err)
		if sink.Error != nil {
			sink.Error(message)
		}
		return ChatResult{Ok: false, Message: message}
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		message := describeHTTPError(res.StatusCode, string(raw))
		if sink.Error != nil {
			sink.Error(message)
		}
		return ChatResult{Ok: false, Message: message}
	}

	buf := make([]byte, 32*1024)
	var acc string
	for {
		n, readErr := res.Body.Read(buf)
		if n > 0 {
			acc += string(buf[:n])
			for {
				sep := strings.Index(acc, "\n\n")
				if sep < 0 {
					break
				}
				chunk := acc[:sep+2]
				acc = acc[sep+2:]
				if sink.Chunk != nil {
					sink.Chunk(chunk)
				}
			}
		}
		if readErr != nil {
			if readErr != io.EOF && !errors.Is(readErr, context.Canceled) {
				message := describeNetworkError(readErr)
				if sink.Error != nil {
					sink.Error(message)
				}
				return ChatResult{Ok: false, Message: message}
			}
			break
		}
	}
	// 兜底：流以单个 \n 结尾等边界情况
	if strings.TrimSpace(acc) != "" {
		if sink.Chunk != nil {
			sink.Chunk(acc)
		}
	}
	if ctx.Err() != nil {
		return ChatResult{Ok: false, Aborted: true, Message: locales.AiServiceAborted()}
	}
	if sink.End != nil {
		sink.End()
	}
	return ChatResult{Ok: true}
}

// ChatAbort 中止指定 requestId 的上游请求。
func (a *AI) ChatAbort(req struct {
	RequestID string `json:"requestId"`
},
) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if cancel, ok := a.aborters[req.RequestID]; ok {
		cancel()
		delete(a.aborters, req.RequestID)
	}
}

// WebSearch Brave Search API 联网搜索。
func (a *AI) WebSearch(req struct {
	Query  string `json:"query"`
	APIKey string `json:"apiKey"`
	Count  int    `json:"count"`
},
) WebSearchResponse {
	query := strings.TrimSpace(req.Query)
	apiKey := strings.TrimSpace(req.APIKey)
	if query == "" {
		return WebSearchResponse{Ok: false, Message: locales.AiServiceSearchQueryEmpty()}
	}
	if apiKey == "" {
		return WebSearchResponse{Ok: false, Message: locales.AiServiceSearchKeyMissing()}
	}
	count := req.Count
	if count <= 0 {
		count = 6
	}

	u, _ := url.Parse("https://api.search.brave.com/res/v1/web/search")
	q := u.Query()
	q.Set("q", query)
	q.Set("count", strconv.Itoa(count))
	u.RawQuery = q.Encode()

	ctx, cancel := context.WithTimeout(context.Background(), aiTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return WebSearchResponse{Ok: false, Message: err.Error()}
	}
	httpReq.Header.Set("X-Subscription-Token", apiKey)
	httpReq.Header.Set("Accept", "application/json")

	res, err := a.client.Do(httpReq)
	if err != nil {
		return WebSearchResponse{Ok: false, Message: describeNetworkError(err)}
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message := locales.AiServiceSearchFailed(res.StatusCode)
		if res.StatusCode == 401 || res.StatusCode == 403 {
			message = locales.AiServiceSearchKeyInvalid()
		}
		return WebSearchResponse{Ok: false, Status: res.StatusCode, Message: message}
	}

	body, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	var data struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if json.Unmarshal(body, &data) != nil {
		return WebSearchResponse{Ok: true}
	}
	out := WebSearchResponse{Ok: true}
	for _, r := range data.Web.Results {
		out.Results = append(out.Results, WebSearchItem{Title: r.Title, URL: r.URL, Description: r.Description})
	}
	return out
}
