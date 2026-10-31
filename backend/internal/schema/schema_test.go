package schema

import (
	"strings"
	"testing"
)

func TestNormalizeSQLType(t *testing.T) {
	cases := map[string]string{
		"datetime(3) NULL":                 "datetime(3)",
		"bigint unsigned AUTO_INCREMENT":   "bigintunsigned",
		"bigint(20) unsigned":              "bigintunsigned",
		"int(11) NOT NULL":                 "int",
		"boolean":                          "tinyint",
		"tinyint(1)":                       "tinyint",
		"varchar(128) DEFAULT '' NOT NULL": "varchar(128)",
		"longtext":                         "longtext",
	}
	for input, want := range cases {
		if got := normalizeSQLType(input); got != want {
			t.Fatalf("normalizeSQLType(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestTableNames(t *testing.T) {
	app := strings.Join(AppTableNames(), ",")
	for _, table := range []string{"sys_users", "sys_roles", "projects", "audits", "tasks"} {
		if !strings.Contains(app, table) {
			t.Fatalf("app tables should include %s: %s", table, app)
		}
	}
	all := strings.Join(AllTableNames(), ",")
	if !strings.Contains(all, "schema_versions") {
		t.Fatalf("all tables should include schema_versions: %s", all)
	}
}

func TestCoreSchemaMissing(t *testing.T) {
	tables := map[string]bool{"sys_users": true}
	if coreSchemaMissing(tables, nil) {
		t.Fatal("complete users table should not be reported as missing core")
	}
	if !coreSchemaMissing(map[string]bool{}, nil) {
		t.Fatal("missing users table must be reported")
	}
	if !coreSchemaMissing(tables, []ColumnChange{{Table: "sys_users", Column: "password_hash"}}) {
		t.Fatal("missing password_hash must be reported")
	}
	if coreSchemaMissing(tables, []ColumnChange{{Table: "projects", Column: "new_column"}}) {
		t.Fatal("unrelated column changes must not mark core as missing")
	}
}

func TestFreshReport(t *testing.T) {
	report := FreshReport()
	if !report.Required || report.Existing || !report.MissingCore {
		t.Fatalf("unexpected fresh report: %+v", report)
	}
	if len(report.MissingTables) == 0 {
		t.Fatal("fresh report should list missing application tables")
	}
}
