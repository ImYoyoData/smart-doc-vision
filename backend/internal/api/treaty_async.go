package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"smartdoc-vision/backend/internal/model"
)

var auditTreatyMu sync.Mutex

const treatyRawTTL = 7 * 24 * time.Hour

func treatyRawKey(auditID uint64, treatyID string) string {
	return fmt.Sprintf("sdv:treaty:raw:%d:%s", auditID, treatyID)
}

func (s *Server) saveTreatyRaw(auditID uint64, treatyID, rawText string) {
	if s.redis == nil || strings.TrimSpace(rawText) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = s.redis.Set(ctx, treatyRawKey(auditID, treatyID), rawText, treatyRawTTL).Err()
}

func (s *Server) loadTreatyRaw(auditID uint64, treatyID string) (string, error) {
	if s.redis == nil {
		return "", fmt.Errorf("缓存不可用，无法重新解析")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	value, err := s.redis.Get(ctx, treatyRawKey(auditID, treatyID)).Result()
	if err != nil {
		return "", fmt.Errorf("原始文本已过期，请删除后重新上传")
	}
	return value, nil
}

func (s *Server) deleteTreatyRaw(auditID uint64, treatyID string) {
	if s.redis == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = s.redis.Del(ctx, treatyRawKey(auditID, treatyID)).Err()
}

// listAuditTreaties returns the treaty table for an audit. The browser polls
// this endpoint while any document is PARSING.
func (s *Server) listAuditTreaties(c *gin.Context) {
	audit, err := s.findAudit(c)
	if err != nil {
		return
	}
	items := auditTreaties(*audit)
	parsing := 0
	for index := range items {
		if items[index].Status == "PARSING" {
			parsing++
		}
		if items[index].Status == model.TreatyStatusFailed && !items[index].Retryable && treatyFailureRetryable(items[index].Error) {
			items[index].Retryable = true
		}
	}
	ok(c, gin.H{"items": items, "parsing": parsing})
}

// addAuditTreaty persists the document immediately with PARSING status and
// returns; a server-side goroutine compiles it in the background. Closing the
// browser does not stop parsing.
func (s *Server) addAuditTreaty(c *gin.Context) {
	audit, err := s.findAudit(c)
	if err != nil {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, treatyRequestMaxBytes)
	if err := c.Request.ParseMultipartForm(treatyRequestMaxBytes); err != nil {
		badRequest(c, "上传内容过大或格式不正确（单文件不能超过 20MB）")
		return
	}

	name := strings.TrimSpace(c.PostForm("name"))
	sourceType := model.TreatySourceText
	fileName := ""
	mimeType := "text/plain"
	var rawText string
	var extractErr error

	fileHeader, fileErr := c.FormFile("file")
	if fileErr == nil {
		if fileHeader.Size > treatyUploadMaxBytes {
			extractErr = fmt.Errorf("文件不能超过 20MB")
		} else {
			file, openErr := fileHeader.Open()
			if openErr != nil {
				extractErr = fmt.Errorf("无法读取上传文件")
			} else {
				data, readErr := io.ReadAll(io.LimitReader(file, treatyUploadMaxBytes+1))
				_ = file.Close()
				if readErr != nil {
					extractErr = fmt.Errorf("读取上传文件失败")
				} else if len(data) > treatyUploadMaxBytes {
					extractErr = fmt.Errorf("文件不能超过 20MB")
				} else {
					fileName = fileHeader.Filename
					text, detectedMime, parseErr := extractTreatyText(fileName, data)
					rawText = text
					mimeType = detectedMime
					sourceType = model.TreatySourceFile
					if parseErr != nil {
						extractErr = parseErr
					}
				}
			}
		}
		if name == "" {
			name = strings.TrimSuffix(fileHeader.Filename, filepathExt(fileHeader.Filename))
		}
	} else {
		rawText = normalizeTreatyText(c.PostForm("text"))
		if name == "" {
			name = "手动输入条约"
		}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "未命名条约"
	}
	name = truncateRunes(name, 255, "")
	rawText = normalizeTreatyText(rawText)
	rawRunes := len([]rune(rawText))
	record := model.AuditTreaty{
		ID:            uuid.NewString(),
		Name:          name,
		SourceType:    sourceType,
		FileName:      fileName,
		MimeType:      mimeType,
		Status:        model.TreatyStatusPARSING,
		OriginalChars: rawRunes,
		CreatedAt:     time.Now(),
	}
	if extractErr != nil {
		record.Status = model.TreatyStatusFailed
		record.Error = extractErr.Error()
	} else if err := validateTreatyMeaningful(rawText); err != nil {
		record.Status = model.TreatyStatusFailed
		record.Error = err.Error()
	} else if rawRunes > treatyMaxTextRunes {
		record.Status = model.TreatyStatusFailed
		record.Error = fmt.Sprintf("文档内容超过 %d 字，请拆分后上传", treatyMaxTextRunes)
	} else {
		s.saveTreatyRaw(audit.ID, record.ID, rawText)
	}

	providerID, _ := parseUint(c.PostForm("provider_id"))
	if err := s.appendAuditTreaty(audit.ID, record); err != nil {
		s.deleteTreatyRaw(audit.ID, record.ID)
		fail(c, http.StatusUnprocessableEntity, "TREATY_SAVE_FAILED", err.Error(), nil)
		return
	}
	if record.Status == model.TreatyStatusPARSING {
		go s.compileAuditTreatyAsync(audit.ID, record.ID, audit.ProjectID, providerID, name, rawText)
	}
	created(c, gin.H{"treaty": record})
}

func (s *Server) deleteAuditTreaty(c *gin.Context) {
	audit, err := s.findAudit(c)
	if err != nil {
		return
	}
	treatyID := strings.TrimSpace(c.Param("treatyID"))
	if treatyID == "" {
		badRequest(c, "条约 ID 无效")
		return
	}
	found := false
	err = s.mutateAuditTreaties(audit.ID, func(items []model.AuditTreaty) ([]model.AuditTreaty, error) {
		out := make([]model.AuditTreaty, 0, len(items))
		for _, item := range items {
			if item.ID == treatyID {
				found = true
				continue
			}
			out = append(out, item)
		}
		return out, nil
	})
	if err != nil {
		serverError(c, err)
		return
	}
	if !found {
		notFound(c, "审核条约不存在")
		return
	}
	s.deleteTreatyRaw(audit.ID, treatyID)
	ok(c, gin.H{"deleted": true})
}

func (s *Server) reparseAuditTreaty(c *gin.Context) {
	audit, err := s.findAudit(c)
	if err != nil {
		return
	}
	treatyID := strings.TrimSpace(c.Param("treatyID"))
	if treatyID == "" {
		badRequest(c, "条约 ID 无效")
		return
	}
	var current *model.AuditTreaty
	for _, item := range auditTreaties(*audit) {
		if item.ID == treatyID {
			itemCopy := item
			current = &itemCopy
			break
		}
	}
	if current == nil {
		notFound(c, "审核条约不存在")
		return
	}
	if current.Status != model.TreatyStatusFailed || (!current.Retryable && !treatyFailureRetryable(current.Error)) {
		fail(c, http.StatusUnprocessableEntity, "TREATY_NOT_RETRYABLE", "该条约不是可重试的解析失败", nil)
		return
	}
	rawText, err := s.loadTreatyRaw(audit.ID, treatyID)
	if err != nil {
		_ = s.mutateAuditTreaties(audit.ID, func(items []model.AuditTreaty) ([]model.AuditTreaty, error) {
			for index := range items {
				if items[index].ID == treatyID {
					items[index].Retryable = false
					items[index].Error = err.Error()
				}
			}
			return items, nil
		})
		fail(c, http.StatusUnprocessableEntity, "TREATY_RAW_EXPIRED", err.Error(), nil)
		return
	}
	providerID, _ := parseUint(c.PostForm("provider_id"))
	updated := *current
	updated.Status = model.TreatyStatusPARSING
	updated.Summary = ""
	updated.Content = ""
	updated.ContentChars = 0
	updated.ParseMS = 0
	updated.Error = ""
	updated.Retryable = false
	if err := s.mutateAuditTreaties(audit.ID, func(items []model.AuditTreaty) ([]model.AuditTreaty, error) {
		for index := range items {
			if items[index].ID == treatyID {
				items[index] = updated
			}
		}
		return items, nil
	}); err != nil {
		serverError(c, err)
		return
	}
	go s.compileAuditTreatyAsync(audit.ID, treatyID, audit.ProjectID, providerID, updated.Name, rawText)
	ok(c, gin.H{"treaty": updated})
}

func (s *Server) appendAuditTreaty(auditID uint64, treaty model.AuditTreaty) error {
	return s.mutateAuditTreaties(auditID, func(items []model.AuditTreaty) ([]model.AuditTreaty, error) {
		if len(items) >= treatyMaxDocs {
			return nil, fmt.Errorf("单个审核配置最多保存 %d 份审核条约", treatyMaxDocs)
		}
		return append(items, treaty), nil
	})
}

func (s *Server) compileAuditTreatyAsync(auditID uint64, treatyID string, projectID, preferredID uint64, name, rawText string) {
	started := time.Now()
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = s.finishAuditTreaty(auditID, treatyID, model.TreatyStatusFailed, "", "", fmt.Sprintf("解析任务异常: %v", recovered), true, time.Since(started).Milliseconds())
		}
	}()
	candidates, err := s.treatyCandidates(projectID, preferredID)
	if err != nil {
		_ = s.finishAuditTreaty(auditID, treatyID, model.TreatyStatusFailed, "", "", err.Error(), true, time.Since(started).Milliseconds())
		return
	}
	var lastErr error
	anyRetryable := false
	for _, provider := range candidates {
		apiKey, decryptErr := s.cipher.Decrypt(provider.APIKeyEnc)
		if decryptErr != nil {
			lastErr = fmt.Errorf("读取模型 %s 密钥失败: %w", provider.Name, decryptErr)
			anyRetryable = true
			s.markTreatyProviderFailure(provider, lastErr)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), treatyParseTimeout)
		summary, compiled, result, compileErr := compileTreatyDocument(ctx, provider, apiKey, name, rawText)
		cancel()
		if compileErr == nil {
			s.recordProviderUsage(provider, model.UsageSourceTreaty, auditID, result, time.Since(started), nil)
			s.clearTreatyProviderFailure(provider)
			s.deleteTreatyRaw(auditID, treatyID)
			_ = s.finishAuditTreaty(auditID, treatyID, model.TreatyStatusReady, summary, compiled, "", false, time.Since(started).Milliseconds())
			return
		}
		lastErr = compileErr
		s.recordProviderUsage(provider, model.UsageSourceTreaty, auditID, nil, time.Since(started), compileErr)
		if treatyFailureRetryable(compileErr.Error()) {
			anyRetryable = true
			s.markTreatyProviderFailure(provider, compileErr)
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("当前项目没有可用的通用大模型")
		anyRetryable = true
	}
	if !anyRetryable {
		s.deleteTreatyRaw(auditID, treatyID)
	}
	_ = s.finishAuditTreaty(auditID, treatyID, model.TreatyStatusFailed, "", "", lastErr.Error(), anyRetryable, time.Since(started).Milliseconds())
}

