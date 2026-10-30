package api

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/model"
)

// auditScopeIDs returns the explicit audit scope for a user. An empty result
// means no restriction: the user can access every audit inside their projects.
func (s *Server) auditScopeIDs(user *model.User) []uint64 {
	if user == nil || user.Role == model.RoleSuperAdmin {
		return nil
	}
	return user.AuditIDs
}

func (s *Server) canAccessAudit(user *model.User, auditID uint64) bool {
	ids := s.auditScopeIDs(user)
	if len(ids) == 0 {
		return true
	}
	for _, id := range ids {
		if id == auditID {
			return true
		}
	}
	return false
}

func (s *Server) applyAuditScope(q *gorm.DB, user *model.User) *gorm.DB {
	ids := s.auditScopeIDs(user)
	if len(ids) == 0 {
		return q
	}
	return q.Where("id IN ?", ids)
}

func (s *Server) applyTaskAuditScope(q *gorm.DB, user *model.User) *gorm.DB {
	ids := s.auditScopeIDs(user)
	if len(ids) == 0 {
		return q
	}
	return q.Where("audit_id IN ?", ids)
}

func (s *Server) requireAuditScope(c *gin.Context, auditID uint64) bool {
	if s.canAccessAudit(currentUser(c), auditID) {
		return true
	}
	forbidden(c, "当前账号无权访问该审核配置")
	return false
}
