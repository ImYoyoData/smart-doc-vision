package api

import (
	"strings"
	"testing"
)

func TestScriptAgentReadOnlyDetection(t *testing.T) {
	cases := []struct {
		content string
		want    bool
	}{
		{"把结果写回数据库", false},
		{"不要修改脚本，只解释一下", true},
		{"请联网搜索 Redis Stream 用法", false},
		{"只查询 XADD 用法，不要改代码", true},
		{"这个脚本是做什么的？", true},
		{"帮我看看当前脚本有没有问题", true},
		{"解释一下这段脚本，然后帮我修复语法错误", false},
		{"检查当前脚本并修复潜在问题", false},
	}
	for _, tc := range cases {
		got := scriptAgentReadOnly([]assistantMessage{{Role: "user", Content: tc.content}})
		if got != tc.want {
			t.Fatalf("readOnly(%q) = %v, want %v", tc.content, got, tc.want)
		}
	}
}

func TestScriptAgentReadOnlyToolsExcludeMutations(t *testing.T) {
	for _, tool := range scriptAgentReadOnlyTools() {
		if isScriptMutationTool(tool.Name) {
			t.Fatalf("read-only tool list contains mutation tool %q", tool.Name)
		}
	}
}

func TestScriptASIWarning(t *testing.T) {
	if warning := scriptASIWarning("ctx.log('after')\n;({ ok: true })"); warning != "" {
		t.Fatalf("leading semicolon should pass: %s", warning)
	}
	warning := scriptASIWarning("ctx.log('after')\n({ ok: true })")
	if !strings.Contains(warning, "第 2 行") {
		t.Fatalf("expected line warning, got %q", warning)
	}
}
