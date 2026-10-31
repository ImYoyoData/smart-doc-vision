package notify

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	"time"

	"smartdoc-vision/backend/internal/model"
)

// MessageEmailData is the content used by the HTML notification email. All
// values are escaped by html/template; callers must not pre-render HTML.
type MessageEmailData struct {
	SiteName    string
	Badge       string
	Heading     string
	Intro       string
	ProjectName string
	ProjectCode string
	AuditName   string
	AuditCode   string
	TaskNo      string
	SourceLabel string
	BizKey      string
	RequestID   string
	StatusLabel string
	CreatedAt   string
	Summary     string
	ActionURL   string
	ActionLabel string
	GeneratedAt string
	HasTaskInfo bool
}

var messageEmailTemplate = template.Must(template.New("message-email").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Heading}}</title>
</head>
<body style="margin:0;padding:0;background:#f3f6fb;color:#243247;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI','PingFang SC','Microsoft YaHei',Arial,sans-serif;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#f3f6fb;padding:28px 12px;">
  <tr><td align="center">
    <table role="presentation" width="600" cellpadding="0" cellspacing="0" border="0" style="width:100%;max-width:600px;background:#ffffff;border:1px solid #e1e8f3;border-radius:16px;overflow:hidden;">
      <tr>
        <td style="padding:24px 28px;background:#0c2450;background-image:linear-gradient(135deg,#0b2450 0%,#1d4f9c 100%);color:#ffffff;">
          <div style="font-size:11px;letter-spacing:2px;color:#9fc1ff;font-weight:700;">{{.SiteName}}</div>
          <div style="margin-top:8px;font-size:22px;line-height:1.35;font-weight:700;">{{.Heading}}</div>
          <div style="margin-top:7px;font-size:12px;line-height:1.7;color:#d6e4ff;">{{.Intro}}</div>
        </td>
      </tr>
      <tr><td style="padding:22px 28px 8px;">
        <span style="display:inline-block;padding:4px 10px;border-radius:999px;background:#fff7e8;border:1px solid #fde68a;color:#92400e;font-size:11px;font-weight:600;">{{.Badge}}</span>
        {{if .HasTaskInfo}}
        <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin-top:16px;border:1px solid #e7edf6;border-radius:10px;border-collapse:separate;border-spacing:0;overflow:hidden;">
          <tr>
            <td style="padding:9px 12px;background:#f8fafd;border-bottom:1px solid #edf2f9;color:#7f90a6;font-size:11px;white-space:nowrap;width:88px;">任务编号</td>
            <td style="padding:9px 12px;border-bottom:1px solid #edf2f9;color:#243247;font-size:12px;font-family:'SFMono-Regular',Consolas,monospace;word-break:break-all;">{{.TaskNo}}</td>
          </tr>
          <tr>
            <td style="padding:9px 12px;background:#f8fafd;border-bottom:1px solid #edf2f9;color:#7f90a6;font-size:11px;white-space:nowrap;">项目</td>
            <td style="padding:9px 12px;border-bottom:1px solid #edf2f9;color:#243247;font-size:12px;">{{.ProjectName}}{{if .ProjectCode}} <span style="color:#7f90a6;font-size:11px;">({{.ProjectCode}})</span>{{end}}</td>
          </tr>
          <tr>
            <td style="padding:9px 12px;background:#f8fafd;border-bottom:1px solid #edf2f9;color:#7f90a6;font-size:11px;white-space:nowrap;">审核配置</td>
            <td style="padding:9px 12px;border-bottom:1px solid #edf2f9;color:#243247;font-size:12px;">{{.AuditName}}{{if .AuditCode}} <span style="color:#7f90a6;font-size:11px;">({{.AuditCode}})</span>{{end}}</td>
          </tr>
          <tr>
            <td style="padding:9px 12px;background:#f8fafd;border-bottom:1px solid #edf2f9;color:#7f90a6;font-size:11px;white-space:nowrap;">来源</td>
            <td style="padding:9px 12px;border-bottom:1px solid #edf2f9;color:#243247;font-size:12px;">{{.SourceLabel}}{{if .StatusLabel}} · {{.StatusLabel}}{{end}}</td>
          </tr>
          {{if .BizKey}}<tr>
            <td style="padding:9px 12px;background:#f8fafd;border-bottom:1px solid #edf2f9;color:#7f90a6;font-size:11px;white-space:nowrap;">业务键</td>
            <td style="padding:9px 12px;border-bottom:1px solid #edf2f9;color:#243247;font-size:12px;word-break:break-all;">{{.BizKey}}</td>
          </tr>{{end}}
          {{if .RequestID}}<tr>
            <td style="padding:9px 12px;background:#f8fafd;border-bottom:1px solid #edf2f9;color:#7f90a6;font-size:11px;white-space:nowrap;">request_id</td>
            <td style="padding:9px 12px;border-bottom:1px solid #edf2f9;color:#243247;font-size:12px;font-family:'SFMono-Regular',Consolas,monospace;word-break:break-all;">{{.RequestID}}</td>
          </tr>{{end}}
          <tr>
            <td style="padding:9px 12px;background:#f8fafd;color:#7f90a6;font-size:11px;white-space:nowrap;">提交时间</td>
            <td style="padding:9px 12px;color:#243247;font-size:12px;">{{.CreatedAt}}</td>
          </tr>
        </table>
        {{end}}
        {{if .Summary}}
        <div style="margin-top:18px;padding:14px 16px;background:#f7f9fc;border:1px solid #e7edf6;border-radius:10px;">
          <div style="color:#7f90a6;font-size:11px;font-weight:600;letter-spacing:.4px;">模型摘要</div>
          <div style="margin-top:6px;color:#334155;font-size:13px;line-height:1.75;white-space:pre-wrap;">{{.Summary}}</div>
        </div>
        {{end}}
        <div style="margin-top:22px;">
          <a href="{{.ActionURL}}" style="display:inline-block;padding:11px 20px;border-radius:9px;background:#2b6cf6;color:#ffffff;text-decoration:none;font-size:13px;font-weight:600;">{{.ActionLabel}}</a>
        </div>
        <div style="margin-top:12px;color:#94a3b8;font-size:11px;line-height:1.7;">如果按钮无法打开，请复制以下地址到浏览器：<br><span style="color:#5b7fb9;word-break:break-all;">{{.ActionURL}}</span></div>
      </td></tr>
      <tr><td style="padding:16px 28px 22px;border-top:1px solid #edf2f9;color:#94a3b8;font-size:10px;line-height:1.7;">
        此邮件由 {{.SiteName}} 自动发送，请勿直接回复。生成时间：{{.GeneratedAt}}
      </td></tr>
    </table>
  </td></tr>
