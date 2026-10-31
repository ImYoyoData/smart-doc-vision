package api

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/model"
)

func (s *Server) exportAuditTasks(c *gin.Context) {
	audit, err := s.findAudit(c)
	if err != nil {
		return
	}
	q := s.db.Where("project_id = ? AND audit_id = ?", audit.ProjectID, audit.ID)
	if v := strings.TrimSpace(c.Query("start")); v != "" {
		t, err := time.ParseInLocation("2006-01-02", v, time.Local)
		if err != nil {
			badRequest(c, "开始日期格式应为 YYYY-MM-DD")
			return
		}
		q = q.Where("created_at >= ?", t)
	}
	if v := strings.TrimSpace(c.Query("end")); v != "" {
		t, err := time.ParseInLocation("2006-01-02", v, time.Local)
		if err != nil {
			badRequest(c, "结束日期格式应为 YYYY-MM-DD")
			return
		}
		q = q.Where("created_at < ?", t.AddDate(0, 0, 1))
	}
	if v := c.Query("status"); v != "" {
		q = q.Where("status IN ?", splitList(v))
	}
	if v := c.Query("result"); v != "" {
		q = q.Where("result IN ?", splitList(v))
	}
	if v := c.Query("source_type"); v != "" {
		q = q.Where("source_type IN ?", splitList(v))
	}
	if v := c.Query("media_type"); v != "" {
		q = q.Where("media_type IN ?", splitList(v))
	}
	if v := strings.TrimSpace(c.Query("keyword")); v != "" {
		like := "%" + v + "%"
		q = q.Where("(task_no LIKE ? OR biz_key LIKE ? OR request_id LIKE ?)", like, like, like)
	}
	if v := strings.TrimSpace(c.Query("biz_key")); v != "" {
		q = q.Where("biz_key = ?", v)
	}
	var tasks []model.Task
	if err := q.Order("created_at desc").Limit(100000).Find(&tasks).Error; err != nil {
		serverError(c, err)
		return
	}
	filename := fmt.Sprintf("smartdoc-%s-%s.csv", audit.Code, time.Now().Format("20060102"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename="+filename+"; filename*=UTF-8''"+url.PathEscape(filename))
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()
	_ = writer.Write([]string{
		"任务号", "业务键", "状态", "结论", "分类编码", "分类名称", "分类置信度", "风险等级", "风险分",
		"标签", "摘要", "原因", "触发来源", "模型", "供应商ID", "耗时(ms)", "创建时间", "完成时间",
		"复核人ID", "复核原因码", "复核备注",
	})
	for _, task := range tasks {
		var detail map[string]any
		_ = model.DecodeJSON(task.ResultJSON, &detail)
		var verdict map[string]any
		_ = model.DecodeJSON(task.ModelResult, &verdict)
		risk := firstNonEmpty(asString(detail["risk_level"]), asString(verdict["risk_level"]))
		score := firstNonEmpty(asString(detail["score"]), asString(verdict["score"]))
		category := firstNonEmpty(asString(detail["category"]), asString(verdict["category"]))
		categoryName := firstNonEmpty(asString(detail["category_name"]), asString(verdict["category_name"]))
		categoryConfidence := firstNonEmpty(asString(detail["category_confidence"]), asString(verdict["category_confidence"]))
		labels := strings.Join(stringSlice(detail["labels"]), " | ")
		if labels == "" {
			labels = strings.Join(stringSlice(verdict["labels"]), " | ")
		}
		reasons := strings.Join(stringSlice(detail["reasons"]), " | ")
		summary := firstNonEmpty(asString(detail["summary"]), asString(verdict["summary"]))
		created := task.CreatedAt.Format("2006-01-02 15:04:05")
		finished := ""
		if task.FinishedAt != nil {
			finished = task.FinishedAt.Format("2006-01-02 15:04:05")
		}
		reviewer := ""
		if task.ReviewerID != nil {
			reviewer = fmt.Sprint(*task.ReviewerID)
		}
		row := []string{
			task.TaskNo, task.BizKey, task.Status, task.Result, category, categoryName, categoryConfidence,
			risk, score, labels, summary, reasons, task.SourceType,
			firstNonEmpty(asString(verdict["model"]), ""), fmt.Sprint(task.ProviderID),
			fmt.Sprint(task.DurationMS), created, finished, reviewer, task.ReviewReasonCode, task.ReviewNote,
		}
		for i := range row {
			row[i] = safeCSV(row[i])
		}
		_ = writer.Write(row)
	}
	writer.Flush()
}

func safeCSV(value string) string {
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@', '	', '':
		return "'" + value
	default:
		return value
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func asString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return fmt.Sprintf("%g", x)
	case float32:
		return fmt.Sprintf("%g", x)
	case int:
		return fmt.Sprint(x)
	case int64:
		return fmt.Sprint(x)
	case bool:
		return fmt.Sprint(x)
	default:
		return fmt.Sprint(x)
	}
}
