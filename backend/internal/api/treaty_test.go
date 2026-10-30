package api

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"smartdoc-vision/backend/internal/model"
)

func TestExtractDocxText(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	document, err := writer.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = document.Write([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p><w:r><w:t>第一条</w:t></w:r><w:r><w:t>：超过 90 天转人工。</w:t></w:r></w:p>
<w:p><w:r><w:t>第二条：缺少盖章直接驳回。</w:t></w:r></w:p>
</w:body>
</w:document>`))
	header, err := writer.Create("word/header1.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = header.Write([]byte(`<w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:r><w:t>页眉条约</w:t></w:r></w:p></w:hdr>`))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	got, mime, err := extractTreatyText("treaty.docx", buffer.Bytes())
	if err != nil {
		t.Fatalf("extract docx: %v", err)
	}
	if !strings.Contains(got, "第一条") || !strings.Contains(got, "缺少盖章直接驳回") || !strings.Contains(got, "页眉条约") {
		t.Fatalf("docx text missing: %q", got)
	}
	if mime == "" {
		t.Fatal("expected a docx mime type")
	}
}

func TestLegacyDocIsRejected(t *testing.T) {
	if _, _, err := extractTreatyText("legacy.doc", []byte{0xD0, 0xCF, 0x11, 0xE0, 0x00, 0x00}); err == nil {
		t.Fatal("legacy .doc should be rejected with a clear error")
	}
}

func TestNormalizeAuditTreaties(t *testing.T) {
	docs := []model.AuditTreaty{
		{Name: "条约 A", SourceType: "text", Status: "ready", Content: "缺少盖章必须 REVIEW。"},
		{Name: "条约 B", SourceType: "file", Status: "failed", Error: "解析失败"},
	}
	got, err := normalizeAuditTreaties(docs)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID == "" || got[1].ID == "" {
		t.Fatalf("unexpected normalized treaties: %+v", got)
	}
	if got[0].ContentChars != len([]rune("缺少盖章必须 REVIEW。")) || got[0].Status != model.TreatyStatusReady {
		t.Fatalf("unexpected ready treaty: %+v", got[0])
	}
	if got[1].Status != model.TreatyStatusFailed {
		t.Fatalf("failed status was not preserved: %+v", got[1])
	}
}

func TestNormalizeAuditTreatiesRejectsUnparsedReadyDocument(t *testing.T) {
	_, err := normalizeAuditTreaties([]model.AuditTreaty{{
		Name: "未完成", Status: model.TreatyStatusReady, Content: "",
	}})
	if err == nil {
		t.Fatal("expected an error for READY without content")
	}
}
