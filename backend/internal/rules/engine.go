package rules

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"smartdoc-vision/backend/internal/ai"
	"smartdoc-vision/backend/internal/model"
)

type Group struct {
	Name        string `json:"name"`
	Priority    int    `json:"priority"`
	Logic       string `json:"logic"`
	StopOnHit   bool   `json:"stop_on_hit"`
	OnHitResult string `json:"on_hit_result"`
	Items       []Item `json:"items"`
}

type Item struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
}

type ItemTrace struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Expected any    `json:"expected"`
	Actual   any    `json:"actual"`
	Hit      bool   `json:"hit"`
}

type GroupTrace struct {
	Name        string      `json:"name"`
	Priority    int         `json:"priority"`
	Logic       string      `json:"logic"`
	Hit         bool        `json:"hit"`
	Skipped     bool        `json:"skipped,omitempty"`
	OnHitResult string      `json:"on_hit_result"`
	Items       []ItemTrace `json:"items"`
}

type Result struct {
	FinalResult string       `json:"final_result"`
	MatchedRule string       `json:"matched_rule,omitempty"`
	Trace       []GroupTrace `json:"trace"`
}

type Context struct {
	Verdict    *model.Verdict
	Input      model.TaskInput
	SourceType string
}

func Evaluate(audit model.Audit, ctx Context) Result {
	var groups []Group
	if err := model.DecodeJSON(audit.Rules, &groups); err != nil {
		return Result{FinalResult: "", Trace: []GroupTrace{{Name: "规则解析", Skipped: true}}}
	}
	res := Result{Trace: []GroupTrace{}}
	sortGroups(groups)
	for _, g := range groups {
		trace := GroupTrace{Name: g.Name, Priority: g.Priority, Logic: strings.ToUpper(defaultString(g.Logic, "AND")), OnHitResult: g.OnHitResult}
		if len(g.Items) == 0 {
			trace.Skipped = true
			res.Trace = append(res.Trace, trace)
			continue
		}
		logic := strings.ToUpper(defaultString(g.Logic, "AND"))
		groupHit := logic == "AND"
		for _, item := range g.Items {
			actual := lookup(ctx, item.Field)
			hit := evalItem(actual, item.Operator, item.Value)
			trace.Items = append(trace.Items, ItemTrace{
				Field: item.Field, Operator: item.Operator, Expected: item.Value,
				Actual: normalizeActual(actual), Hit: hit,
			})
			if logic == "AND" && !hit {
				groupHit = false
			}
			if logic == "OR" && hit {
				groupHit = true
			}
		}
		trace.Hit = groupHit
		res.Trace = append(res.Trace, trace)
		if groupHit {
			res.FinalResult = strings.ToUpper(g.OnHitResult)
			res.MatchedRule = g.Name
			if g.StopOnHit {
				break
			}
		}
	}
	return res
}

func sortGroups(groups []Group) {
	// Stable insertion sort keeps the JSON order for equal priorities.
	for i := 1; i < len(groups); i++ {
		for j := i; j > 0 && groups[j].Priority < groups[j-1].Priority; j-- {
			groups[j], groups[j-1] = groups[j-1], groups[j]
		}
	}
}

func lookup(ctx Context, field string) any {
	field = strings.TrimSpace(field)
	switch {
	case field == "model.result":
		return ctx.Verdict.Result
	case field == "model.score":
		return ctx.Verdict.Score
	case field == "model.risk_level":
		return ctx.Verdict.RiskLevel
	case field == "model.label":
		if len(ctx.Verdict.Labels) > 0 {
			return ctx.Verdict.Labels[0]
		}
		return ""
	case field == "model.labels":
		return ctx.Verdict.Labels
	case field == "model.summary":
		return ctx.Verdict.Summary
	case field == "model.category":
		return ctx.Verdict.Category
	case field == "model.category_name":
		return ctx.Verdict.CategoryName
	case field == "model.category_confidence":
		return ctx.Verdict.CategoryConfidence
	case field == "media.text":
		return ai.CollectText(ctx.Input)
	case field == "media.text_len":
		return len([]rune(ai.CollectText(ctx.Input)))
	case field == "media.count":
		return len(ctx.Input.Media)
	case field == "media.image_count":
		return len(ai.CollectImages(ctx.Input))
	case field == "media.type":
		return ai.MediaType(ctx.Input)
	case field == "task.source_type":
		return ctx.SourceType
	case field == "task.biz_key":
		return ctx.Input.BizKey
	case strings.HasPrefix(field, "ext."):
		return lookupMap(ctx.Input.Ext, strings.TrimPrefix(field, "ext."))
	case strings.HasPrefix(field, "input."):
		return lookupMap(map[string]any{"biz_key": ctx.Input.BizKey, "ext": ctx.Input.Ext}, strings.TrimPrefix(field, "input."))
	default:
		return nil
	}
}

