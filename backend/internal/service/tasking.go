package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/ai"
	"smartdoc-vision/backend/internal/engine"
	"smartdoc-vision/backend/internal/media"
	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/textenc"
)

type TaskService struct {
	db           *gorm.DB
	redis        *redis.Client
	engine       *engine.Engine
	mediaLoader  *media.Loader
	allowPrivate bool
}

type SubmitInput struct {
	ProjectID  uint64
	AuditID    uint64
	SourceType string
	SourceID   uint64
	Input      model.TaskInput
}

type SubmitOutcome struct {
	Task       model.Task
	Duplicated bool
}

type SubmitError struct {
	Status  int
	Code    string
	Message string
	Data    any
}

func (e *SubmitError) Error() string { return e.Message }

func NewTaskService(db *gorm.DB, cache *redis.Client, eng *engine.Engine, allowPrivate bool, loader *media.Loader) *TaskService {
	return &TaskService{db: db, redis: cache, engine: eng, allowPrivate: allowPrivate, mediaLoader: loader}
}

func (s *TaskService) Submit(ctx context.Context, in SubmitInput) (*SubmitOutcome, error) {
	if in.ProjectID == 0 || in.AuditID == 0 {
		return nil, submitErr(http.StatusBadRequest, "INVALID_PARAM", "请选择项目和审核配置")
	}
	if in.SourceType == "" {
		in.SourceType = "ADMIN"
	}
	in.SourceType = strings.ToUpper(in.SourceType)
	for i := range in.Input.Media {
		if strings.EqualFold(in.Input.Media[i].Type, "text") {
			in.Input.Media[i].Content = textenc.Normalize(in.Input.Media[i].Content)
		}
		in.Input.Media[i].Name = textenc.Normalize(in.Input.Media[i].Name)
	}
	in.Input.RequestID = strings.TrimSpace(in.Input.RequestID)
	in.Input.SourceRef = strings.TrimSpace(in.Input.SourceRef)
	if in.Input.RequestID != "" {
		if utf8.RuneCountInString(in.Input.RequestID) > 128 {
			return nil, submitErr(http.StatusBadRequest, "INVALID_PARAM", "request_id 不能超过 128 个字符")
		}
		for _, r := range in.Input.RequestID {
			if unicode.IsControl(r) {
				return nil, submitErr(http.StatusBadRequest, "INVALID_PARAM", "request_id 不能包含控制字符")
			}
		}
	}
	if err := ai.ValidateInput(in.Input); err != nil {
		return nil, submitErr(http.StatusBadRequest, "INVALID_PARAM", err.Error())
	}
	if strings.TrimSpace(in.Input.CallbackURL) != "" {
		u := strings.ToLower(strings.TrimSpace(in.Input.CallbackURL))
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			return nil, submitErr(http.StatusBadRequest, "INVALID_PARAM", "回调地址必须以 http:// 或 https:// 开头")
		}
		if !s.allowPrivate {
			if err := media.ValidateOutboundURL(in.Input.CallbackURL); err != nil {
				return nil, submitErr(http.StatusBadRequest, "INVALID_PARAM", "回调地址未通过 SSRF 校验: "+err.Error())
			}
		}
	}
	var project model.Project
	if err := s.db.Where("id = ? AND deleted_at IS NULL", in.ProjectID).First(&project).Error; err != nil {
		return nil, submitErr(http.StatusNotFound, "NOT_FOUND", "项目不存在或已删除")
	}
	if project.Status != model.ProjectActive {
		return nil, submitErr(http.StatusForbidden, "PROJECT_PAUSED", "项目当前不可提交任务")
	}
	requestHash := ""
	if in.Input.RequestID != "" {
		requestHash = requestFingerprint(in.AuditID, in.SourceType, in.SourceID, in.Input)
		if existing := s.lookupByRequestID(in.ProjectID, in.Input.RequestID); existing != nil {
			if existing.RequestHash != "" && existing.RequestHash != requestHash {
				return nil, &SubmitError{
					Status: http.StatusConflict, Code: "REQUEST_ID_CONFLICT",
					Message: "request_id 已被其他提交使用，且审核内容不一致",
					Data:    map[string]any{"request_id": in.Input.RequestID, "task_no": existing.TaskNo},
				}
			}
			return &SubmitOutcome{Task: *existing, Duplicated: true}, nil
		}
	}

	var audit model.Audit
	if err := s.db.Where("id = ? AND project_id = ? AND deleted_at IS NULL", in.AuditID, in.ProjectID).First(&audit).Error; err != nil {
		return nil, submitErr(http.StatusNotFound, "NOT_FOUND", "审核配置不存在或不属于该项目")
	}
	if audit.Status != model.AuditActive {
		return nil, submitErr(http.StatusBadRequest, "AUDIT_NOT_ACTIVE", "审核配置未启用，无法提交任务")
	}
	var provider model.AIProvider
	if err := s.db.First(&provider, audit.ProviderID).Error; err != nil {
		return nil, submitErr(http.StatusUnprocessableEntity, "DEPENDENCY_MISSING", "审核绑定的模型供应商不存在")
	}
	if provider.Status != model.ProviderActive {
		return nil, submitErr(http.StatusUnprocessableEntity, "PROVIDER_DISABLED", "审核绑定的模型供应商已停用")
	}

	window := audit.DedupeWindowSec
	hash, previousHash, lockHash := dedupeHash(in.ProjectID, in.AuditID, in.SourceType, in.SourceID, in.Input, window)
	if in.Input.RequestID == "" && window > 0 && hash != "" {
		if existing := s.lookupDedupePair(ctx, in.ProjectID, hash, previousHash, window); existing != nil {
			return &SubmitOutcome{Task: *existing, Duplicated: true}, nil
		}
		if s.redis != nil {
			lockKey := fmt.Sprintf("sdv:%d:dedupe-lock:%s", in.ProjectID, lockHash)
			locked, lockErr := s.redis.SetNX(ctx, lockKey, "1", 8*time.Second).Result()
			if lockErr == nil && locked {
				defer s.redis.Del(context.Background(), lockKey)
				if existing := s.lookupDedupePair(ctx, in.ProjectID, hash, previousHash, window); existing != nil {
					return &SubmitOutcome{Task: *existing, Duplicated: true}, nil
				}
			} else {
				for i := 0; i < 20; i++ {
					select {
					case <-time.After(100 * time.Millisecond):
					case <-ctx.Done():
						return nil, submitErr(http.StatusRequestTimeout, "REQUEST_TIMEOUT", "请求已取消")
					}
					if existing := s.lookupDedupePair(ctx, in.ProjectID, hash, previousHash, window); existing != nil {
						return &SubmitOutcome{Task: *existing, Duplicated: true}, nil
					}
				}
			}
		}
	}

	if project.MaxTasksPerDay > 0 {
		now := time.Now()
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		var used int64
		_ = s.db.Model(&model.Task{}).
			Where("project_id = ? AND created_at >= ?", project.ID, start).
			Count(&used).Error
		if used >= int64(project.MaxTasksPerDay) {
			return nil, &SubmitError{
				Status: http.StatusTooManyRequests, Code: "PROJECT_QUOTA_EXCEEDED",
				Message: fmt.Sprintf("项目今日任务配额已用完（%d/%d），请明日再试", used, project.MaxTasksPerDay),
				Data:    map[string]any{"used": used, "limit": project.MaxTasksPerDay},
			}
		}
	}

	// Move browser-inline images into the configured storage backend only
	// after dedupe/quota checks, so duplicate submissions do not write files.
	var storedKeys []string
	if s.mediaLoader != nil {
		for i := range in.Input.Media {
			updated, persisted, err := s.mediaLoader.PersistInline(ctx, in.Input.Media[i], in.ProjectID)
			if err != nil {
				return nil, submitErr(http.StatusBadGateway, "STORAGE_ERROR", "图片保存失败: "+err.Error())
			}
			if !persisted {
				continue
			}
			in.Input.Media[i] = updated
			storedKeys = append(storedKeys, updated.Key)
		}
	}

	var requestID *string
	if in.Input.RequestID != "" {
		value := in.Input.RequestID
		requestID = &value
	}
	task := model.Task{
		TaskNo:      NewTaskNo(),
		ProjectID:   in.ProjectID,
		AuditID:     in.AuditID,
		SourceType:  in.SourceType,
		SourceID:    in.SourceID,
		BizKey:      strings.TrimSpace(in.Input.BizKey),
		RequestID:   requestID,
		RequestHash: requestHash,
		DedupeHash:  hash,
		MediaType:   ai.MediaType(in.Input),
		Status:      model.TaskQueued,
		Result:      model.ResultUnknown,
		InputJSON:   model.JSONBytes(in.Input),
		ProviderID:  provider.ID,
	}
	if err := s.db.Create(&task).Error; err != nil {
		if len(storedKeys) > 0 && s.mediaLoader.Store() != nil {
			for _, key := range storedKeys {
				_ = s.mediaLoader.Store().Delete(context.Background(), key)
			}
		}
		if in.Input.RequestID != "" {
			if existing := s.lookupByRequestID(in.ProjectID, in.Input.RequestID); existing != nil {
				if existing.RequestHash != "" && existing.RequestHash != requestHash {
					return nil, &SubmitError{
						Status: http.StatusConflict, Code: "REQUEST_ID_CONFLICT",
						Message: "request_id 已被其他提交使用，且审核内容不一致",
						Data:    map[string]any{"request_id": in.Input.RequestID, "task_no": existing.TaskNo},
					}
				}
				return &SubmitOutcome{Task: *existing, Duplicated: true}, nil
			}
		}
		if hash != "" {
			var existing model.Task
			if findErr := s.db.Where("project_id = ? AND dedupe_hash = ?", in.ProjectID, hash).First(&existing).Error; findErr == nil {
				return &SubmitOutcome{Task: existing, Duplicated: true}, nil
			}
		}
		return nil, submitErr(http.StatusInternalServerError, "INTERNAL_ERROR", "创建任务失败")
	}
	if window > 0 && s.redis != nil {
		_ = s.redis.Set(ctx, fmt.Sprintf("sdv:%d:dedupe:%s", task.ProjectID, hash),
			fmt.Sprintf("%d", task.ID), time.Duration(window)*time.Second).Err()
	}
	enqueueCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.engine.Enqueue(enqueueCtx, task.ProjectID, task.ID); err != nil {
		_ = s.db.Model(&task).Updates(map[string]any{
			"status": model.TaskFailed, "error_message": "任务队列已满或引擎不可用，请稍后重试",
		})
		return nil, submitErr(http.StatusServiceUnavailable, "QUEUE_FULL", "任务队列繁忙，请稍后重试")
	}
	return &SubmitOutcome{Task: task}, nil
}

