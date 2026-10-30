package catalog

type RoleDefinition struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	Menus       []string `json:"menus"`
	Builtin     bool     `json:"builtin"`
}

var allMenus = []string{
	"dashboard", "analytics", "reviews", "tasks", "audits", "providers",
	"credentials", "connections", "notifications", "projects", "logs", "recycle", "users", "monitor", "help", "profile",
}

var allPermissions = []string{
	"project:create", "project:update", "project:delete", "project:restore",
	"provider:write", "audit:write", "credential:write", "notification:write",
	"task:submit", "task:review", "task:retry", "user:manage",
	"log:read", "recycle:write", "system:monitor",
}

var roleDefinitions = []RoleDefinition{
	{
		Code: "SUPER_ADMIN", Name: "超级管理员", Builtin: true,
		Description: "系统全局管理，包含账号、项目、全部配置与回收站。",
		Permissions: allPermissions, Menus: allMenus,
	},
	{
		Code: "PROJECT_ADMIN", Name: "项目管理员", Builtin: true,
		Description: "管理被授权项目内的模型、审核、凭证、任务和回收站。",
		Permissions: []string{
			"project:update", "provider:write", "audit:write", "credential:write", "notification:write",
			"task:submit", "task:review", "task:retry", "recycle:write", "log:read",
		},
		Menus: []string{"dashboard", "analytics", "reviews", "tasks", "audits", "providers", "credentials", "connections", "notifications", "projects", "logs", "recycle", "help", "profile"},
	},
	{
		Code: "AUDITOR", Name: "审核配置员", Builtin: true,
		Description: "配置模型与审核规则、提交任务、查看日志；不管理凭证和账号。",
		Permissions: []string{"provider:write", "audit:write", "task:submit"},
		Menus:       []string{"dashboard", "analytics", "tasks", "audits", "providers", "logs", "help", "profile"},
	},
	{
		Code: "REVIEWER", Name: "复核员", Builtin: true,
		Description: "处理待议任务、查看任务详情与报表，不修改审核配置。",
		Permissions: []string{"task:review"},
		Menus:       []string{"dashboard", "analytics", "reviews", "tasks", "help", "profile"},
	},
	{
		Code: "VIEWER", Name: "只读用户", Builtin: true,
		Description: "查看被授权项目的任务、配置、报表和日志，所有写操作被禁止。",
		Permissions: []string{"log:read"},
		Menus:       []string{"dashboard", "analytics", "tasks", "audits", "logs", "help", "profile"},
	},
}

func AllMenus() []string {
	return append([]string(nil), allMenus...)
}

func AllPermissions() []string {
	return append([]string(nil), allPermissions...)
}

func Roles() []RoleDefinition {
	out := make([]RoleDefinition, len(roleDefinitions))
	for i, role := range roleDefinitions {
		out[i] = cloneRole(role)
	}
	return out
}

func RoleDefinitionByCode(code string) (RoleDefinition, bool) {
	for _, role := range roleDefinitions {
		if role.Code == code {
			return cloneRole(role), true
		}
	}
	return RoleDefinition{}, false
}

func RolePermissions(code string) []string {
	role, ok := RoleDefinitionByCode(code)
	if !ok {
		return []string{}
	}
	return role.Permissions
}

func RoleMenus(code string) []string {
	role, ok := RoleDefinitionByCode(code)
	if !ok {
		return []string{}
	}
	return role.Menus
}

func ValidRole(code string) bool {
	_, ok := RoleDefinitionByCode(code)
	return ok
}

func HasPermission(code, permission string) bool {
	for _, p := range RolePermissions(code) {
		if p == permission {
			return true
		}
	}
	return false
}

func HasMenu(code, menu string) bool {
	for _, m := range RoleMenus(code) {
		if m == menu {
			return true
		}
	}
	return false
}

func cloneRole(role RoleDefinition) RoleDefinition {
	role.Permissions = append([]string(nil), role.Permissions...)
	role.Menus = append([]string(nil), role.Menus...)
	return role
}
