package api

import (
	"fmt"
	"strings"

	"github.com/Calcium-Ion/moejs"
)

const maxScriptBytes = 64 << 10

func validateJSActionInput(label string, action routeActionInput) error {
	script := strings.TrimSpace(action.Script)
	if script == "" {
		return fmt.Errorf("%s缺少 script", label)
	}
	if len(script) > maxScriptBytes {
		return fmt.Errorf("%s脚本超过 64KB 限制", label)
	}
	if len([]rune(action.Remark)) > 128 {
		return fmt.Errorf("%s备注不能超过 128 个字符", label)
	}
	if action.MemoryMB < 0 || action.MemoryMB > 512 {
		return fmt.Errorf("%s的 memory_mb 需在 0~512 之间", label)
	}
	if _, err := moejs.CompileScript("script.js", script); err != nil {
		return fmt.Errorf("%s脚本语法错误: %v", label, err)
	}
	return nil
}
