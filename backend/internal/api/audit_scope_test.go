package api

import (
	"testing"

	"smartdoc-vision/backend/internal/model"
)

func TestCanAccessAudit(t *testing.T) {
	server := &Server{}
	limited := &model.User{ID: 1, Role: model.RoleViewer, AuditIDs: []uint64{2, 3}}
	if !server.canAccessAudit(limited, 2) {
		t.Fatal("audit in scope should be accessible")
	}
	if server.canAccessAudit(limited, 1) {
		t.Fatal("audit outside scope must be denied")
	}

	unrestricted := &model.User{ID: 2, Role: model.RoleViewer}
	if !server.canAccessAudit(unrestricted, 99) {
		t.Fatal("empty audit scope means all audits")
	}

	superAdmin := &model.User{ID: 3, Role: model.RoleSuperAdmin}
	if !server.canAccessAudit(superAdmin, 99) {
		t.Fatal("super admin should not be restricted")
	}
}