func (s *TaskService) lookupByRequestID(projectID uint64, requestID string) *model.Task {
	if requestID == "" {
		return nil
	}
	var task model.Task
	if err := s.db.Where("project_id = ? AND request_id = ?", projectID, requestID).First(&task).Error; err != nil {
		return nil
	}
	return &task
}

func requestFingerprint(auditID uint64, sourceType string, sourceID uint64, input model.TaskInput) string {
	return hashPayload(map[string]any{
		"audit_id": auditID, "source_type": sourceType, "source_id": sourceID,
		"biz_key": input.BizKey, "media": input.Media, "ext": input.Ext,
	})
}

func (s *TaskService) lookupDedupedTask(ctx context.Context, projectID uint64, hash string, windowSec int) *model.Task {
	if existing := s.findRecentDuplicate(projectID, hash, windowSec); existing != nil {
		return existing
	}
	if s.redis == nil {
		return nil
	}
	val, err := s.redis.Get(ctx, fmt.Sprintf("sdv:%d:dedupe:%s", projectID, hash)).Result()
	if err != nil {
		return nil
	}
	var id uint64
	if _, err := fmt.Sscanf(val, "%d", &id); err != nil || id == 0 {
		return nil
	}
	var task model.Task
	if s.db.First(&task, id).Error != nil {
		return nil
	}
	return &task
}

