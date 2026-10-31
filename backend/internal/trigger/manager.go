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
	db           *gorm.DB
	cipher       *security.Cipher
	tasks        *service.TaskService
	allowPrivate bool
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.Mutex
	active       map[uint64]*workerHandle
	clients      map[string]*clientHandle
	wg           sync.WaitGroup
}

type workerHandle struct {
	cancel      context.CancelFunc
	fingerprint string
}

type clientHandle struct {
	client      *redis.Client
	fingerprint string
}

func NewManager(db *gorm.DB, cipher *security.Cipher, tasks *service.TaskService, allowPrivate bool) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		db: db, cipher: cipher, tasks: tasks, allowPrivate: allowPrivate,
		ctx: ctx, cancel: cancel,
		active:  map[uint64]*workerHandle{},
		clients: map[string]*clientHandle{},
	}
}

func (m *Manager) Start() {
	m.wg.Add(1)
	go m.reconcileLoop()
	m.wg.Add(1)
	go m.healthLoop()
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
	for key, handle := range m.clients {
		_ = handle.client.Close()
		delete(m.clients, key)
	}
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

func (m *Manager) healthLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkConnections()
		}
	}
}

func (m *Manager) reconcile() {
	if err := m.pauseInvalidConnectionTriggers(); err != nil {
		log.Printf("trigger reconcile pause invalid connections: %v", err)
	}
	var triggers []model.TriggerSource
	err := m.db.Table("trigger_sources AS ts").
		Joins("JOIN audits a ON a.id = ts.audit_id").
		Joins("JOIN projects p ON p.id = ts.project_id").
		Joins("JOIN redis_connections rc ON rc.id = ts.redis_connection_id").
		Where("ts.type = ? AND ts.status = ?", model.TriggerRedis, "ACTIVE").
		Where("a.status = ? AND a.deleted_at IS NULL AND p.status = ? AND p.deleted_at IS NULL",
			model.AuditActive, model.ProjectActive).
		Where("rc.status = ? AND rc.last_test_ok = ? AND rc.health = ?",
			model.RedisConnectionActive, true, model.RedisHealthOK).
		Select("ts.*").Find(&triggers).Error
	if err != nil {
		log.Printf("trigger reconcile: %v", err)
		return
	}

	connectionIDs := make(map[uint64]bool)
	for _, trigger := range triggers {
		if trigger.RedisConnectionID != nil {
			connectionIDs[*trigger.RedisConnectionID] = true
		}
	}
	connectionMap := map[uint64]model.RedisConnection{}
	if len(connectionIDs) > 0 {
		ids := make([]uint64, 0, len(connectionIDs))
		for id := range connectionIDs {
			ids = append(ids, id)
		}
		var connections []model.RedisConnection
		if err := m.db.Where("id IN ?", ids).Find(&connections).Error; err != nil {
			log.Printf("trigger reconcile connections: %v", err)
			return
		}
		for _, connection := range connections {
			connectionMap[connection.ID] = connection
		}
	}

	clientByTrigger := map[uint64]*redis.Client{}
	clientFingerprints := map[uint64]string{}
	desiredClients := map[string]bool{}
	m.mu.Lock()
	for _, trigger := range triggers {
		if trigger.RedisConnectionID == nil {
			continue
		}
		connection, ok := connectionMap[*trigger.RedisConnectionID]
		if !ok {
			continue
		}
		password := ""
		if connection.PasswordEnc != "" {
			decrypted, decryptErr := m.cipher.Decrypt(connection.PasswordEnc)
			if decryptErr != nil {
				log.Printf("trigger reconcile decrypt connection %s: %v", connection.Name, decryptErr)
				continue
			}
			password = decrypted
		}
		key := clientKey(connection.ID, trigger.RedisDB)
		fp := clientFingerprint(connection, password)
		desiredClients[key] = true
		if existing, ok := m.clients[key]; ok && existing.fingerprint != fp {
			_ = existing.client.Close()
			delete(m.clients, key)
		}
		if _, ok := m.clients[key]; !ok {
			m.clients[key] = &clientHandle{
				client:      NewRedisClient(connection, password, trigger.RedisDB),
				fingerprint: fp,
			}
		}
		clientByTrigger[trigger.ID] = m.clients[key].client
		clientFingerprints[trigger.ID] = fp
	}
	for key, handle := range m.clients {
		if !desiredClients[key] {
			_ = handle.client.Close()
			delete(m.clients, key)
		}
	}

	desired := map[uint64]model.TriggerSource{}
	for _, trigger := range triggers {
		if clientByTrigger[trigger.ID] != nil {
			desired[trigger.ID] = trigger
		}
	}
	for id, handle := range m.active {
		trigger, ok := desired[id]
		if !ok || fingerprint(trigger)+"|"+clientFingerprints[id] != handle.fingerprint {
			handle.cancel()
			delete(m.active, id)
		}
	}
	for _, trigger := range desired {
		if _, ok := m.active[trigger.ID]; ok {
			continue
		}
		ctx, cancel := context.WithCancel(m.ctx)
		fp := fingerprint(trigger) + "|" + clientFingerprints[trigger.ID]
		m.active[trigger.ID] = &workerHandle{cancel: cancel, fingerprint: fp}
		m.wg.Add(1)
		go func(tr model.TriggerSource, client *redis.Client, fp string) {
			defer m.wg.Done()
			defer func() {
				m.mu.Lock()
				if handle, ok := m.active[tr.ID]; ok && handle.fingerprint == fp {
					delete(m.active, tr.ID)
				}
				m.mu.Unlock()
				cancel()
			}()
			m.runRedisWorker(ctx, tr, client)
		}(trigger, clientByTrigger[trigger.ID], fp)
	}
	m.mu.Unlock()
}