func (s *Server) treatyCandidates(projectID, preferredID uint64) ([]model.AIProvider, error) {
	var providers []model.AIProvider
	if err := s.db.Where("project_id = ? AND status = ? AND kind IN ?", projectID, model.ProviderActive,
		[]string{model.ProviderOpenAI, model.ProviderAnthropic}).
		Order("priority asc, id asc").Find(&providers).Error; err != nil {
		return nil, err
	}
	available := make([]model.AIProvider, 0, len(providers))
	cooling := make([]string, 0)
	for _, provider := range providers {
		if provider.BreakerUntil != nil {
			cooling = append(cooling, provider.Name+"（冷却中）")
			continue
		}
		available = append(available, provider)
	}
	if len(available) == 0 {
		if len(cooling) > 0 {
			return nil, fmt.Errorf("项目内通用大模型均在熔断冷却：%s", strings.Join(cooling, "、"))
		}
		return nil, fmt.Errorf("当前项目没有可用的通用大模型（OpenAI 兼容或 Anthropic 协议）")
	}
	if preferredID == 0 {
		return available, nil
	}
	for index := range available {
		if available[index].ID == preferredID {
			preferred := available[index]
			copy(available[1:index+1], available[:index])
			available[0] = preferred
			break
		}
	}
	return available, nil
}

