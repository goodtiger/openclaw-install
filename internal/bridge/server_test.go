package bridge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/goodtiger/openclaw-install/internal/config"
	"golang.org/x/time/rate"
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

func TestFeishuRequiresConfiguredVerificationToken(t *testing.T) {
	cfg := config.BridgeConfig{
		Version: 1,
		Channels: map[string]config.BridgeChannelConfig{
			"feishu": {
				Enabled: true,
				Driver:  "feishu",
				Path:    "/feishu/events",
				Fields: map[string]string{
					"verification_token": "expected-token",
				},
			},
		},
	}

	server := NewServer(cfg, stubCompleter{reply: "ignored"}, nil, io.Discard)
	handler, err := server.Handler("feishu")
	if err != nil {
		t.Fatalf("Handler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/feishu/events", strings.NewReader(`{"header":{"event_id":"evt-001"},"event":{"message":{"content":"{\"text\":\"hi\"}"}}}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestWeComEchoStr(t *testing.T) {
	token := "wecom-token"
	echostr := "Hello123"
	signature := calculateWeComSignature(token, "1700000000", "nonce-1", echostr)

	cfg := config.BridgeConfig{
		Version: 1,
		Channels: map[string]config.BridgeChannelConfig{
			"wecom": {
				Enabled:    true,
				Driver:     "wecom",
				ListenAddr: "127.0.0.1:19092",
				Path:       "/wecom/events",
				Fields: map[string]string{
					"token": token,
				},
			},
		},
	}

	server := NewServer(cfg, stubCompleter{reply: "ignored"}, nil, io.Discard)
	handler, err := server.Handler("wecom")
	if err != nil {
		t.Fatalf("Handler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/wecom/events?echostr="+echostr+"&timestamp=1700000000&nonce=nonce-1&msg_signature="+signature, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if strings.TrimSpace(rec.Body.String()) != echostr {
		t.Fatalf("body = %q, want %q", rec.Body.String(), echostr)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("content-type = %q, want %q", got, "text/plain; charset=utf-8")
	}
}

func TestWeComEchoStrRejectsInvalidInput(t *testing.T) {
	token := "wecom-token"
	echostr := "hello<script>"
	signature := calculateWeComSignature(token, "1700000000", "nonce-1", echostr)

	cfg := config.BridgeConfig{
		Version: 1,
		Channels: map[string]config.BridgeChannelConfig{
			"wecom": {
				Enabled: true,
				Driver:  "wecom",
				Path:    "/wecom/events",
				Fields: map[string]string{
					"token": token,
				},
			},
		},
	}

	server := NewServer(cfg, stubCompleter{reply: "ignored"}, nil, io.Discard)
	handler, err := server.Handler("wecom")
	if err != nil {
		t.Fatalf("Handler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/wecom/events?echostr="+echostr+"&timestamp=1700000000&nonce=nonce-1&msg_signature="+signature, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCleanupSeenEventIDsRemovesExpiredEntries(t *testing.T) {
	server := NewServer(config.BridgeConfig{}, stubCompleter{reply: "ok"}, nil, io.Discard)
	server.seenEventIDs.Store("fresh", time.Now())
	server.seenEventIDs.Store("expired", time.Now().Add(-eventDedupeWindow-time.Second))

	server.cleanupSeenEventIDs(time.Now())

	if _, ok := server.seenEventIDs.Load("expired"); ok {
		t.Fatal("expected expired event ID to be removed")
	}
	if _, ok := server.seenEventIDs.Load("fresh"); !ok {
		t.Fatal("expected fresh event ID to remain")
	}
}

func TestHandlerRateLimitReturnsTooManyRequests(t *testing.T) {
	cfg := config.BridgeConfig{
		Version: 1,
		Channels: map[string]config.BridgeChannelConfig{
			"qq": {
				Enabled: true,
				Driver:  "onebot",
				Path:    "/qq/events",
			},
		},
	}

	server := NewServer(cfg, stubCompleter{reply: "pong"}, nil, io.Discard)
	server.limiter = rate.NewLimiter(rate.Every(time.Hour), 1)

	handler, err := server.Handler("qq")
	if err != nil {
		t.Fatalf("Handler() error = %v", err)
	}

	body := `{"post_type":"message","message_type":"private","user_id":1,"raw_message":"ping"}`
	firstReq := httptest.NewRequest(http.MethodPost, "/qq/events", strings.NewReader(body))
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d", firstRec.Code, http.StatusOK)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/qq/events", strings.NewReader(body))
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", secondRec.Code, http.StatusTooManyRequests)
	}
}
