package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/goodtiger/openclaw-install/internal/config"
)

type Completer interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

type Server struct {
	cfg       config.BridgeConfig
	completer Completer
	client    *http.Client
	logger    *log.Logger
}

type OpenAICompatibleClient struct {
	provider     config.ProviderConfig
	systemPrompt string
	httpClient   *http.Client
}

func NewServer(cfg config.BridgeConfig, completer Completer, httpClient *http.Client, logOutput io.Writer) *Server {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second}
		if cfg.TimeoutSeconds == 0 {
			httpClient.Timeout = 30 * time.Second
		}
	}
	if logOutput == nil {
		logOutput = io.Discard
	}
	if completer == nil {
		completer = OpenAICompatibleClient{
			provider:     cfg.Provider,
			systemPrompt: cfg.SystemPrompt,
			httpClient:   httpClient,
		}
	}

	return &Server{
		cfg:       cfg,
		completer: completer,
		client:    httpClient,
		logger:    log.New(logOutput, "[bridge] ", log.LstdFlags),
	}
}

func Serve(ctx context.Context, cfg config.BridgeConfig, channel string, logOutput io.Writer) error {
	server := NewServer(cfg, nil, nil, logOutput)
	channelCfg, ok := cfg.Channels[channel]
	if !ok {
		return fmt.Errorf("unknown channel %q", channel)
	}

	handler, err := server.Handler(channel)
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Addr:    channelCfg.ListenAddr,
		Handler: handler,
	}

	errCh := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	go func() {
		server.logger.Printf("starting channel=%s addr=%s path=%s", channel, channelCfg.ListenAddr, channelCfg.Path)
		err := httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	return <-errCh
}

func (s *Server) Handler(channel string) (http.Handler, error) {
	channelCfg, ok := s.cfg.Channels[channel]
	if !ok {
		return nil, fmt.Errorf("unknown channel %q", channel)
	}
	if !channelCfg.Enabled {
		return nil, fmt.Errorf("channel %q is disabled", channel)
	}
	if strings.TrimSpace(channelCfg.Provisioner) != "" && channelCfg.Provisioner != "bridge" {
		return nil, fmt.Errorf("channel %q is configured via %s, not via the bridge server", channel, channelCfg.Provisioner)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"channel": channel,
		})
	})

	switch channel {
	case "qq":
		mux.HandleFunc(channelCfg.Path, s.handleQQ(channelCfg))
	case "feishu":
		mux.HandleFunc(channelCfg.Path, s.handleFeishu(channelCfg))
	case "wecom":
		mux.HandleFunc(channelCfg.Path, s.handleWeCom(channelCfg))
	default:
		return nil, fmt.Errorf("unsupported channel %q", channel)
	}

	return mux, nil
}

func (s *Server) handleQQ(channelCfg config.BridgeChannelConfig) http.HandlerFunc {
	type event struct {
		PostType    string `json:"post_type"`
		MessageType string `json:"message_type"`
		UserID      int64  `json:"user_id"`
		GroupID     int64  `json:"group_id"`
		RawMessage  string `json:"raw_message"`
		Message     string `json:"message"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		accessToken := channelCfg.Fields["access_token"]
		if accessToken != "" {
			headerValues := []string{
				strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "),
				r.Header.Get("X-Self-Token"),
			}
			if accessToken != headerValues[0] && accessToken != headerValues[1] {
				http.Error(w, "invalid access token", http.StatusForbidden)
				return
			}
		}

		var payload event
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}

		text := strings.TrimSpace(payload.RawMessage)
		if text == "" {
			text = strings.TrimSpace(payload.Message)
		}
		reply, err := s.completer.Complete(r.Context(), text)
		if err != nil {
			http.Error(w, "completion failed: "+err.Error(), http.StatusBadGateway)
			return
		}

		if err := s.sendQQReply(r.Context(), channelCfg, payload.MessageType, payload.UserID, payload.GroupID, reply); err != nil {
			s.logger.Printf("qq reply send failed: %v", err)
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    true,
			"reply": reply,
		})
	}
}

func (s *Server) handleFeishu(channelCfg config.BridgeChannelConfig) http.HandlerFunc {
	type envelope struct {
		Challenge string `json:"challenge"`
		Token     string `json:"token"`
		Header    struct {
			EventType string `json:"event_type"`
		} `json:"header"`
		Event struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Sender struct {
				SenderID struct {
					OpenID string `json:"open_id"`
				} `json:"sender_id"`
			} `json:"sender"`
		} `json:"event"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var payload envelope
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}

		if payload.Challenge != "" {
			writeJSON(w, http.StatusOK, map[string]any{"challenge": payload.Challenge})
			return
		}

		if token := channelCfg.Fields["verification_token"]; token != "" && payload.Token != "" && payload.Token != token {
			http.Error(w, "invalid verification token", http.StatusForbidden)
			return
		}

		text := parseFeishuText(payload.Event.Message.Content)
		reply, err := s.completer.Complete(r.Context(), text)
		if err != nil {
			http.Error(w, "completion failed: "+err.Error(), http.StatusBadGateway)
			return
		}

		if err := s.sendFeishuReply(r.Context(), channelCfg, payload.Event.Sender.SenderID.OpenID, reply); err != nil {
			s.logger.Printf("feishu reply send failed: %v", err)
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"code":  0,
			"reply": reply,
		})
	}
}

