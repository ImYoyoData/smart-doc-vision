package store

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/model"
)

// cleanupLegacyData performs the one-way migration to the project-level Redis
// trigger model. It is idempotent: old platform-Redis bindings and polling
// bindings are removed, and old platform-Redis result actions are dropped.
func cleanupLegacyData(db *gorm.DB) error {
	if err := cleanupLegacyTriggers(db); err != nil {
		return err
	}
	return cleanupLegacyActions(db)
}

func cleanupLegacyTriggers(db *gorm.DB) error {
	var triggers []model.TriggerSource
	if err := db.Where("type = ? OR (type = ? AND redis_connection_id IS NULL AND config_json LIKE ?)",
		"POLLING", model.TriggerRedis, "%\"stream\"%").Find(&triggers).Error; err != nil {
		return err
	}
	if len(triggers) == 0 {
		return nil
	}
	ids := make([]uint64, 0, len(triggers))
	for _, trigger := range triggers {
		ids = append(ids, trigger.ID)
		projectID := trigger.ProjectID
		entry := model.AuditLog{
			ProjectID: &projectID, Operator: "system",
			Action: "LEGACY_TRIGGER_REMOVED", ObjectType: "TRIGGER",
			ObjectID: fmt.Sprint(trigger.ID), Result: "SUCCESS",
			DetailJSON: model.JSONBytes(map[string]any{
				"type": trigger.Type, "audit_id": trigger.AuditID,
				"config": string(trigger.ConfigJSON),
			}),
		}
		if err := db.Create(&entry).Error; err != nil {
			log.Printf("legacy trigger cleanup log %d: %v", trigger.ID, err)
		}
	}
	if err := db.Where("id IN ?", ids).Delete(&model.TriggerSource{}).Error; err != nil {
		return err
	}
	log.Printf("legacy cleanup: removed %d Redis/Polling trigger bindings", len(ids))
	return nil
}

func cleanupLegacyActions(db *gorm.DB) error {
	var audits []model.Audit
	if err := db.Select("id", "project_id", "code", "routes").
		Where("routes LIKE ? OR routes LIKE ?", "%\"mysql\"%", "%\"redis\"%").Find(&audits).Error; err != nil {
		return err
	}
	for _, audit := range audits {
		var routes map[string]map[string]any
		if err := json.Unmarshal(audit.Routes, &routes); err != nil {
			log.Printf("legacy action cleanup: audit %s routes invalid: %v", audit.Code, err)
			continue
		}
		removedMySQL, removedRedis := 0, 0
		for _, route := range routes {
			actions, _ := route["actions"].([]any)
			filtered := make([]any, 0, len(actions))
			for _, raw := range actions {
				action, _ := raw.(map[string]any)
				actionType := strings.ToLower(fmt.Sprint(action["type"]))
				switch actionType {
				case "mysql":
					removedMySQL++
					continue
				case "redis":
					if actionConnectionID(action) == 0 {
						removedRedis++
						continue
					}
				}
				filtered = append(filtered, raw)
			}
			route["actions"] = filtered
		}
		if removedMySQL == 0 && removedRedis == 0 {
			continue
		}
		payload, err := json.Marshal(routes)
		if err != nil {
			return err
		}
		if err := db.Model(&model.Audit{}).Where("id = ?", audit.ID).
			Update("routes", model.JSONText(payload)).Error; err != nil {
			return err
		}
		projectID := audit.ProjectID
		entry := model.AuditLog{
			ProjectID: &projectID, Operator: "system",
			Action: "LEGACY_RESULT_ACTION_REMOVED", ObjectType: "AUDIT",
			ObjectID: audit.Code, Result: "SUCCESS",
			DetailJSON: model.JSONBytes(map[string]any{
				"mysql_actions": removedMySQL, "redis_actions": removedRedis,
			}),
		}
		if err := db.Create(&entry).Error; err != nil {
			log.Printf("legacy action cleanup log %s: %v", audit.Code, err)
		}
	}
	return nil
}

func actionConnectionID(action map[string]any) uint64 {
	switch value := action["connection_id"].(type) {
	case float64:
		if value > 0 {
			return uint64(value)
		}
	case json.Number:
		number, err := value.Int64()
		if err == nil && number > 0 {
			return uint64(number)
		}
	case int:
		if value > 0 {
			return uint64(value)
		}
	}
	return 0
}
