package bridge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/goodtiger/openclaw-install/internal/config"
)

type stubCompleter struct {
	reply string
}

func (s stubCompleter) Complete(context.Context, string) (string, error) {
	return s.reply, nil
}

type countingCompleter struct {
	reply string
	count *int
}

func (c *countingCompleter) Complete(context.Context, string) (string, error) {
	*c.count++
	return c.reply, nil
}

func TestQQHandlerOK(t *testing.T) {
	cfg := config.BridgeConfig{
		Version: 1,
		Provider: config.ProviderConfig{
			ID:           "deepseek",
			BaseURL:      "https://api.deepseek.com/v1",
			PrimaryModel: "deepseek-chat",
		},
		Channels: map[string]config.BridgeChannelConfig{
			"qq": {
				Enabled:    true,
				Driver:     "onebot",
				ListenAddr: "127.0.0.1:19090",
				Path:       "/qq/events",
			},
		},
	}

	server := NewServer(cfg, stubCompleter{reply: "pong"}, nil, io.Discard)
	handler, err := server.Handler("qq")
	if err != nil {
		t.Fatalf("Handler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/qq/events", strings.NewReader(`{"post_type":"message","message_type":"private","user_id":1,"raw_message":"ping"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	// reply 不应暴露在响应体中
	if _, hasReply := body["reply"]; hasReply {
		t.Fatal("response should not contain 'reply' field")
	}
	if body["ok"] != true {
		t.Fatalf("ok = %#v, want true", body["ok"])
	}
}

func TestQQHandlerBodyTooLarge(t *testing.T) {
	cfg := config.BridgeConfig{
		Version: 1,
		Channels: map[string]config.BridgeChannelConfig{
			"qq": {Enabled: true, Driver: "onebot", Path: "/qq/events"},
		},
	}
	server := NewServer(cfg, stubCompleter{reply: "x"}, nil, io.Discard)
	handler, err := server.Handler("qq")
	if err != nil {
		t.Fatalf("Handler() error = %v", err)
	}

	bigBody := strings.NewReader(strings.Repeat("a", int(maxRequestBodySize)+1))
	req := httptest.NewRequest(http.MethodPost, "/qq/events", bigBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatal("expected non-200 for oversized body")
	}
}

func TestFeishuEventDedup(t *testing.T) {
	cfg := config.BridgeConfig{
		Version: 1,
		Channels: map[string]config.BridgeChannelConfig{
			"feishu": {
				Enabled:    true,
				Driver:     "feishu",
				ListenAddr: "127.0.0.1:19091",
				Path:       "/feishu/events",
			},
		},
	}

	var callCount int
	counter := &countingCompleter{reply: "ok", count: &callCount}
	server := NewServer(cfg, counter, nil, io.Discard)
	handler, err := server.Handler("feishu")
	if err != nil {
		t.Fatalf("Handler() error = %v", err)
	}

	payload := `{"header":{"event_type":"im.message.receive_v1","event_id":"evt-001"},"event":{"message":{"content":"{\"text\":\"hi\"}"},"sender":{"sender_id":{"open_id":"ou_123"}}}}`

	for i := range 3 {
		req := httptest.NewRequest(http.MethodPost, "/feishu/events", strings.NewReader(payload))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want %d", i, rec.Code, http.StatusOK)
		}
	}

	if callCount != 1 {
		t.Fatalf("completer called %d times, want 1 (dedup should suppress duplicates)", callCount)
	}
}

func TestFeishuChallenge(t *testing.T) {
	cfg := config.BridgeConfig{
		Version: 1,
		Channels: map[string]config.BridgeChannelConfig{
			"feishu": {
				Enabled:    true,
				Driver:     "feishu",
				ListenAddr: "127.0.0.1:19091",
				Path:       "/feishu/events",
			},
		},
	}

	server := NewServer(cfg, stubCompleter{reply: "ignored"}, nil, io.Discard)
	handler, err := server.Handler("feishu")
	if err != nil {
		t.Fatalf("Handler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/feishu/events", strings.NewReader(`{"challenge":"abc123"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "abc123") {
		t.Fatalf("expected challenge response, got %s", rec.Body.String())
	}
}

func TestWeComEchoStr(t *testing.T) {
	cfg := config.BridgeConfig{
		Version: 1,
		Channels: map[string]config.BridgeChannelConfig{
			"wecom": {
				Enabled:    true,
				Driver:     "wecom",
				ListenAddr: "127.0.0.1:19092",
				Path:       "/wecom/events",
			},
		},
	}

	server := NewServer(cfg, stubCompleter{reply: "ignored"}, nil, io.Discard)
	handler, err := server.Handler("wecom")
	if err != nil {
		t.Fatalf("Handler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/wecom/events?echostr=hello", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if strings.TrimSpace(rec.Body.String()) != "hello" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "hello")
	}
}
