package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/model"
)

func (s *Server) listScriptExecutions(c *gin.Context) {
	audit, err := s.findAudit(c)
	if err != nil {
		return
	}
	page, pageSize := pagination(c)
	q := s.db.Model(&model.ScriptExecution{}).Where("audit_id = ?", audit.ID)
	if status := c.Query("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	if taskNo := c.Query("task_no"); taskNo != "" {
		q = q.Where("task_no LIKE ?", "%"+taskNo+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		serverError(c, err)
		return
	}
	var items []model.ScriptExecution
	if err := q.Order("started_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		serverError(c, err)
		return
	}
	for i := range items {
		items[i].LogText = ""
		items[i].ResultJSON = nil
	}
	ok(c, gin.H{
		"items": items, "total": total,
		"page": page, "page_size": pageSize,
		"has_more": int64(page*pageSize) < total,
	})
}

func (s *Server) getScriptExecution(c *gin.Context) {
	id, okID := parseUint(c.Param("id"))
	if !okID {
		badRequest(c, "脚本执行 ID 无效")
		return
	}
	var execution model.ScriptExecution
	if err := s.db.First(&execution, id).Error; err != nil {
		notFound(c, "脚本执行记录不存在")
		return
	}
	if !s.requireProjectAccess(c, execution.ProjectID) {
		return
	}
	ok(c, execution)
}

func (s *Server) clearAuditScriptExecutions(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "audit:write") {
		forbidden(c, "当前角色无权清理脚本日志")
		return
	}
	audit, err := s.findAudit(c)
	if err != nil {
		return
	}
	if err := s.db.Where("audit_id = ?", audit.ID).Delete(&model.ScriptExecution{}).Error; err != nil {
		serverError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