func (s *Server) markTreatyProviderFailure(provider model.AIProvider, err error) {
	failures := provider.FailureCount + 1
	cooldown := 30 * time.Second
	for i := 1; i < failures; i++ {
		cooldown *= 2
		if cooldown >= 10*time.Minute {
			cooldown = 10 * time.Minute
			break
		}
	}
	reason := err.Error()
	if len([]rune(reason)) > 500 {
		reason = string([]rune(reason)[:500])
	}
	_ = s.db.Model(&model.AIProvider{}).Where("id = ?", provider.ID).Updates(map[string]any{
		"breaker_until":  time.Now().Add(cooldown),
		"breaker_reason": reason,
		"failure_count":  failures,
	}).Error
}

func (s *Server) clearTreatyProviderFailure(provider model.AIProvider) {
	if provider.FailureCount == 0 && provider.BreakerUntil == nil {
		return
	}
	_ = s.db.Model(&model.AIProvider{}).Where("id = ?", provider.ID).Updates(map[string]any{
		"breaker_until": nil, "breaker_reason": "", "failure_count": 0,
	}).Error
}

func (s *Server) finishAuditTreaty(auditID uint64, treatyID, status, summary, content, errorText string, retryable bool, parseMS int64) error {
	return s.mutateAuditTreaties(auditID, func(items []model.AuditTreaty) ([]model.AuditTreaty, error) {
		for index := range items {
			if items[index].ID != treatyID {
				continue
			}
			items[index].Status = status
			items[index].Summary = truncateRunes(strings.TrimSpace(summary), 20, "")
			items[index].Content = content
			items[index].ContentChars = len([]rune(content))
			items[index].ParseMS = parseMS
			items[index].Error = truncateRunes(strings.TrimSpace(errorText), 500, "…")
			items[index].Retryable = retryable
			return items, nil
		}
		// The document was deleted while parsing.
		return items, nil
	})
}

