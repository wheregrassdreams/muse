package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected request: %s, auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"group/model","display_name":"Gateway model","reasoning":true,"context_window":256000,"max_output_tokens":32000,"modalities":{"input":["text","image"]}},{"id":"group/model"},{"id":""},{"id":"custom/model"}]}`))
	}))
	defer server.Close()
	for _, base := range []string{server.URL, server.URL + "/v1/"} {
		result := NewAI().ListModels(TestConnRequest{BaseURL: base, APIKey: " test-key "})
		if !result.Ok || len(result.Models) != 2 {
			t.Fatalf("discovery failed: %+v", result)
		}
		model := result.Models[0]
		if model.Name != "Gateway model" || !model.Reasoning || model.ContextWindow != 256000 || model.MaxTokens != 32000 || len(model.Input) != 2 {
			t.Fatalf("metadata missing: %+v", model)
		}
	}
}

func TestListModelsErrors(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		ok     bool
	}{
		{"auth", 401, `{"error":{"message":"secret"}}`, false},
		{"html", 200, `<html>gateway dashboard</html>`, false},
		{"missing list", 200, `{}`, false},
		{"empty list", 200, `{"data":[]}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" {
					t.Error("optional key should not send authorization")
				}
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			result := NewAI().ListModels(TestConnRequest{BaseURL: server.URL})
			if result.Ok != test.ok || result.Status != test.status {
				t.Fatalf("unexpected result: %+v", result)
			}
		})
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	for input, want := range map[string]string{
		" http://127.0.0.1:3425/ ":       "http://127.0.0.1:3425/v1",
		"https://example.com/v1/":        "https://example.com/v1",
		"https://example.com/zen/go/v1/": "https://example.com/zen/go/v1",
		"":                               "",
	} {
		if got := normalizeBaseURL(input); got != want {
			t.Errorf("normalize(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestChatStreamWithDirectSink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	var chunks []string
	ended := false
	res := NewAI().ChatStreamWith(ChatStreamRequest{
		RequestID: "t1",
		BaseURL:   server.URL,
		Body:      map[string]any{"model": "m", "stream": true},
	}, ChatSink{
		Chunk: func(c string) { chunks = append(chunks, c) },
		End:   func() { ended = true },
	})
	if !res.Ok || !ended || len(chunks) != 3 {
		t.Fatalf("res=%+v ended=%v chunks=%q", res, ended, chunks)
	}
	if !strings.Contains(strings.Join(chunks, ""), "Hel") {
		t.Fatalf("chunks = %q", chunks)
	}
}
