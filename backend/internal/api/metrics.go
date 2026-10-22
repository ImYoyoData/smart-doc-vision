package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/metrics"
	"smartdoc-vision/backend/internal/model"
	appversion "smartdoc-vision/backend/internal/version"
)

func metricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		metrics.ObserveRequest(c.Request.Method, route, c.Writer.Status())
	}
}

func (s *Server) prometheusMetrics(c *gin.Context) {
	lines := []string{
		fmt.Sprintf("# HELP smartdoc_info Build and runtime metadata.\n# TYPE smartdoc_info gauge\nsmartdoc_info{version=%q,redis_db=%q} 1", appversion.Version, fmt.Sprint(s.cfg.RedisDB)),
	}
	type group struct {
		Name  string
		Count int64
	}
	var taskGroups []group
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	if err := s.db.WithContext(ctx).Model(&model.Task{}).Select("status as name, count(*) as count").Group("status").Scan(&taskGroups).Error; err == nil {
		lines = append(lines,
			"# HELP smartdoc_tasks_total Tasks by current status.\n# TYPE smartdoc_tasks_total gauge")
		for _, g := range taskGroups {
			lines = append(lines, fmt.Sprintf("smartdoc_tasks_total{status=%q} %d", g.Name, g.Count))
		}
	}
	var today int64
	start := time.Now().Truncate(24 * time.Hour)
	if err := s.db.WithContext(ctx).Model(&model.Task{}).Where("created_at >= ?", start).Count(&today).Error; err == nil {
		lines = append(lines,
			"# HELP smartdoc_tasks_created_today Tasks created since local midnight.\n# TYPE smartdoc_tasks_created_today gauge",
			fmt.Sprintf("smartdoc_tasks_created_today %d", today))
	}
	if stats := s.engine.QueueStats(); len(stats) > 0 {
		lines = append(lines,
			"# HELP smartdoc_project_worker_active In-flight tasks per project worker pool.\n# TYPE smartdoc_project_worker_active gauge",
			"# HELP smartdoc_project_worker_pending Queued tasks waiting for a project worker slot.\n# TYPE smartdoc_project_worker_pending gauge",
			"# HELP smartdoc_project_worker_limit Configured per-project concurrency limit.\n# TYPE smartdoc_project_worker_limit gauge")
		for _, stat := range stats {
			lines = append(lines,
				fmt.Sprintf("smartdoc_project_worker_active{project_id=\"%d\"} %d", stat.ProjectID, stat.Active),
				fmt.Sprintf("smartdoc_project_worker_pending{project_id=\"%d\"} %d", stat.ProjectID, stat.Pending),
				fmt.Sprintf("smartdoc_project_worker_limit{project_id=\"%d\"} %d", stat.ProjectID, stat.Limit))
		}
	}
	var projects int64
	if err := s.db.WithContext(ctx).Model(&model.Project{}).Where("status = ? AND deleted_at IS NULL", "ACTIVE").Count(&projects).Error; err == nil {
		lines = append(lines,
			"# HELP smartdoc_projects_active Active projects.\n# TYPE smartdoc_projects_active gauge",
			fmt.Sprintf("smartdoc_projects_active %d", projects))
	}
	var audits int64
	if err := s.db.WithContext(ctx).Model(&model.Audit{}).Where("status = ? AND deleted_at IS NULL", "ACTIVE").Count(&audits).Error; err == nil {
		lines = append(lines,
			"# HELP smartdoc_audits_active Active audit configurations.\n# TYPE smartdoc_audits_active gauge",
			fmt.Sprintf("smartdoc_audits_active %d", audits))
	}
	c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	c.String(http.StatusOK, metrics.Render(lines...))
}
