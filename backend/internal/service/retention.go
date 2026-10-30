package service

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/model"
)

type RetentionService struct {
	db       *gorm.DB
	interval time.Duration
}

func NewRetentionService(db *gorm.DB) *RetentionService {
	return &RetentionService{db: db, interval: 6 * time.Hour}
}

func (r *RetentionService) Run(ctx context.Context) {
	r.CleanupOnce()
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.CleanupOnce()
		}
	}
}

func (r *RetentionService) CleanupOnce() {
	now := time.Now()
	var projects []model.Project
	if err := r.db.Where("deleted_at IS NOT NULL").Find(&projects).Error; err != nil {
		log.Printf("retention: list deleted projects: %v", err)
		return
	}
	for _, project := range projects {
		if project.DeletedAt == nil {
			continue
		}
		if project.DeletedAt.AddDate(0, 0, project.RetentionDays).After(now) {
			continue
		}
		if err := PurgeProject(r.db, project.ID); err != nil {
			log.Printf("retention: purge project %d: %v", project.ID, err)
			continue
		}
		log.Printf("retention: permanently purged project %d (%s)", project.ID, project.Code)
	}

	var audits []model.Audit
	if err := r.db.Where("deleted_at IS NOT NULL").Find(&audits).Error; err != nil {
		log.Printf("retention: list deleted audits: %v", err)
		return
	}
	projectCache := map[uint64]model.Project{}
	for _, audit := range audits {
		if audit.DeletedAt == nil {
			continue
		}
		project, okProject := projectCache[audit.ProjectID]
		if !okProject {
			if err := r.db.First(&project, audit.ProjectID).Error; err != nil {
				continue
			}
			projectCache[audit.ProjectID] = project
		}
		retention := project.RetentionDays
		if retention <= 0 {
			retention = 30
		}
		if audit.DeletedAt.AddDate(0, 0, retention).After(now) {
			continue
		}
		if err := PurgeAudit(r.db, audit.ID); err != nil {
			log.Printf("retention: purge audit %d: %v", audit.ID, err)
			continue
		}
		log.Printf("retention: permanently purged audit %d (%s)", audit.ID, audit.Code)
	}
}

func PurgeAudit(db *gorm.DB, auditID uint64) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM task_events WHERE task_id IN (SELECT id FROM tasks WHERE audit_id = ?)", auditID).Error; err != nil {
			return err
		}
		if err := tx.Where("audit_id = ?", auditID).Delete(&model.Task{}).Error; err != nil {
			return err
		}
		if err := tx.Where("audit_id = ?", auditID).Delete(&model.TriggerSource{}).Error; err != nil {
			return err
		}
		if err := tx.Where("audit_id = ?", auditID).Delete(&model.ScriptExecution{}).Error; err != nil {
			return err
		}
		if err := tx.Where("audit_id = ?", auditID).Delete(&model.UserAuditScope{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&model.Audit{}, auditID).Error; err != nil {
			return err
		}
		return removeAuditFromCredentials(tx, auditID)
	})
}

func PurgeProject(db *gorm.DB, projectID uint64) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ?", projectID).Delete(&model.TaskEvent{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", projectID).Delete(&model.Task{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", projectID).Delete(&model.TriggerSource{}).Error; err != nil {
			return err
		}
		var auditIDs []uint64
		if err := tx.Model(&model.Audit{}).Where("project_id = ?", projectID).Pluck("id", &auditIDs).Error; err != nil {
			return err
		}
		if len(auditIDs) > 0 {
			if err := tx.Where("audit_id IN ?", auditIDs).Delete(&model.UserAuditScope{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("project_id = ?", projectID).Delete(&model.Audit{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", projectID).Delete(&model.AIProvider{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", projectID).Delete(&model.APICredential{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", projectID).Delete(&model.ScriptExecution{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", projectID).Delete(&model.ProjectSecret{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", projectID).Delete(&model.RedisConnection{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", projectID).Delete(&model.NotificationChannel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", projectID).Delete(&model.ProjectMember{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Project{}, projectID).Error
	})
}

func removeAuditFromCredentials(tx *gorm.DB, auditID uint64) error {
	var credentials []model.APICredential
	if err := tx.Find(&credentials).Error; err != nil {
		return err
	}
	for _, credential := range credentials {
		var ids []uint64
		if err := model.DecodeJSON(credential.AuditIDs, &ids); err != nil {
			continue
		}
		filtered := make([]uint64, 0, len(ids))
		changed := false
		for _, id := range ids {
			if id == auditID {
				changed = true
				continue
			}
			filtered = append(filtered, id)
		}
		if changed {
			if err := tx.Model(&model.APICredential{}).Where("id = ?", credential.ID).
				Update("audit_ids", model.JSONBytes(filtered)).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
