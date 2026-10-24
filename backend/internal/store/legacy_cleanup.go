package store

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/model"
)

// cleanupLegacyMySQLActions removes the retired "mysql" result action from
// existing audit routes. The JS action now covers database writes. The cleanup
// is idempotent and leaves an audit log entry for every changed audit.
func cleanupLegacyMySQLActions(db *gorm.DB) error {
	var audits []model.Audit
	if err := db.Select("id", "project_id", "code", "routes").
		Where("routes LIKE ?", "%\"mysql\"%").Find(&audits).Error; err != nil {
		return err
	}
	for _, audit := range audits {
		var routes map[string]map[string]any
		if err := json.Unmarshal(audit.Routes, &routes); err != nil {
			log.Printf("legacy mysql cleanup: audit %s routes invalid: %v", audit.Code, err)
			continue
		}
		removed := 0
		for _, route := range routes {
			actions, _ := route["actions"].([]any)
			filtered := make([]any, 0, len(actions))
			for _, raw := range actions {
				action, _ := raw.(map[string]any)
				if strings.EqualFold(fmt.Sprint(action["type"]), "mysql") {
					removed++
					continue
				}
				filtered = append(filtered, raw)
			}
			route["actions"] = filtered
		}
		if removed == 0 {
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
			Action: "LEGACY_MYSQL_ACTION_REMOVED", ObjectType: "AUDIT",
			ObjectID: audit.Code, Result: "SUCCESS",
			DetailJSON: model.JSONBytes(map[string]any{"removed_actions": removed}),
		}
		if err := db.Create(&entry).Error; err != nil {
			log.Printf("legacy mysql cleanup: audit log %s: %v", audit.Code, err)
		}
	}
	return nil
}
