package api

import (
	"sort"
	"time"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/model"
)

func (s *Server) dashboard(c *gin.Context) {
	var projectID *uint64
	if v := c.Query("project_id"); v != "" {
		id, okID := parseUint(v)
		if !okID {
			badRequest(c, "project_id 无效")
			return
		}
		if !s.requireProjectAccess(c, id) {
			return
		}
		projectID = &id
	}
	user := currentUser(c)
	if projectID == nil && !isSuperAdmin(user) {
		forbidden(c, "非超级管理员查看仪表盘时必须选择项目")
		return
	}
	now := time.Now()
	today := startOfToday()
	yesterday := today.AddDate(0, 0, -1)
	weekAgo := today.AddDate(0, 0, -6)
	d7 := today.AddDate(0, 0, -6)

	countTasks := func() int64 {
		q := s.db.Model(&model.Task{}).Where("created_at >= ?", today)
		if projectID != nil {
			q = q.Where("project_id = ?", *projectID)
		}
		q = s.applyTaskAuditScope(q, user)
		var n int64
		_ = q.Count(&n).Error
		return n
	}
	todayTasks := countTasks()
	countBetween := func(from, to time.Time) int64 {
		q := s.db.Model(&model.Task{}).Where("created_at >= ? AND created_at < ?", from, to)
		if projectID != nil {
			q = q.Where("project_id = ?", *projectID)
		}
		q = s.applyTaskAuditScope(q, user)
		var n int64
		_ = q.Count(&n).Error
		return n
	}
	yesterdayTasks := countBetween(yesterday, today)

	var pending int64
	pendingQ := s.db.Model(&model.Task{}).Where("status = ?", model.TaskReviewPending)
	if projectID != nil {
		pendingQ = pendingQ.Where("project_id = ?", *projectID)
	}
	pendingQ = s.applyTaskAuditScope(pendingQ, user)
	_ = pendingQ.Count(&pending).Error

	var failedToday int64
	failQ := s.db.Model(&model.Task{}).Where("status = ? AND created_at >= ?", model.TaskFailed, today)
	if projectID != nil {
		failQ = failQ.Where("project_id = ?", *projectID)
	}
	failQ = s.applyTaskAuditScope(failQ, user)
	_ = failQ.Count(&failedToday).Error

	var activeAudits int64
	auditQ := s.db.Model(&model.Audit{}).Where("status = ? AND deleted_at IS NULL", model.AuditActive)
	if projectID != nil {
		auditQ = auditQ.Where("project_id = ?", *projectID)
	}
	auditQ = s.applyAuditScope(auditQ, user)
	_ = auditQ.Count(&activeAudits).Error

	var activeProjects int64
	_ = s.db.Model(&model.Project{}).Where("status = ? AND deleted_at IS NULL", model.ProjectActive).Count(&activeProjects).Error

	type trendRow struct {
		Day    string
		Total  int64
		Pass   int64
		Reject int64
		Review int64
		Failed int64
		AvgMS  float64
	}
	var rows []trendRow
	q := s.db.Table("tasks").
		Select("DATE_FORMAT(created_at, '%Y-%m-%d') AS day, COUNT(*) AS total, "+
			"SUM(CASE WHEN result = 'PASS' THEN 1 ELSE 0 END) AS pass, "+
			"SUM(CASE WHEN result = 'REJECT' THEN 1 ELSE 0 END) AS reject, "+
			"SUM(CASE WHEN result = 'REVIEW' THEN 1 ELSE 0 END) AS review, "+
			"SUM(CASE WHEN status = 'FAILED' THEN 1 ELSE 0 END) AS failed, "+
			"COALESCE(AVG(NULLIF(duration_ms,0)),0) AS avg_ms").
		Where("created_at >= ?", d7)
	if projectID != nil {
		q = q.Where("project_id = ?", *projectID)
	}
	q = s.applyTaskAuditScope(q, user)
	if err := q.Group("day").Order("day asc").Scan(&rows).Error; err != nil {
		serverError(c, err)
		return
	}
	byDay := map[string]trendRow{}
	var total7, pass7, reject7, review7, failed7, durationSum, durationCount int64
	for _, row := range rows {
		byDay[row.Day] = row
		total7 += row.Total
		pass7 += row.Pass
		reject7 += row.Reject
		review7 += row.Review
		failed7 += row.Failed
		if row.AvgMS > 0 {
			durationSum += int64(row.AvgMS * float64(row.Total))
			durationCount += row.Total
		}
	}
	avgDuration := int64(0)
	if durationCount > 0 {
		avgDuration = durationSum / durationCount
	}
	type trendPoint struct {
		Date   string `json:"date"`
		Label  string `json:"label"`
		Total  int64  `json:"total"`
		Pass   int64  `json:"pass"`
		Reject int64  `json:"reject"`
		Review int64  `json:"review"`
		Failed int64  `json:"failed"`
	}
	trend := make([]trendPoint, 0, 7)
	for i := 0; i < 7; i++ {
		d := weekAgo.AddDate(0, 0, i)
		key := d.Format("2006-01-02")
		row := byDay[key]
		trend = append(trend, trendPoint{
			Date: key, Label: d.Format("01-02"), Total: row.Total,
			Pass: row.Pass, Reject: row.Reject, Review: row.Review, Failed: row.Failed,
		})
	}
	passRate := 0.0
	decided := pass7 + reject7
	if decided > 0 {
		passRate = float64(pass7) / float64(decided) * 100
	}

	recentQ := s.db.Model(&model.Task{}).
		Select("id, task_no, project_id, audit_id, source_type, biz_key, media_type, status, result, provider_id, error_message, retry_count, duration_ms, started_at, finished_at, created_at, updated_at").
		Order("created_at desc").Limit(8)
	if projectID != nil {
		recentQ = recentQ.Where("project_id = ?", *projectID)
	}
	recentQ = s.applyTaskAuditScope(recentQ, user)
	var recent []model.Task
	_ = recentQ.Find(&recent).Error
	s.populateTaskNames(recent)

	providerQ := s.db.Model(&model.AIProvider{}).Order("id asc")
	if projectID != nil {
		providerQ = providerQ.Where("project_id = ?", *projectID)
	}
	var providers []model.AIProvider
	_ = providerQ.Find(&providers).Error
	providerItems := make([]gin.H, 0, len(providers))
	for _, p := range providers {
		providerItems = append(providerItems, gin.H{
			"id": p.ID, "name": p.Name, "kind": p.Kind, "model": p.Model, "status": p.Status,
			"last_test_ok": p.LastTestOK, "last_test_at": p.LastTestAt,
			"last_test_ms": p.LastTestMS, "project_id": p.ProjectID,
		})
	}
	_ = model.Project{}
	dist := []gin.H{
		{"key": "PASS", "name": "通过", "value": pass7},
		{"key": "REJECT", "name": "不通过", "value": reject7},
		{"key": "REVIEW", "name": "待议/复核", "value": review7},
		{"key": "FAILED", "name": "执行失败", "value": failed7},
	}
	sort.Slice(dist, func(i, j int) bool { return dist[i]["value"].(int64) > dist[j]["value"].(int64) })

	ok(c, gin.H{
		"overview": gin.H{
			"today_tasks":     todayTasks,
			"yesterday_tasks": yesterdayTasks,
			"tasks_7d":        total7,
			"pass_rate":       round1(passRate),
			"review_pending":  pending,
			"failed_today":    failedToday,
			"avg_duration_ms": avgDuration,
			"active_audits":   activeAudits,
			"active_projects": activeProjects,
		},
		"trend":        trend,
		"distribution": dist,
		"recent_tasks": recent,
		"providers":    providerItems,
		"generated_at": now,
	})
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}