func (m *Manager) pauseInvalidConnectionTriggers() error {
	var invalid []model.RedisConnection
	if err := m.db.Where("status = ? OR health <> ? OR last_test_ok = ?",
		model.RedisConnectionDisabled, model.RedisHealthOK, false).Find(&invalid).Error; err != nil {
		return err
	}
	for _, connection := range invalid {
		reason := model.TriggerPauseConnectionUnhealthy
		if connection.Status == model.RedisConnectionDisabled {
			reason = model.TriggerPauseConnectionDisabled
		}
		if err := PauseTriggersForConnection(m.db, connection.ID, reason); err != nil {
			log.Printf("trigger pause connection %s: %v", connection.Name, err)
		}
	}
	return nil
}

func (m *Manager) checkConnections() {
	var connections []model.RedisConnection
	if err := m.db.Where("status = ?", model.RedisConnectionActive).Find(&connections).Error; err != nil {
		log.Printf("trigger health connections: %v", err)
		return
	}
	for _, connection := range connections {
		password := ""
		if connection.PasswordEnc != "" {
			decrypted, err := m.cipher.Decrypt(connection.PasswordEnc)
			if err != nil {
				m.markConnectionUnhealthy(connection, err)
				continue
			}
			password = decrypted
		}
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		latency, err := TestRedisConnection(ctx, connection, password)
		cancel()
		if err != nil {
			m.markConnectionUnhealthy(connection, err)
			continue
		}
		now := time.Now()
		if updateErr := m.db.Model(&model.RedisConnection{}).Where("id = ?", connection.ID).Updates(map[string]any{
			"health":        model.RedisHealthOK,
			"last_test_at":  now,
			"last_test_ok":  true,
			"last_test_ms":  latency.Milliseconds(),
			"last_test_msg": "健康检查通过",
		}).Error; updateErr != nil {
			log.Printf("trigger health update %s: %v", connection.Name, updateErr)
			continue
		}
		if connection.Health == model.RedisHealthUnhealthy && connection.AutoResumeTriggers {
			if err := ResumeTriggersForConnection(m.db, connection.ID, model.TriggerPauseConnectionUnhealthy); err != nil {
				log.Printf("trigger auto resume %s: %v", connection.Name, err)
			}
		}
	}
}

func (m *Manager) markConnectionUnhealthy(connection model.RedisConnection, err error) {
	now := time.Now()
	if updateErr := m.db.Model(&model.RedisConnection{}).Where("id = ?", connection.ID).Updates(map[string]any{
		"health":        model.RedisHealthUnhealthy,
		"last_test_at":  now,
		"last_test_ok":  false,
		"last_test_msg": truncate(err.Error(), 500),
	}).Error; updateErr != nil {
		log.Printf("trigger mark connection unhealthy %s: %v", connection.Name, updateErr)
	}
	if pauseErr := PauseTriggersForConnection(m.db, connection.ID, model.TriggerPauseConnectionUnhealthy); pauseErr != nil {
		log.Printf("trigger pause unhealthy %s: %v", connection.Name, pauseErr)
	}
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
		updates["pause_reason"] = model.TriggerPauseFailureLimit
		updates["auto_paused"] = true
		updates["last_error"] = truncate("连续失败 5 次，已自动暂停："+msg, 500)
	}
	_ = m.db.Model(&model.TriggerSource{}).Where("id = ?", triggerID).Updates(updates).Error
}

func clientKey(connectionID uint64, db int) string {
	return fmt.Sprintf("%d:%d", connectionID, db)
}

func clientFingerprint(connection model.RedisConnection, password string) string {
	return fmt.Sprintf("%s|%s|%s|%t", connection.Addr, connection.Username, password, connection.TLS)
}

func fingerprint(t model.TriggerSource) string {
	connectionID := uint64(0)
	if t.RedisConnectionID != nil {
		connectionID = *t.RedisConnectionID
	}
	return fmt.Sprintf("%d:%s:%s:%d:%d:%s:%s:%s:%d:%d",
		t.ID, t.Type, t.Status, connectionID, t.RedisDB,
		t.Stream, t.ConsumerGroup, t.StartPosition, t.BatchSize, t.MaxInflight)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
