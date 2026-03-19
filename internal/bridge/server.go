package bridge

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/goodtiger/openclaw-install/internal/config"
	"github.com/goodtiger/openclaw-install/internal/shared"
	"golang.org/x/time/rate"
)

const (
	maxRequestBodySize   = 1 << 20 // 1 MB
	eventDedupeWindow    = 5 * time.Minute
	eventCleanupInterval = time.Minute
	maxWeComEchoStrBytes = 64
	requestRateInterval  = 200 * time.Millisecond
	requestRateBurst     = 20
)

type Completer interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

var (
	weComEchoPattern           = regexp.MustCompile(`^[A-Za-z0-9]{1,64}$`)
	errWeComTokenNotConfigured = errors.New("未配置企业微信 Callback Token")
)

type Server struct {
	cfg          config.BridgeConfig
	completer    Completer
	client       *http.Client
	logger       *log.Logger
	seenEventIDs sync.Map // string → time.Time，用于飞书消息去重
	limiter      *rate.Limiter
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
		limiter:   rate.NewLimiter(rate.Every(requestRateInterval), requestRateBurst),
	}
}

// isSeenEvent 检查 eventID 是否在去重窗口内已处理过。
// 首次见到返回 false 并记录；窗口内重复出现返回 true。
func (s *Server) isSeenEvent(eventID string) bool {
	if eventID == "" {
		return false
	}
	now := time.Now()
	if val, ok := s.seenEventIDs.Load(eventID); ok {
		if now.Sub(val.(time.Time)) < eventDedupeWindow {
			return true
		}
		s.seenEventIDs.Delete(eventID)
	}
	s.seenEventIDs.Store(eventID, now)
	return false
}

func (s *Server) cleanupSeenEventIDs(now time.Time) {
	expireBefore := now.Add(-eventDedupeWindow)
	s.seenEventIDs.Range(func(key, value any) bool {
		seenAt, ok := value.(time.Time)
		if !ok || seenAt.Before(expireBefore) {
			s.seenEventIDs.Delete(key)
		}
		return true
	})
}

func (s *Server) runSeenEventCleanup(stop <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(eventCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanupSeenEventIDs(time.Now())
		case <-stop:
			return
		}
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

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go server.runSeenEventCleanup(stop, &wg)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
				server.logger.Printf("关闭服务器时出错：%v", err)
			}
		case <-stop:
		}
	}()

	server.logger.Printf("starting channel=%s addr=%s path=%s", channel, channelCfg.ListenAddr, channelCfg.Path)
	err = httpServer.ListenAndServe()
	close(stop)
	wg.Wait()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// channelHandlerFactory 根据渠道配置生成对应的 HTTP 处理函数。
type channelHandlerFactory func(config.BridgeChannelConfig) http.HandlerFunc

func (s *Server) Handler(channel string) (http.Handler, error) {
	channelCfg, ok := s.cfg.Channels[channel]
	if !ok {
		return nil, fmt.Errorf("未知渠道 %q", channel)
	}
	if !channelCfg.Enabled {
		return nil, fmt.Errorf("渠道 %q 已禁用", channel)
	}
	if strings.TrimSpace(channelCfg.Provisioner) != "" && channelCfg.Provisioner != "bridge" {
		return nil, fmt.Errorf("渠道 %q 通过 %s 配置，不使用 bridge 服务器", channel, channelCfg.Provisioner)
	}

	factories := map[string]channelHandlerFactory{
		"qq":     s.handleQQ,
		"feishu": s.handleFeishu,
		"wecom":  s.handleWeCom,
	}
	factory, ok := factories[channel]
	if !ok {
		return nil, fmt.Errorf("不支持的渠道 %q", channel)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"channel": channel,
		})
	})
	mux.Handle(channelCfg.Path, s.withRateLimit(factory(channelCfg)))

	return mux, nil
}

func (s *Server) withRateLimit(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.limiter != nil && !s.limiter.Allow() {
			http.Error(w, "请求过于频繁", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
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
			http.Error(w, "不支持的请求方法", http.StatusMethodNotAllowed)
			return
		}

		accessToken := channelCfg.Fields["access_token"]
		if accessToken != "" {
			headerValues := []string{
				strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "),
				r.Header.Get("X-Self-Token"),
			}
			if accessToken != headerValues[0] && accessToken != headerValues[1] {
				http.Error(w, "无效的 access token", http.StatusForbidden)
				return
			}
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
		var payload event
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "无效的请求体", http.StatusBadRequest)
			return
		}

		text := strings.TrimSpace(payload.RawMessage)
		if text == "" {
			text = strings.TrimSpace(payload.Message)
		}
		reply, err := s.completer.Complete(r.Context(), text)
		if err != nil {
			http.Error(w, "模型调用失败", http.StatusBadGateway)
			return
		}

		if err := s.sendQQReply(r.Context(), channelCfg, payload.MessageType, payload.UserID, payload.GroupID, reply); err != nil {
			s.logger.Printf("qq 回复发送失败：%v", err)
		}

		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func (s *Server) handleFeishu(channelCfg config.BridgeChannelConfig) http.HandlerFunc {
	type envelope struct {
		Challenge string `json:"challenge"`
		Token     string `json:"token"`
		Header    struct {
			EventType string `json:"event_type"`
			EventID   string `json:"event_id"`
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
			http.Error(w, "不支持的请求方法", http.StatusMethodNotAllowed)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
		var payload envelope
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "无效的请求体", http.StatusBadRequest)
			return
		}

		if payload.Challenge != "" {
			writeJSON(w, http.StatusOK, map[string]any{"challenge": payload.Challenge})
			return
		}

		if token := channelCfg.Fields["verification_token"]; token != "" && payload.Token != token {
			http.Error(w, "验证 token 无效", http.StatusForbidden)
			return
		}

		// 飞书采用 at-least-once 投递，用 event_id 去重
		if s.isSeenEvent(payload.Header.EventID) {
			writeJSON(w, http.StatusOK, map[string]any{"code": 0})
			return
		}

		text := parseFeishuText(payload.Event.Message.Content)
		reply, err := s.completer.Complete(r.Context(), text)
		if err != nil {
			http.Error(w, "模型调用失败", http.StatusBadGateway)
			return
		}

		if err := s.sendFeishuReply(r.Context(), channelCfg, payload.Event.Sender.SenderID.OpenID, reply); err != nil {
			s.logger.Printf("飞书回复发送失败：%v", err)
		}

		writeJSON(w, http.StatusOK, map[string]any{"code": 0})
	}
}