</table>
</body>
</html>`))

func RenderMessageEmail(data MessageEmailData) (string, string, error) {
	if strings.TrimSpace(data.SiteName) == "" {
		data.SiteName = "SmartDoc Vision"
	}
	if strings.TrimSpace(data.ActionLabel) == "" {
		data.ActionLabel = "登录系统查看详情"
	}
	if strings.TrimSpace(data.Badge) == "" {
		data.Badge = "系统通知"
	}
	if strings.TrimSpace(data.Heading) == "" {
		data.Heading = "你有一条新的系统消息"
	}
	htmlBody := bytes.Buffer{}
	if err := messageEmailTemplate.Execute(&htmlBody, data); err != nil {
		return "", "", err
	}
	text := strings.Builder{}
	text.WriteString(data.SiteName + "\n\n" + data.Heading + "\n\n" + data.Intro + "\n\n")
	text.WriteString("任务编号：" + data.TaskNo + "\n")
	text.WriteString("项目：" + data.ProjectName + "\n")
	text.WriteString("审核配置：" + data.AuditName + "\n")
	text.WriteString("来源：" + data.SourceLabel)
	if data.StatusLabel != "" {
		text.WriteString(" · " + data.StatusLabel)
	}
	text.WriteString("\n")
	if data.BizKey != "" {
		text.WriteString("业务键：" + data.BizKey + "\n")
	}
	if data.RequestID != "" {
		text.WriteString("request_id：" + data.RequestID + "\n")
	}
	text.WriteString("提交时间：" + data.CreatedAt + "\n")
	if data.Summary != "" {
		text.WriteString("\n模型摘要：\n" + data.Summary + "\n")
	}
	text.WriteString("\n" + data.ActionURL + "\n")
	return text.String(), htmlBody.String(), nil
}

func buildMessageEmailData(siteName, siteURL string, message model.InAppMessage, task model.Task, audit model.Audit, project model.Project) MessageEmailData {
	actionURL := strings.TrimRight(strings.TrimSpace(siteURL), "/")
	if strings.TrimSpace(message.Link) != "" {
		actionURL += message.Link
	}
	createdAt := message.CreatedAt
	if !task.CreatedAt.IsZero() {
		createdAt = task.CreatedAt
	}
	summary := strings.TrimSpace(message.Body)
	if summary == "" {
		summary = strings.TrimSpace(message.Title)
	}
	statusLabel := ""
	if task.Status == model.TaskReviewPending {
		statusLabel = "待人工复核"
	}
	return MessageEmailData{
		SiteName:    siteName,
		Badge:       "待人工复核",
		Heading:     "有一条任务等待你复核",
		Intro:       "任务已进入待议复核队列，请核对内容后给出最终结论。",
		ProjectName: firstNonEmpty(project.Name, fmt.Sprintf("项目 #%d", message.ProjectID)),
		ProjectCode: project.Code,
		AuditName:   firstNonEmpty(audit.Name, fmt.Sprintf("审核配置 #%d", message.AuditID)),
		AuditCode:   audit.Code,
		TaskNo:      firstNonEmpty(task.TaskNo, fmt.Sprintf("任务 #%d", message.TaskID)),
		SourceLabel: sourceLabel(task.SourceType),
		BizKey:      task.BizKey,
		RequestID:   derefString(task.RequestID),
		StatusLabel: statusLabel,
		CreatedAt:   createdAt.Format("2006-01-02 15:04:05"),
		Summary:     summary,
		ActionURL:   actionURL,
		ActionLabel: "打开待议复核",
		GeneratedAt: time.Now().Format("2006-01-02 15:04:05"),
		HasTaskInfo: task.TaskNo != "" || message.TaskID != 0,
	}
}

func sourceLabel(source string) string {
	switch strings.ToUpper(strings.TrimSpace(source)) {
	case model.TriggerOpenAPI:
		return "开放 API"
	case model.TriggerRedis:
		return "Redis Stream"
	case model.TriggerWebSocket:
		return "WebSocket"
	case "ADMIN":
		return "后台提交"
	default:
		if strings.TrimSpace(source) == "" {
			return "后台提交"
		}
		return source
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

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