func (s *Server) handleWeCom(channelCfg config.BridgeChannelConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if echostr := r.URL.Query().Get("echostr"); echostr != "" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(echostr))
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		text := parseWeComText(body)
		reply, err := s.completer.Complete(r.Context(), text)
		if err != nil {
			http.Error(w, "completion failed: "+err.Error(), http.StatusBadGateway)
			return
		}

		if err := s.sendWeComReply(r.Context(), channelCfg, reply); err != nil {
			s.logger.Printf("wecom reply send failed: %v", err)
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    true,
			"reply": reply,
		})
	}
}

func (s *Server) sendQQReply(ctx context.Context, channelCfg config.BridgeChannelConfig, messageType string, userID, groupID int64, reply string) error {
	baseURL := strings.TrimRight(channelCfg.Fields["onebot_url"], "/")
	if baseURL == "" {
		return nil
	}

	endpoint := "/send_private_msg"
	payload := map[string]any{
		"user_id": userID,
		"message": reply,
	}
	if messageType == "group" && groupID != 0 {
		endpoint = "/send_group_msg"
		payload = map[string]any{
			"group_id": groupID,
			"message":  reply,
		}
	}
	return s.postJSON(ctx, baseURL+endpoint, payload, bearerHeader(channelCfg.Fields["access_token"]), nil)
}

func (s *Server) sendFeishuReply(ctx context.Context, channelCfg config.BridgeChannelConfig, openID, reply string) error {
	if openID == "" {
		return nil
	}
	appID := channelCfg.Fields["app_id"]
	appSecret := channelCfg.Fields["app_secret"]
	if appID == "" || appSecret == "" {
		return nil
	}

	token, err := s.fetchFeishuTenantToken(ctx, appID, appSecret)
	if err != nil {
		return err
	}

	content, err := json.Marshal(map[string]string{"text": reply})
	if err != nil {
		return err
	}

	body := map[string]any{
		"receive_id": openID,
		"msg_type":   "text",
		"content":    string(content),
	}

	headers := map[string]string{
		"Authorization": "Bearer " + token,
	}
	return s.postJSON(ctx, "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=open_id", body, headers, nil)
}

func (s *Server) sendWeComReply(ctx context.Context, channelCfg config.BridgeChannelConfig, reply string) error {
	webhookURL := strings.TrimSpace(channelCfg.Fields["webhook_url"])
	if webhookURL == "" {
		return nil
	}
	body := map[string]any{
		"msgtype": "text",
		"text": map[string]string{
			"content": reply,
		},
	}
	return s.postJSON(ctx, webhookURL, body, nil, nil)
}

func (s *Server) fetchFeishuTenantToken(ctx context.Context, appID, appSecret string) (string, error) {
	requestBody := map[string]string{
		"app_id":     appID,
		"app_secret": appSecret,
	}
	responseBody := struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}{}

	if err := s.postJSON(ctx, "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal", requestBody, nil, &responseBody); err != nil {
		return "", err
	}
	if responseBody.TenantAccessToken == "" {
		return "", fmt.Errorf("feishu token response missing tenant_access_token")
	}
	return responseBody.TenantAccessToken, nil
}

func (s *Server) postJSON(ctx context.Context, rawURL string, body any, headers map[string]string, responseTarget any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("request to %s failed with HTTP %d: %s", rawURL, resp.StatusCode, strings.TrimSpace(string(data)))
	}

	if responseTarget != nil {
		return json.NewDecoder(resp.Body).Decode(responseTarget)
	}
	return nil
}

func (c OpenAICompatibleClient) Complete(ctx context.Context, prompt string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = "你好"
	}

	if strings.TrimSpace(c.provider.BaseURL) == "" || strings.TrimSpace(c.provider.PrimaryModel) == "" || strings.TrimSpace(c.provider.APIKey) == "" {
		return "已收到消息：" + prompt, nil
	}

	endpoint := strings.TrimRight(c.provider.BaseURL, "/")
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}

	requestBody := map[string]any{
		"model": c.provider.PrimaryModel,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": valueOrDefault(c.systemPrompt, "You are a concise assistant."),
			},
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature": 0.2,
	}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.provider.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("provider returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	responseBody := struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&responseBody); err != nil {
		return "", err
	}
	if len(responseBody.Choices) == 0 {
		return "", errors.New("provider response did not contain any choices")
	}
	content := strings.TrimSpace(responseBody.Choices[0].Message.Content)
	if content == "" {
		return "", errors.New("provider response was empty")
	}
	return content, nil
}

func parseFeishuText(raw string) string {
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err == nil && strings.TrimSpace(payload.Text) != "" {
		return strings.TrimSpace(payload.Text)
	}
	return strings.TrimSpace(raw)
}

func parseWeComText(raw []byte) string {
	var payload struct {
		Text    string `json:"text"`
		Content string `json:"content"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &payload); err == nil {
		for _, value := range []string{payload.Text, payload.Content, payload.Message} {
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return strings.TrimSpace(string(raw))
}

func bearerHeader(token string) map[string]string {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	return map[string]string{
		"Authorization": "Bearer " + token,
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