func (s *TaskService) lookupDedupePair(ctx context.Context, projectID uint64, currentHash, previousHash string, windowSec int) *model.Task {
	if currentHash != "" {
		if existing := s.lookupDedupedTask(ctx, projectID, currentHash, windowSec); existing != nil {
			return existing
		}
	}
	if previousHash != "" && previousHash != currentHash {
		if existing := s.lookupDedupedTask(ctx, projectID, previousHash, windowSec); existing != nil {
			return existing
		}
	}
	return nil
}

func (s *TaskService) findRecentDuplicate(projectID uint64, hash string, windowSec int) *model.Task {
	cutoff := time.Now().Add(-time.Duration(windowSec) * time.Second)
	var task model.Task
	if err := s.db.Where("project_id = ? AND dedupe_hash = ? AND created_at >= ?", projectID, hash, cutoff).
		Order("id desc").First(&task).Error; err != nil {
		return nil
	}
	return &task
}

// dedupeHash returns the current and previous time-bucket hashes plus a
// bucket-independent lock hash. Checking the previous bucket closes the
// boundary hole where two identical submissions straddle a window boundary.
func dedupeHash(projectID, auditID uint64, sourceType string, sourceID uint64, input model.TaskInput, windowSec int) (current, previous, lock string) {
	if input.BizKey == "" && len(input.Media) == 0 {
		return "", "", ""
	}
	payload := map[string]any{
		"project_id": projectID, "audit_id": auditID,
		"source_type": sourceType, "source_id": sourceID,
		"biz_key": input.BizKey, "media": input.Media,
	}
	if input.RequestID != "" {
		payload["request_id"] = input.RequestID
	}
	if windowSec <= 0 {
		// Dedupe is disabled, but dedupe_hash still participates in a unique
		// index; keep a random nonce so concurrent tasks do not collide.
		payload["nonce"] = uuid.NewString()
		return hashPayload(payload), "", ""
	}
	base, _ := json.Marshal(payload)
	baseSum := sha256.Sum256(base)
	lock = hex.EncodeToString(baseSum[:])
	bucket := time.Now().Unix() / int64(windowSec)
	payload["window"] = bucket
	current = hashPayload(payload)
	payload["window"] = bucket - 1
	previous = hashPayload(payload)
	return current, previous, lock
}

func hashPayload(payload map[string]any) string {
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func NewTaskNo() string {
	now := time.Now()
	return "SDV" + now.Format("20060102150405") + strings.ToUpper(uuid.NewString()[:6])
}

func submitErr(status int, code, message string) *SubmitError {
	return &SubmitError{Status: status, Code: code, Message: message}
}
