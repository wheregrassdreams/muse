// Package agent implements the concrete OpenAI-compatible and Brave HTTP
// adapters used by the application agent contracts. Credentials remain in
// the Go process and never cross into a renderer.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	applicationagent "muse/application/agent"
	"muse/locales"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const aiTimeout = 10 * time.Second

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

// Provider is the concrete HTTP-backed provider adapter.
type Provider struct {
	mu       sync.Mutex
	aborters map[string]context.CancelFunc
	client   *http.Client
}

var _ applicationagent.ProviderManagement = (*Provider)(nil)

// NewProvider constructs the HTTP-backed OpenAI-compatible and Brave adapter.
func NewProvider() *Provider {
	return &Provider{aborters: map[string]context.CancelFunc{}, client: &http.Client{}}
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
func (a *Provider) TestConnection(ctx context.Context, req applicationagent.ProviderCredentials) (applicationagent.ProviderCheck, error) {
	models, err := a.ListModels(ctx, req)
	return applicationagent.ProviderCheck{StatusCode: models.StatusCode}, err
}

// ListModels discovers models through the same API prefix used for chat requests.
func (a *Provider) ListModels(parent context.Context, req applicationagent.ProviderCredentials) (applicationagent.ProviderModelList, error) {
	failure := func(status int, kind applicationagent.ProviderErrorKind, detail string) (applicationagent.ProviderModelList, error) {
		return applicationagent.ProviderModelList{StatusCode: status}, &applicationagent.ProviderError{Kind: kind, StatusCode: status, Detail: detail}
	}
	baseURL := normalizeBaseURL(req.BaseURL)
	apiKey := strings.TrimSpace(req.APIKey)
	if baseURL == "" {
		return failure(0, applicationagent.ProviderErrorInvalidConfiguration, "")
	}

	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, aiTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return failure(0, applicationagent.ProviderErrorInvalidConfiguration, err.Error())
	}
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}
	httpReq.Header.Set("Accept", "application/json")
	res, err := a.client.Do(httpReq)
	if err != nil {
		if parent.Err() != nil {
			return applicationagent.ProviderModelList{}, parent.Err()
		}
		kind := applicationagent.ProviderErrorNetwork
		if errors.Is(err, context.DeadlineExceeded) || osIsTimeout(err) {
			kind = applicationagent.ProviderErrorTimeout
		}
		return failure(0, kind, providerNetworkDetail(err))
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		if parent.Err() != nil {
			return applicationagent.ProviderModelList{}, parent.Err()
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return failure(res.StatusCode, applicationagent.ProviderErrorTimeout, providerNetworkDetail(ctx.Err()))
		}
		return failure(res.StatusCode, applicationagent.ProviderErrorReadResponse, "")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		kind := applicationagent.ProviderErrorRequest
		if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
			kind = applicationagent.ProviderErrorUnauthorized
		}
		return failure(res.StatusCode, kind, providerHTTPDetail(res.StatusCode, string(body)))
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
		return failure(res.StatusCode, applicationagent.ProviderErrorInvalidModelList, "")
	}
	models := make([]applicationagent.ProviderModel, 0, len(data.Data))
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
		models = append(models, applicationagent.ProviderModel{ID: id, Name: name, Reasoning: m.Reasoning,
			Input: m.Modalities.Input, ContextWindow: contextWindow, MaxTokens: m.MaxTokens})
	}
	return applicationagent.ProviderModelList{StatusCode: res.StatusCode, Models: models}, nil
}

func providerNetworkDetail(err error) string {
	var uerr *url.Error
	if errors.As(err, &uerr) && uerr.Err != nil {
		return uerr.Err.Error()
	}
	return err.Error()
}

func providerHTTPDetail(status int, raw string) string {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return ""
	}
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(raw), &parsed) == nil {
		return parsed.Error.Message
	}
	return ""
}

// ChatStreamWith is ChatStream without the web event bus: the caller gets the
// raw SSE chunks, end and error through sink.
func (a *Provider) ChatStreamWith(req ChatStreamRequest, sink ChatSink) ChatResult {
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
func (a *Provider) ChatAbort(req struct {
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
func (a *Provider) WebSearch(req struct {
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