func (s *Server) mutateAuditTreaties(auditID uint64, mutate func([]model.AuditTreaty) ([]model.AuditTreaty, error)) error {
	auditTreatyMu.Lock()
	defer auditTreatyMu.Unlock()
	var audit model.Audit
	if err := s.db.Where("id = ? AND deleted_at IS NULL", auditID).First(&audit).Error; err != nil {
		return err
	}
	items := auditTreaties(audit)
	items, err := mutate(items)
	if err != nil {
		return err
	}
	return s.db.Model(&model.Audit{}).Where("id = ?", auditID).
		Update("treaties", model.JSONBytes(items)).Error
}

func auditTreaties(audit model.Audit) []model.AuditTreaty {
	var items []model.AuditTreaty
	_ = model.DecodeJSON(audit.Treaties, &items)
	if items == nil {
		return []model.AuditTreaty{}
	}
	return items
}

func treatyFailureRetryable(message string) bool {
	msg := strings.ToLower(strings.TrimSpace(message))
	if msg == "" {
		return false
	}
	invalidMarkers := []string{
		"没有提取到足够的有效文本", "文本缺少有效规则内容", "文档内容超过",
		"暂不支持", "无法解析", "未能从 docx", "文件不能超过", "未返回有效的条约内容",
	}
	for _, marker := range invalidMarkers {
		if strings.Contains(msg, marker) {
			return false
		}
	}
	transientMarkers := []string{"超时", "timeout", "模型", "密钥", "请求模型失败", "响应", "网络", "连接", "取消"}
	for _, marker := range transientMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func validateTreatyMeaningful(text string) error {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) < treatyMinTextRunes {
		return fmt.Errorf("没有提取到足够的有效文本")
	}
	meaningful := 0
	unique := map[rune]struct{}{}
	for _, item := range runes {
		if unicode.IsLetter(item) || unicode.IsDigit(item) {
			meaningful++
			unique[item] = struct{}{}
		}
	}
	if meaningful < 10 || len(unique) < 4 {
		return fmt.Errorf("文本缺少有效规则内容，请检查文件或输入内容")
	}
	return nil
}

func filepathExt(name string) string {
	index := strings.LastIndexByte(name, '.')
	if index < 0 {
		return ""
	}
	return name[index:]
}

// RecoverInterruptedTreaties marks documents that were still parsing when the
// previous process stopped as retryable failures. The raw text remains in Redis
// so the administrator can retry from the table. This assumes the normal
// single-instance deployment used by this project.
func (s *Server) RecoverInterruptedTreaties() {
	var auditIDs []uint64
	if err := s.db.Model(&model.Audit{}).
		Where("deleted_at IS NULL AND treaties LIKE ?", `%"PARSING"%`).
		Pluck("id", &auditIDs).Error; err != nil {
		return
	}
	for _, auditID := range auditIDs {
		_ = s.mutateAuditTreaties(auditID, func(items []model.AuditTreaty) ([]model.AuditTreaty, error) {
			for index := range items {
				if items[index].Status != model.TreatyStatusPARSING {
					continue
				}
				items[index].Status = model.TreatyStatusFailed
				items[index].Error = "服务重启导致解析中断，可点击重新解析"
				items[index].Retryable = true
				items[index].ParseMS = 0
			}
			return items, nil
		})
	}
}