func lookupMap(m map[string]any, path string) any {
	if m == nil {
		return nil
	}
	var cur any = m
	for _, part := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = obj[part]
	}
	return cur
}

func evalItem(actual any, op string, expected any) bool {
	op = strings.ToLower(strings.TrimSpace(op))
	switch op {
	case "eq":
		return equal(actual, expected)
	case "ne":
		return !equal(actual, expected)
	case "gt", "gte", "lt", "lte":
		a, ok1 := toFloat(actual)
		b, ok2 := toFloat(expected)
		if !ok1 || !ok2 {
			return false
		}
		switch op {
		case "gt":
			return a > b
		case "gte":
			return a >= b
		case "lt":
			return a < b
		default:
			return a <= b
		}
	case "between":
		a, ok := toFloat(actual)
		vals, ok2 := toSlice(expected)
		if !ok || !ok2 || len(vals) != 2 {
			return false
		}
		lo, okLo := toFloat(vals[0])
		hi, okHi := toFloat(vals[1])
		return okLo && okHi && a >= lo && a <= hi
	case "in", "not_in":
		vals, ok := toSlice(expected)
		if !ok {
			return false
		}
		hit := false
		for _, v := range vals {
			if equal(actual, v) {
				hit = true
				break
			}
		}
		if op == "not_in" {
			return !hit
		}
		return hit
	case "contains", "not_contains":
		hit := strings.Contains(strings.ToLower(toText(actual)), strings.ToLower(toText(expected)))
		if op == "not_contains" {
			return !hit
		}
		return hit
	case "contains_any", "contains_all":
		actors, ok := toTextSlice(actual)
		wants, ok2 := toTextSlice(expected)
		if !ok || !ok2 {
			return false
		}
		if op == "contains_any" {
			for _, a := range actors {
				for _, w := range wants {
					if strings.EqualFold(a, w) {
						return true
					}
				}
			}
			return false
		}
		for _, w := range wants {
			found := false
			for _, a := range actors {
				if strings.EqualFold(a, w) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	case "starts_with":
		return strings.HasPrefix(toText(actual), toText(expected))
	case "ends_with":
		return strings.HasSuffix(toText(actual), toText(expected))
	case "regex":
		pattern, ok := expected.(string)
		if !ok {
			return false
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		return re.MatchString(toText(actual))
	case "is_null":
		return actual == nil
	case "is_not_null":
		return actual != nil
	case "is_empty":
		return toText(actual) == "" || len(toSliceNoBool(actual)) == 0
	case "is_not_empty":
		return toText(actual) != ""
	default:
		return false
	}
}

func equal(a, b any) bool {
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if aok && bok {
		return af == bf
	}
	return strings.EqualFold(strings.TrimSpace(toText(a)), strings.TrimSpace(toText(b)))
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func toSlice(v any) ([]any, bool) {
	switch x := v.(type) {
	case []any:
		return x, true
	case []string:
		out := make([]any, len(x))
		for i := range x {
			out[i] = x[i]
		}
		return out, true
	default:
		return nil, false
	}
}

func toSliceNoBool(v any) []any {
	s, _ := toSlice(v)
	return s
}

func toTextSlice(v any) ([]string, bool) {
	s, ok := toSlice(v)
	if !ok {
		if str, ok := v.(string); ok {
			return []string{str}, true
		}
		return nil, false
	}
	out := make([]string, 0, len(s))
	for _, item := range s {
		out = append(out, toText(item))
	}
	return out, true
}

func toText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprint(x)
		}
		return string(b)
	}
}

func normalizeActual(v any) any {
	if len(toSliceNoBool(v)) > 0 {
		return v
	}
	if s, ok := v.(string); ok && len([]rune(s)) > 240 {
		return string([]rune(s)[:240]) + "…"
	}
	return v
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
