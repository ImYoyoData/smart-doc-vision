package trigger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	"smartdoc-vision/backend/internal/media"
	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/service"
)

type PollingPage struct {
	Items      []model.TaskInput
	NextCursor string
	RawCount   int
}

func (m *Manager) runPollingWorker(ctx context.Context, trigger model.TriggerSource) {
	var cfg PollingConfig
	_ = model.DecodeJSON(trigger.ConfigJSON, &cfg)
	if cfg.URL == "" {
		return
	}
	if cfg.IntervalSec < 10 {
		cfg.IntervalSec = 60
	}
	secret := ""
	if trigger.SecretEnc != "" {
		if decrypted, err := m.cipher.Decrypt(trigger.SecretEnc); err == nil {
			secret = decrypted
		}
	}
	m.runPollingOnce(ctx, trigger, cfg, secret)
	ticker := time.NewTicker(time.Duration(cfg.IntervalSec) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.runPollingOnce(ctx, trigger, cfg, secret)
		}
	}
}

func (m *Manager) runPollingOnce(ctx context.Context, trigger model.TriggerSource, cfg PollingConfig, secret string) {
	page, err := FetchPollingPage(ctx, cfg, secret, trigger.CursorValue)
	if err != nil {
		m.markFailure(trigger.ID, err)
		return
	}
	submitted := 0
	for _, item := range page.Items {
		if strings.TrimSpace(item.BizKey) == "" {
			item.BizKey = "poll:" + trigger.CursorValue + ":" + fmt.Sprint(submitted)
		}
		_, err := m.tasks.Submit(ctx, service.SubmitInput{
			ProjectID: trigger.ProjectID, AuditID: trigger.AuditID,
			SourceType: model.TriggerPolling, SourceID: trigger.ID, Input: item,
		})
		if err != nil {
			m.markFailure(trigger.ID, err)
			continue
		}
		submitted++
	}
	if page.NextCursor != "" {
		_ = m.db.Model(&model.TriggerSource{}).Where("id = ?", trigger.ID).Update("cursor_value", page.NextCursor).Error
	}
	m.touchRun(trigger.ID, "", submitted > 0)
}

func FetchPollingPage(ctx context.Context, cfg PollingConfig, secret, cursor string) (*PollingPage, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("轮询 URL 不能为空")
	}
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("轮询 URL 格式错误")
	}
	if !cfg.AllowPrivate {
		if err := media.ValidateOutboundURL(cfg.URL); err != nil {
			return nil, fmt.Errorf("轮询 URL 未通过 SSRF 校验: %w", err)
		}
	}
	param := cfg.CursorParam
	if param == "" {
		param = "cursor"
	}
	q := u.Query()
	if cursor != "" {
		q.Set(param, cursor)
	}
	u.RawQuery = q.Encode()
	method := strings.ToUpper(cfg.Method)
	if method == "" {
		method = http.MethodGet
	}
	var bodyReader io.Reader
	if method == http.MethodPost {
		payload, _ := json.Marshal(map[string]any{"cursor": cursor})
		bodyReader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	if cfg.AuthHeader != "" && secret != "" {
		req.Header.Set(cfg.AuthHeader, secret)
	}
	for k, v := range cfg.Headers {
		if strings.TrimSpace(k) != "" {
			req.Header.Set(k, v)
		}
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("轮询请求失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("轮询接口返回 HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	itemsPath := cfg.ItemsPath
	if itemsPath == "" {
		itemsPath = "items"
	}
	result := gjson.GetBytes(raw, itemsPath)
	if !result.IsArray() {
		return nil, fmt.Errorf("轮询响应中 %s 不是数组", itemsPath)
	}
	page := &PollingPage{}
	for _, item := range result.Array() {
		var input model.TaskInput
		if err := json.Unmarshal([]byte(item.Raw), &input); err != nil {
			return nil, fmt.Errorf("轮询条目解析失败: %w", err)
		}
		page.Items = append(page.Items, input)
	}
	page.RawCount = len(page.Items)
	cursorPath := cfg.CursorPath
	if cursorPath == "" {
		cursorPath = "next_cursor"
	}
	page.NextCursor = gjson.GetBytes(raw, cursorPath).String()
	return page, nil
}
