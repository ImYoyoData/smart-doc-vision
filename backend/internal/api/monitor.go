package api

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/metrics"
	"smartdoc-vision/backend/internal/model"
)

// Task status counts are the only monitor value that needs a full table
// aggregate. Cache them so 10s polling from several admins does not keep
// scanning the tasks table.
const monitorTaskCacheTTL = 30 * time.Second

var (
	monitorTaskMu   sync.Mutex
	monitorTaskAt   time.Time
	monitorTaskLast map[string]int64
)

func (s *Server) taskStatusCounts(ctx context.Context) map[string]int64 {
	monitorTaskMu.Lock()
	defer monitorTaskMu.Unlock()
	if monitorTaskLast != nil && time.Since(monitorTaskAt) < monitorTaskCacheTTL {
		return cloneCounts(monitorTaskLast)
	}
	counts := map[string]int64{
		model.TaskQueued: 0, model.TaskProcessing: 0, model.TaskReviewPending: 0,
		model.TaskSuccess: 0, model.TaskFailed: 0,
	}
	type statusCount struct {
		Status string
		Count  int64
	}
	var rows []statusCount
	if err := s.db.WithContext(ctx).Model(&model.Task{}).
		Select("status, count(*) as count").Group("status").Scan(&rows).Error; err == nil {
		for _, item := range rows {
			counts[item.Status] = item.Count
		}
		monitorTaskLast = cloneCounts(counts)
		monitorTaskAt = time.Now()
	}
	return counts
}

func cloneCounts(source map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

type monitorQueueItem struct {
	ProjectID   uint64 `json:"project_id"`
	ProjectName string `json:"project_name"`
	ProjectCode string `json:"project_code"`
	Active      int    `json:"active"`
	Pending     int    `json:"pending"`
	Limit       int    `json:"limit"`
}

// systemMonitor exposes process runtime, HTTP throughput and project worker
// queue metrics for the super-admin monitor page. History is sampled in
// memory every metrics.SampleInterval and kept for metrics.Retention.
func (s *Server) systemMonitor(c *gin.Context) {
	if !s.hasPermission(currentUser(c).Role, "system:monitor") {
		forbidden(c, "当前角色无权查看运行监控")
		return
	}
	rangeKey := strings.ToLower(strings.TrimSpace(c.DefaultQuery("range", "1h")))
	var window time.Duration
	switch rangeKey {
	case "1h":
		window = time.Hour
	case "6h":
		window = 6 * time.Hour
	case "24h":
		window = 24 * time.Hour
	default:
		badRequest(c, "range 仅支持 1h、6h、24h")
		return
	}
	since := time.Now().Add(-window)

	queueStats := s.engine.QueueStats()
	names := map[uint64]model.Project{}
	if len(queueStats) > 0 {
		ids := make([]uint64, 0, len(queueStats))
		for _, stat := range queueStats {
			ids = append(ids, stat.ProjectID)
		}
		var projects []model.Project
		if err := s.db.Select("id", "name", "code").Where("id IN ?", ids).Find(&projects).Error; err == nil {
			for _, project := range projects {
				names[project.ID] = project
			}
		}
	}
	queue := make([]monitorQueueItem, 0, len(queueStats))
	for _, stat := range queueStats {
		project := names[stat.ProjectID]
		name := project.Name
		if name == "" {
			name = "已删除项目"
		}
		queue = append(queue, monitorQueueItem{
			ProjectID: stat.ProjectID, ProjectName: name, ProjectCode: project.Code,
			Active: stat.Active, Pending: stat.Pending, Limit: stat.Limit,
		})
	}
	sort.Slice(queue, func(i, j int) bool {
		left := queue[i].Active + queue[i].Pending
		right := queue[j].Active + queue[j].Pending
		if left != right {
			return left > right
		}
		return queue[i].ProjectID < queue[j].ProjectID
	})

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	dependencies := gin.H{"mysql": true, "redis": true}
	sqlDB, err := s.db.DB()
	if err != nil {
		dependencies["mysql"] = false
	} else if err := sqlDB.PingContext(ctx); err != nil {
		dependencies["mysql"] = false
	}
	if err := s.redis.Ping(ctx).Err(); err != nil {
		dependencies["redis"] = false
	}

	tasks := s.taskStatusCounts(ctx)

	mysqlPool := gin.H{}
	if sqlDB != nil {
		stats := sqlDB.Stats()
		mysqlPool = gin.H{
			"open_connections": stats.OpenConnections,
			"in_use":           stats.InUse,
			"idle":             stats.Idle,
			"max_open":         stats.MaxOpenConnections,
			"wait_count":       stats.WaitCount,
			"wait_duration_ms": stats.WaitDuration.Milliseconds(),
		}
	}
	redisPool := gin.H{}
	if pool := s.redis.PoolStats(); pool != nil {
		redisPool = gin.H{
			"total_conns": pool.TotalConns,
			"idle_conns":  pool.IdleConns,
			"hits":        pool.Hits,
			"misses":      pool.Misses,
			"timeouts":    pool.Timeouts,
		}
	}

	ok(c, gin.H{
		"range":                   rangeKey,
		"since":                   since.Format(time.RFC3339),
		"now":                     time.Now().Format(time.RFC3339),
		"sample_interval_seconds": int(metrics.SampleInterval / time.Second),
		"retention_hours":         int(metrics.Retention / time.Hour),
		"current":                 metrics.Snapshot(),
		"history":                 metrics.History(since, 120),
		"queue":                   queue,
		"tasks":                   tasks,
		"dependencies":            dependencies,
		"mysql_pool":              mysqlPool,
		"redis_pool":              redisPool,
	})
}
