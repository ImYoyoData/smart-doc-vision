package trigger

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/security"
	"smartdoc-vision/backend/internal/service"
)

type Manager struct {
	db     *gorm.DB
	redis  *redis.Client
	cipher *security.Cipher
	tasks  *service.TaskService
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	active map[uint64]*workerHandle
	wg     sync.WaitGroup
}

type workerHandle struct {
	cancel      context.CancelFunc
	fingerprint string
}

func NewManager(db *gorm.DB, cache *redis.Client, cipher *security.Cipher, tasks *service.TaskService) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		db: db, redis: cache, cipher: cipher, tasks: tasks,
		ctx: ctx, cancel: cancel, active: map[uint64]*workerHandle{},
	}
}

func (m *Manager) Start() {
	m.wg.Add(1)
	go m.reconcileLoop()
	log.Println("trigger manager started")
}

func (m *Manager) Stop() {
	m.cancel()
	m.wg.Wait()
	m.mu.Lock()
	for _, h := range m.active {
		h.cancel()
	}
	m.active = map[uint64]*workerHandle{}
	m.mu.Unlock()
}

func (m *Manager) reconcileLoop() {
	defer m.wg.Done()
	m.reconcile()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.reconcile()
		}
	}
}

func (m *Manager) reconcile() {
	var triggers []model.TriggerSource
	err := m.db.Table("trigger_sources AS ts").
		Joins("JOIN audits a ON a.id = ts.audit_id").
		Joins("JOIN projects p ON p.id = ts.project_id").
		Where("ts.status = ? AND a.status = ? AND a.deleted_at IS NULL AND p.status = ? AND p.deleted_at IS NULL",
			"ACTIVE", model.AuditActive, model.ProjectActive).
		Where("ts.type IN ?", []string{model.TriggerRedis, model.TriggerPolling}).
		Select("ts.*").Find(&triggers).Error
	if err != nil {
		log.Printf("trigger reconcile: %v", err)
		return
	}
	desired := map[uint64]model.TriggerSource{}
	for _, t := range triggers {
		desired[t.ID] = t
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, h := range m.active {
		t, ok := desired[id]
		if !ok || fingerprint(t) != h.fingerprint {
			h.cancel()
			delete(m.active, id)
		}
	}
	for _, t := range desired {
		if _, ok := m.active[t.ID]; ok {
			continue
		}
		ctx, cancel := context.WithCancel(m.ctx)
		m.active[t.ID] = &workerHandle{cancel: cancel, fingerprint: fingerprint(t)}
		m.wg.Add(1)
		go func(tr model.TriggerSource, fp string) {
			defer m.wg.Done()
			defer func() {
				m.mu.Lock()
				if h, ok := m.active[tr.ID]; ok && h.fingerprint == fp {
					delete(m.active, tr.ID)
				}
				m.mu.Unlock()
				cancel()
			}()
			switch tr.Type {
			case model.TriggerRedis:
				m.runRedisWorker(ctx, tr)
			case model.TriggerPolling:
				m.runPollingWorker(ctx, tr)
			}
		}(t, fingerprint(t))
	}
}

func fingerprint(t model.TriggerSource) string {
	return fmt.Sprintf("%d:%s:%s:%s:%s", t.ID, t.Type, t.Status, string(t.ConfigJSON), t.SecretEnc)
}

func (m *Manager) touchRun(triggerID uint64, errMsg string, consume bool) {
	updates := map[string]any{
		"last_run_at": time.Now(),
		"last_error":  truncate(errMsg, 500),
	}
	if consume {
		updates["last_consume_at"] = time.Now()
	}
	if errMsg == "" {
		updates["fail_count"] = 0
	}
	_ = m.db.Model(&model.TriggerSource{}).Where("id = ?", triggerID).Updates(updates).Error
}

func (m *Manager) markFailure(triggerID uint64, err error) {
	msg := truncate(err.Error(), 500)
	var trigger model.TriggerSource
	_ = m.db.First(&trigger, triggerID).Error
	trigger.FailCount++
	updates := map[string]any{
		"last_run_at": time.Now(),
		"last_error":  msg,
		"fail_count":  trigger.FailCount,
	}
	if trigger.FailCount >= 5 {
		updates["status"] = "PAUSED"
		updates["last_error"] = truncate("连续失败 5 次，已自动暂停："+msg, 500)
	}
	_ = m.db.Model(&model.TriggerSource{}).Where("id = ?", triggerID).Updates(updates).Error
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