func (s *Server) handleWeCom(channelCfg config.BridgeChannelConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if echostr := r.URL.Query().Get("echostr"); echostr != "" {
				if err := validateWeComEchoStr(echostr); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				if err := validateWeComRequestSignature(channelCfg, r, nil); err != nil {
					writeWeComAuthError(w, err)
					return
				}
				writePlainText(w, http.StatusOK, echostr)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "不支持的请求方法", http.StatusMethodNotAllowed)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "读取请求体失败", http.StatusBadRequest)
			return
		}
		if err := validateWeComRequestSignature(channelCfg, r, body); err != nil {
			writeWeComAuthError(w, err)
			return
		}

		text := parseWeComText(body)
		reply, err := s.completer.Complete(r.Context(), text)
		if err != nil {
			http.Error(w, "模型调用失败", http.StatusBadGateway)
			return
		}

		if err := s.sendWeComReply(r.Context(), channelCfg, reply); err != nil {
			s.logger.Printf("企微回复发送失败：%v", err)
		}

		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
				"content": shared.ValueOrDefault(c.systemPrompt, "You are a concise assistant."),
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

	var xmlPayload struct {
		Text    string `xml:"Text"`
		Content string `xml:"Content"`
		Message string `xml:"Message"`
	}
	if err := xml.Unmarshal(raw, &xmlPayload); err == nil {
		for _, value := range []string{xmlPayload.Text, xmlPayload.Content, xmlPayload.Message} {
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
	data, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "JSON 编码失败", http.StatusInternalServerError)
		return
	}
	data = append(data, '\n')

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if _, err := w.Write(data); err != nil {
		log.Printf("writeJSON: %v", err)
	}
}

func writePlainText(w http.ResponseWriter, statusCode int, payload string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(statusCode)
	if _, err := io.WriteString(w, payload); err != nil {
		log.Printf("writePlainText: %v", err)
	}
}

func validateWeComEchoStr(echostr string) error {
	if len(echostr) > maxWeComEchoStrBytes {
		return fmt.Errorf("echostr 过长")
	}
	if !weComEchoPattern.MatchString(echostr) {
		return fmt.Errorf("echostr 只能包含字母和数字")
	}
	return nil
}

func validateWeComRequestSignature(channelCfg config.BridgeChannelConfig, r *http.Request, body []byte) error {
	token := strings.TrimSpace(channelCfg.Fields["token"])
	if token == "" {
		return errWeComTokenNotConfigured
	}

	signature := strings.TrimSpace(r.URL.Query().Get("msg_signature"))
	timestamp := strings.TrimSpace(r.URL.Query().Get("timestamp"))
	nonce := strings.TrimSpace(r.URL.Query().Get("nonce"))
	if signature == "" || timestamp == "" || nonce == "" {
		return errors.New("缺少企业微信签名参数")
	}

	expected := calculateWeComSignature(token, timestamp, nonce, weComSignaturePayload(r, body))
	if subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) != 1 {
		return errors.New("企业微信签名校验失败")
	}
	return nil
}

func calculateWeComSignature(token, timestamp, nonce, payload string) string {
	parts := []string{token, timestamp, nonce, payload}
	slices.Sort(parts)
	sum := sha1.Sum([]byte(strings.Join(parts, "")))
	return hex.EncodeToString(sum[:])
}

func weComSignaturePayload(r *http.Request, body []byte) string {
	if r.Method == http.MethodGet {
		return r.URL.Query().Get("echostr")
	}
	if len(body) == 0 {
		return ""
	}
	if encrypt := extractWeComEncrypt(body); encrypt != "" {
		return encrypt
	}
	return string(body)
}

func extractWeComEncrypt(body []byte) string {
	var jsonPayload struct {
		EncryptLower string `json:"encrypt"`
		EncryptUpper string `json:"Encrypt"`
	}
	if err := json.Unmarshal(body, &jsonPayload); err == nil {
		for _, value := range []string{jsonPayload.EncryptLower, jsonPayload.EncryptUpper} {
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}

	var xmlPayload struct {
		Encrypt string `xml:"Encrypt"`
	}
	if err := xml.Unmarshal(body, &xmlPayload); err == nil && strings.TrimSpace(xmlPayload.Encrypt) != "" {
		return strings.TrimSpace(xmlPayload.Encrypt)
	}
	return ""
}

func writeWeComAuthError(w http.ResponseWriter, err error) {
	if errors.Is(err, errWeComTokenNotConfigured) {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	http.Error(w, err.Error(), http.StatusForbidden)
}
