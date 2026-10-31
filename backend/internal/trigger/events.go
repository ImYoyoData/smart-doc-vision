package trigger

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"smartdoc-vision/backend/internal/media"
	"smartdoc-vision/backend/internal/model"
)

// sendEvent delivers trigger-level notifications that happen before or
// without a task result: accepted, deduplicated and submit_failed. It reuses
// the audit callback URL/secret/headers but never writes to the business Redis.
func (m *Manager) sendEvent(ctx context.Context, projectID, auditID uint64, input model.TaskInput, event string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["event"] = event
	var project model.Project
	if err := m.db.First(&project, projectID).Error; err != nil {
		return nil
	}
	var audit model.Audit
	if err := m.db.First(&audit, auditID).Error; err != nil {
		return nil
	}
	// Trigger-level events are opt-in through the message callback_url. Do not
	// fall back to the audit callback: existing consumers configured for
	// audit.completed must not suddenly receive accepted/deduplicated events.
	targetURL := strings.TrimSpace(input.CallbackURL)
	if targetURL == "" {
		return nil
	}
	payload["project_id"] = project.ID
	payload["project_code"] = project.Code
	payload["project_name"] = project.Name
	payload["audit_id"] = audit.ID
	payload["audit_code"] = audit.Code
	payload["audit_name"] = audit.Name
	ws := strings.HasPrefix(strings.ToLower(targetURL), "ws://") || strings.HasPrefix(strings.ToLower(targetURL), "wss://")
	if !m.allowPrivate {
		var err error
		if ws {
			err = validateTriggerWSURL(targetURL)
		} else {
			err = media.ValidateOutboundURL(targetURL)
		}
		if err != nil {
			return fmt.Errorf("回调地址未通过 SSRF 校验: %w", err)
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	secret := ""
	if audit.CallbackSecretEnc != "" {
		if decrypted, decryptErr := m.cipher.Decrypt(audit.CallbackSecretEnc); decryptErr == nil {
			secret = decrypted
		}
	}
	var customHeaders map[string]string
	_ = model.DecodeJSON(audit.CallbackHeaders, &customHeaders)
	headers := map[string]string{
		"Content-Type":         "application/json",
		"User-Agent":           "SmartDoc-Vision-Event/1.0",
		"X-SmartDoc-Event":     event,
		"X-SmartDoc-Timestamp": strconv.FormatInt(time.Now().UnixMilli(), 10),
	}
	if taskNo, ok := payload["task_no"].(string); ok && taskNo != "" {
		headers["X-SmartDoc-Task"] = taskNo
		headers["X-Idempotency-Key"] = event + ":" + taskNo
	}
	if requestID := strings.TrimSpace(input.RequestID); requestID != "" {
		headers["X-SmartDoc-Request-Id"] = requestID
		if headers["X-Idempotency-Key"] == "" {
			headers["X-Idempotency-Key"] = event + ":" + requestID
		}
	}
	if headers["X-Idempotency-Key"] == "" {
		headers["X-Idempotency-Key"] = fmt.Sprintf("%s:%d", event, time.Now().UnixNano())
	}
	if result, ok := payload["result"].(string); ok && result != "" {
		headers["X-SmartDoc-Result"] = result
	}
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(raw)
		headers["X-SmartDoc-Signature"] = "sha256=" + hex.EncodeToString(mac.Sum(nil))
	}
	for key, value := range customHeaders {
		if strings.TrimSpace(key) != "" {
			headers[key] = value
		}
	}
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if ws {
			lastErr = sendTriggerWS(ctx, targetURL, headers, string(raw))
		} else {
			lastErr = sendTriggerHTTP(ctx, targetURL, headers, raw)
		}
		if lastErr == nil {
			return nil
		}
		if attempt < 3 {
			select {
			case <-time.After(time.Duration(1<<uint(attempt-1)) * time.Second):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	log.Printf("trigger event %s task callback %s failed: %v", event, targetURL, lastErr)
	return lastErr
}

func sendTriggerHTTP(ctx context.Context, target string, headers map[string]string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateTriggerText(string(raw), 300))
	}
	return nil
}

func sendTriggerWS(ctx context.Context, target string, headers map[string]string, body string) error {
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	httpHeaders := http.Header{}
	for key, value := range headers {
		httpHeaders.Set(key, value)
	}
	conn, _, err := dialer.DialContext(ctx, target, httpHeaders)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := conn.WriteMessage(websocket.TextMessage, []byte(body)); err != nil {
		return err
	}
	return nil
}

func validateTriggerWSURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("WebSocket 地址无效: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	default:
		return fmt.Errorf("WebSocket 地址仅支持 ws/wss")
	}
	return media.ValidateOutboundURL(u.String())
}

func truncateTriggerText(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}
