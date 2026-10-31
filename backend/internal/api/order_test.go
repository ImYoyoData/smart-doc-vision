package api

import "testing"

func TestTaskOrder(t *testing.T) {
	cases := map[string]string{
		"":                               "created_at DESC",
		"created_at:asc":                 "created_at ASC",
		"status:asc,created_at:desc":     "status ASC, created_at DESC",
		"duration_ms:asc,task_no:desc":   "duration_ms ASC, task_no DESC",
		"password:asc,result:asc":        "result ASC",
		"task_no:asc; DROP TABLE tasks;": "task_no DESC",
	}
	for input, want := range cases {
		if got := taskOrder(input); got != want {
			t.Fatalf("taskOrder(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAuditLogOrder(t *testing.T) {
	cases := map[string]string{
		"":                            "created_at DESC",
		"operator:asc,created_at:asc": "operator ASC, created_at ASC",
		"action:desc,result:asc":      "action DESC, result ASC",
		"unknown:asc":                 "created_at DESC",
	}
	for input, want := range cases {
		if got := auditLogOrder(input); got != want {
			t.Fatalf("auditLogOrder(%q) = %q, want %q", input, got, want)
		}
	}
}
