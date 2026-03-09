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

func TestQQHandlerReturnsReply(t *testing.T) {
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
	if body["reply"] != "pong" {
		t.Fatalf("reply = %#v, want %q", body["reply"], "pong")
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
