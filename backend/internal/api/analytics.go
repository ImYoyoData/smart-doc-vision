package api

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/model"
)

type analyticsCounts struct {
	Total   int64
	Pass    int64
	Reject  int64
	Review  int64
	Failed  int64
	Pending int64
	Queued  int64
	AvgMS   float64
}

func (s *Server) analytics(c *gin.Context) {
	days := 7
	rangeKey := strings.TrimSpace(c.DefaultQuery("range", "7d"))
	switch rangeKey {
	case "today":
		days = 1
	case "7d", "":
		days = 7
	case "30d":
		days = 30
	case "90d":
		days = 90
	default:
		if n, err := strconv.Atoi(strings.TrimSuffix(rangeKey, "d")); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}
	now := time.Now()
	from := startOfDay(now).AddDate(0, 0, -(days - 1))
	to := startOfDay(now).AddDate(0, 0, 1)

	projectID := uint64(0)
	if v := c.Query("project_id"); v != "" {
		id, okID := parseUint(v)
		if !okID {
			badRequest(c, "project_id 无效")
			return
		}
		projectID = id
	}
	auditID := uint64(0)
	if v := c.Query("audit_id"); v != "" {
		id, okID := parseUint(v)
		if !okID {
			badRequest(c, "audit_id 无效")
			return
		}
		auditID = id
	}
	user := currentUser(c)
	if !isSuperAdmin(user) {
		if projectID == 0 || !s.hasProjectAccess(user, projectID) {
			forbidden(c, "没有该项目的报表权限")
			return
		}
	}
	if projectID > 0 && !s.hasProjectAccess(user, projectID) {
		forbidden(c, "没有该项目的报表权限")
		return
	}
	scopeTasks := func() *gormQuery {
		q := newGormQuery(s.db.Model(&model.Task{})).
			Where("created_at >= ? AND created_at < ?", from, to)
		if projectID > 0 {
			q = q.Where("project_id = ?", projectID)
		}
		if auditID > 0 {
			q = q.Where("audit_id = ?", auditID)
		}
		return q
	}

	overview := analyticsCounts{}
	db := scopeTasks().DB()
	if err := db.Count(&overview.Total).Error; err != nil {
		serverError(c, err)
		return
	}
	db = scopeTasks().DB()
	_ = db.Where("result = ?", model.ResultPass).Count(&overview.Pass).Error
	db = scopeTasks().DB()
	_ = db.Where("result = ?", model.ResultReject).Count(&overview.Reject).Error
	db = scopeTasks().DB()
	_ = db.Where("result = ?", model.ResultReview).Count(&overview.Review).Error
	db = scopeTasks().DB()
	_ = db.Where("status = ?", model.TaskFailed).Count(&overview.Failed).Error
	db = scopeTasks().DB()
	_ = db.Where("status = ?", model.TaskReviewPending).Count(&overview.Pending).Error
	db = scopeTasks().DB()
	_ = db.Where("status = ?", model.TaskQueued).Count(&overview.Queued).Error
	type avgRow struct {
		AvgMS float64
	}
	var avg avgRow
	db = scopeTasks().DB()
	_ = db.Select("COALESCE(AVG(NULLIF(duration_ms,0)),0) AS avg_ms").Scan(&avg).Error
	overview.AvgMS = avg.AvgMS

	passRate := 0.0
	decided := overview.Pass + overview.Reject
	if decided > 0 {
		passRate = float64(overview.Pass) / float64(decided) * 100
	}
	reviewRate := 0.0
	if overview.Total > 0 {
		reviewRate = float64(overview.Review) / float64(overview.Total) * 100
	}

	trendRows := make([]struct {
		Day    string
		Total  int64
		Pass   int64
		Reject int64
		Review int64
		Failed int64
	}, 0)
	trendQ := s.db.Model(&model.Task{}).
		Select("DATE_FORMAT(created_at, '%Y-%m-%d') AS day, COUNT(*) AS total, "+
			"SUM(CASE WHEN result = 'PASS' THEN 1 ELSE 0 END) AS pass, "+
			"SUM(CASE WHEN result = 'REJECT' THEN 1 ELSE 0 END) AS reject, "+
			"SUM(CASE WHEN result = 'REVIEW' THEN 1 ELSE 0 END) AS review, "+
			"SUM(CASE WHEN status = 'FAILED' THEN 1 ELSE 0 END) AS failed").
		Where("created_at >= ? AND created_at < ?", from, to)
	if projectID > 0 {
		trendQ = trendQ.Where("project_id = ?", projectID)
	}
	if auditID > 0 {
		trendQ = trendQ.Where("audit_id = ?", auditID)
	}
	_ = trendQ.Group("day").Order("day asc").Scan(&trendRows).Error
	trendMap := map[string]struct {
		Day    string
		Total  int64
		Pass   int64
		Reject int64
		Review int64
		Failed int64
	}{}
	for _, row := range trendRows {
		trendMap[row.Day] = row
	}
	trend := make([]gin.H, 0, days)
	for i := 0; i < days; i++ {
		d := from.AddDate(0, 0, i)
		key := d.Format("2006-01-02")
		row := trendMap[key]
		trend = append(trend, gin.H{
			"date": key, "label": d.Format("01-02"),
			"total": row.Total, "pass": row.Pass, "reject": row.Reject,
			"review": row.Review, "failed": row.Failed,
		})
	}

	type auditAgg struct {
		AuditID uint64
		Total   int64
		Pass    int64
		Reject  int64
		Review  int64
		Failed  int64
		Pending int64
		AvgMS   float64
	}
	var auditRows []auditAgg
	auditQ := s.db.Model(&model.Task{}).
		Select("audit_id, COUNT(*) AS total, "+
			"SUM(CASE WHEN result = 'PASS' THEN 1 ELSE 0 END) AS pass, "+
			"SUM(CASE WHEN result = 'REJECT' THEN 1 ELSE 0 END) AS reject, "+
			"SUM(CASE WHEN result = 'REVIEW' THEN 1 ELSE 0 END) AS review, "+
			"SUM(CASE WHEN status = 'FAILED' THEN 1 ELSE 0 END) AS failed, "+
			"SUM(CASE WHEN status = 'REVIEW_PENDING' THEN 1 ELSE 0 END) AS pending, "+
			"COALESCE(AVG(NULLIF(duration_ms,0)),0) AS avg_ms").
		Where("created_at >= ? AND created_at < ?", from, to)
	if projectID > 0 {
		auditQ = auditQ.Where("project_id = ?", projectID)
	}
	if auditID > 0 {
		auditQ = auditQ.Where("audit_id = ?", auditID)
	}
	_ = auditQ.Group("audit_id").Order("total desc").Scan(&auditRows).Error
	auditMap := map[uint64]model.Audit{}
	byAudit := make([]gin.H, 0, len(auditRows))
	for _, row := range auditRows {
		audit, okA := auditMap[row.AuditID]
		if !okA {
			_ = s.db.First(&audit, row.AuditID).Error
			auditMap[row.AuditID] = audit
		}
		rate := 0.0
		if dec := row.Pass + row.Reject; dec > 0 {
			rate = float64(row.Pass) / float64(dec) * 100
		}
		byAudit = append(byAudit, gin.H{
			"audit_id": row.AuditID, "audit_name": audit.Name, "audit_code": audit.Code,
			"project_id": audit.ProjectID, "total": row.Total, "pass": row.Pass,
			"reject": row.Reject, "review": row.Review, "failed": row.Failed,
			"pending": row.Pending, "pass_rate": round1(rate), "avg_duration_ms": int64(row.AvgMS),
		})
	}

	byProject := []gin.H{}
	if projectID == 0 {
		type projectAgg struct {
			ProjectID uint64
			Total     int64
			Pass      int64
			Reject    int64
			Review    int64
			Failed    int64
		}
		var projectRows []projectAgg
		_ = s.db.Model(&model.Task{}).
			Select("project_id, COUNT(*) AS total, "+
				"SUM(CASE WHEN result = 'PASS' THEN 1 ELSE 0 END) AS pass, "+
				"SUM(CASE WHEN result = 'REJECT' THEN 1 ELSE 0 END) AS reject, "+
				"SUM(CASE WHEN result = 'REVIEW' THEN 1 ELSE 0 END) AS review, "+
				"SUM(CASE WHEN status = 'FAILED' THEN 1 ELSE 0 END) AS failed").
			Where("created_at >= ? AND created_at < ?", from, to).
			Group("project_id").Order("total desc").Scan(&projectRows).Error
		for _, row := range projectRows {
			var project model.Project
			_ = s.db.First(&project, row.ProjectID).Error
			rate := 0.0
			if dec := row.Pass + row.Reject; dec > 0 {
				rate = float64(row.Pass) / float64(dec) * 100
			}
			byProject = append(byProject, gin.H{
				"project_id": row.ProjectID, "project_name": project.Name, "project_code": project.Code,
				"total": row.Total, "pass": row.Pass, "reject": row.Reject,
				"review": row.Review, "failed": row.Failed, "pass_rate": round1(rate),
			})
		}
	}

	var samples []model.Task
	sampleQ := s.db.Model(&model.Task{}).
		Select("id, task_no, project_id, audit_id, source_type, biz_key, media_type, status, result, result_json, model_result, rule_trace, error_message, duration_ms, created_at").
		Where("created_at >= ? AND created_at < ?", from, to)
	if projectID > 0 {
		sampleQ = sampleQ.Where("project_id = ?", projectID)
	}
	if auditID > 0 {
		sampleQ = sampleQ.Where("audit_id = ?", auditID)
	}
	_ = sampleQ.Order("created_at desc").Limit(5000).Find(&samples).Error

	labelCounts := map[string]int{}
	reasonCounts := map[string]int{}
	errorCounts := map[string]int{}
	ruleCounts := map[string]int{}
	sourceCounts := map[string]int{}
	mediaCounts := map[string]int{}
	statusCounts := map[string]int{}
	categoryCounts := map[string]int{}
	for _, task := range samples {
		sourceCounts[task.SourceType]++
		mediaCounts[task.MediaType]++
		statusCounts[task.Status]++
		if task.ErrorMessage != "" {
			errorCounts[truncateText(task.ErrorMessage, 120)]++
		}
		var detail map[string]any
		_ = model.DecodeJSON(task.ResultJSON, &detail)
		var modelResult map[string]any
		_ = model.DecodeJSON(task.ModelResult, &modelResult)
		labels := stringSlice(detail["labels"])
		if len(labels) == 0 {
			labels = stringSlice(modelResult["labels"])
		}
		category := firstNonEmpty(
			asString(detail["category_name"]), asString(modelResult["category_name"]),
			asString(detail["category"]), asString(modelResult["category"]),
		)
		if category != "" {
			categoryCounts[category]++
		}
		if task.Result == model.ResultReject || task.Result == model.ResultReview {
			for _, label := range labels {
				labelCounts[label]++
			}
			for _, reason := range stringSlice(detail["reasons"]) {
				reasonCounts[truncateText(reason, 90)]++
			}
			var ruleDetail map[string]any
			_ = model.DecodeJSON(task.RuleTrace, &ruleDetail)
			if matched, okR := ruleDetail["matched_rule"].(string); okR && matched != "" {
				ruleCounts[matched]++
			} else if detail["rule"] != nil {
				if ruleMap, okR := detail["rule"].(map[string]any); okR {
					if matched, okM := ruleMap["matched_rule"].(string); okM && matched != "" {
						ruleCounts[matched]++
					}
				}
			}
		}
	}

	topList := func(counter map[string]int, limit int) []gin.H {
		type entry struct {
			Name  string
			Count int
		}
		entries := make([]entry, 0, len(counter))
		for k, v := range counter {
			if k == "" {
				continue
			}
			entries = append(entries, entry{k, v})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Count > entries[j].Count })
		if len(entries) > limit {
			entries = entries[:limit]
		}
		out := make([]gin.H, 0, len(entries))
		for _, e := range entries {
			out = append(out, gin.H{"name": e.Name, "value": e.Count})
		}
		return out
	}

	var pendingTasks []model.Task
	pendingQ := s.db.Model(&model.Task{}).
		Select("id, task_no, project_id, audit_id, source_type, biz_key, media_type, status, result, provider_id, duration_ms, created_at").
		Where("status = ?", model.TaskReviewPending)
	if projectID > 0 {
		pendingQ = pendingQ.Where("project_id = ?", projectID)
	}
	if auditID > 0 {
		pendingQ = pendingQ.Where("audit_id = ?", auditID)
	}
	_ = pendingQ.Order("created_at asc").Limit(6).Find(&pendingTasks).Error
	s.populateTaskNames(pendingTasks)

	latencyBuckets := []gin.H{
		{"name": "<0.5s", "value": 0}, {"name": "0.5~1s", "value": 0}, {"name": "1~3s", "value": 0},
		{"name": "3~10s", "value": 0}, {"name": ">10s", "value": 0},
	}
	latencyValues := []int{0, 0, 0, 0, 0}
	for _, task := range samples {
		ms := task.DurationMS
		switch {
		case ms <= 0:
			continue
		case ms < 500:
			latencyValues[0]++
		case ms < 1000:
			latencyValues[1]++
		case ms < 3000:
			latencyValues[2]++
		case ms < 10000:
			latencyValues[3]++
		default:
			latencyValues[4]++
		}
	}
	for i := range latencyBuckets {
		latencyBuckets[i]["value"] = latencyValues[i]
	}

	ok(c, gin.H{
		"range": gin.H{"key": rangeKey, "days": days, "from": from, "to": to},
		"scope": gin.H{"project_id": projectID, "audit_id": auditID},
		"overview": gin.H{
			"total": overview.Total, "pass": overview.Pass, "reject": overview.Reject,
			"review": overview.Review, "failed": overview.Failed, "pending": overview.Pending,
			"queued": overview.Queued, "pass_rate": round1(passRate), "review_rate": round1(reviewRate),
			"avg_duration_ms": int64(overview.AvgMS),
		},
		"trend":      trend,
		"by_audit":   byAudit,
		"by_project": byProject,
		"distribution": []gin.H{
			{"key": "PASS", "name": "通过", "value": overview.Pass},
			{"key": "REJECT", "name": "不通过", "value": overview.Reject},
			{"key": "REVIEW", "name": "待议/复核", "value": overview.Review},
			{"key": "FAILED", "name": "执行失败", "value": overview.Failed},
		},
		"reasons": gin.H{
			"labels": topList(labelCounts, 8), "reasons": topList(reasonCounts, 8),
			"rules": topList(ruleCounts, 8), "errors": topList(errorCounts, 8),
		},
		"sources":         topList(sourceCounts, 8),
		"categories":      topList(categoryCounts, 12),
		"media_types":     topList(mediaCounts, 8),
		"statuses":        topList(statusCounts, 10),
		"latency_buckets": latencyBuckets,
		"pending_tasks":   pendingTasks,
		"sample_size":     len(samples),
		"sampled":         len(samples) >= 5000,
		"generated_at":    now,
	})
}

// gormQuery is a tiny wrapper so the analytics builder stays readable while
// still supporting chained GORM conditions.
type gormQuery struct {
	db *gorm.DB
}

func newGormQuery(db *gorm.DB) *gormQuery { return &gormQuery{db: db} }
func (q *gormQuery) Where(query any, args ...any) *gormQuery {
	q.db = q.db.Where(query, args...)
	return q
}
func (q *gormQuery) DB() *gorm.DB { return q.db }

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func stringSlice(v any) []string {
	items, ok := v.([]any)
	if !ok {
		if s, okS := v.(string); okS && s != "" {
			return []string{s}
		}
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, okS := item.(string); okS && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}
