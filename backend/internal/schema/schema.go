package schema

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	gschema "gorm.io/gorm/schema"

	"smartdoc-vision/backend/internal/model"
	appversion "smartdoc-vision/backend/internal/version"
)

// Version must be bumped whenever model structs change in a way that requires
// an explicit database structure upgrade. Existing installations compare this
// value and the live schema when the process starts.
const Version = "20261101.1"

type ColumnChange struct {
	Table    string `json:"table"`
	Column   string `json:"column"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type IndexChange struct {
	Table   string `json:"table"`
	Name    string `json:"name"`
	Columns string `json:"columns,omitempty"`
	Unique  bool   `json:"unique,omitempty"`
}

// Report is the non-destructive schema comparison shown to administrators
// before any migration runs.
type Report struct {
	CheckedAt       time.Time      `json:"checked_at"`
	Existing        bool           `json:"existing"`
	Required        bool           `json:"required"`
	MissingCore     bool           `json:"missing_core"`
	HasSuperAdmin   bool           `json:"has_super_admin"`
	SchemaVersion   string         `json:"schema_version"`
	DatabaseVersion string         `json:"database_version"`
	HasVersionTable bool           `json:"has_version_table"`
	ExistingTables  []string       `json:"existing_tables"`
	MissingTables   []string       `json:"missing_tables"`
	AddedColumns    []ColumnChange `json:"added_columns"`
	ModifiedColumns []ColumnChange `json:"modified_columns"`
	AddedIndexes    []IndexChange  `json:"added_indexes"`
	ExtraTables     []string       `json:"extra_tables"`
	ExtraColumns    []ColumnChange `json:"extra_columns"`
	Warnings        []string       `json:"warnings"`
}

func AppModels() []any {
	return []any{
		&model.User{},
		&model.ProjectMember{},
		&model.UserAuditScope{},
		&model.Project{},
		&model.APICredential{},
		&model.AIProvider{},
		&model.Audit{},
		&model.TriggerSource{},
		&model.Task{},
		&model.TaskEvent{},
		&model.AuditLog{},
		&model.Role{},
		&model.ProjectSecret{},
		&model.ScriptExecution{},
		&model.RedisConnection{},
		&model.ProviderUsageLog{},
		&model.NotificationChannel{},
		&model.InAppMessage{},
	}
}

func AllModels() []any {
	models := AppModels()
	return append(models, &model.SchemaVersion{})
}

// FreshReport describes a database that does not exist yet.
func FreshReport() Report {
	report := newReport()
	report.MissingTables = AppTableNames()
	report.Required = true
	report.MissingCore = true
	report.Warnings = append(report.Warnings, "数据库不存在，安装时会自动创建。")
	return report
}

func newReport() Report {
	return Report{
		CheckedAt:       time.Now(),
		SchemaVersion:   Version,
		ExistingTables:  []string{},
		MissingTables:   []string{},
		AddedColumns:    []ColumnChange{},
		ModifiedColumns: []ColumnChange{},
		AddedIndexes:    []IndexChange{},
		ExtraTables:     []string{},
		ExtraColumns:    []ColumnChange{},
		Warnings:        []string{},
	}
}

// Inspect compares GORM model definitions with the live MySQL database without
// changing it.
func Inspect(db *gorm.DB) (Report, error) {
	report := newReport()
	driver := db.Dialector.Name()
	tables, err := db.Migrator().GetTables()
	if err != nil {
		return report, fmt.Errorf("读取数据库表失败: %w", err)
	}
	tableSet := make(map[string]bool, len(tables))
	for _, table := range tables {
		tableSet[table] = true
	}
	report.HasVersionTable = tableSet["schema_versions"]
	report.DatabaseVersion = readVersion(db, report.HasVersionTable)

	expectedTables := map[string]bool{}
	expectedColumns := map[string]map[string]bool{}
	for _, dest := range AppModels() {
		parsed, err := parseModel(dest)
		if err != nil {
			return report, err
		}
		table := parsed.Table
		expectedTables[table] = true
		expectedColumns[table] = map[string]bool{}
		if !tableSet[table] {
			report.MissingTables = append(report.MissingTables, table)
			continue
		}
		report.ExistingTables = append(report.ExistingTables, table)
		columnTypes, err := db.Migrator().ColumnTypes(dest)
		if err != nil {
			return report, fmt.Errorf("读取 %s 表结构失败: %w", table, err)
		}
		actual := make(map[string]gorm.ColumnType, len(columnTypes))
		for _, column := range columnTypes {
			actual[column.Name()] = column
		}
		for _, field := range parsed.Fields {
			if field.DBName == "" || field.IgnoreMigration {
				continue
			}
			expectedColumns[table][field.DBName] = true
			expectedType := expectedDataType(db, field)
			columnChange := ColumnChange{
				Table: table, Column: field.DBName, Expected: expectedType,
			}
			column, exists := actual[field.DBName]
			if !exists {
				columnChange.Reason = "字段缺失"
				report.AddedColumns = append(report.AddedColumns, columnChange)
				continue
			}
			actualType := columnTypeString(column)
			if normalizeSQLTypeFor(driver, expectedType) != normalizeSQLTypeFor(driver, actualType) {
				columnChange.Actual = actualType
				columnChange.Reason = "列类型不同"
				report.ModifiedColumns = append(report.ModifiedColumns, columnChange)
				continue
			}
			if field.PrimaryKey || field.AutoIncrement {
				continue
			}
			expectedNullable := !field.NotNull
			if nullable, ok := column.Nullable(); ok && nullable != expectedNullable {
				report.ModifiedColumns = append(report.ModifiedColumns, ColumnChange{
					Table: table, Column: field.DBName,
					Expected: nullableLabel(expectedNullable),
					Actual:   nullableLabel(nullable),
					Reason:   "可空性不同",
				})
			}
		}
		for name := range actual {
			if !expectedColumns[table][name] {
				report.ExtraColumns = append(report.ExtraColumns, ColumnChange{
					Table: table, Column: name, Reason: "模型之外，保留不删除",
				})
			}
		}
		for name, index := range parsed.ParseIndexes() {
			if name == "" || index.Class == "PRIMARY" || db.Migrator().HasIndex(dest, name) {
				continue
			}
			report.AddedIndexes = append(report.AddedIndexes, IndexChange{
				Table:   table,
				Name:    name,
				Columns: indexColumns(index),
				Unique:  strings.EqualFold(index.Class, "UNIQUE"),
			})
		}
	}

	for table := range tableSet {
		if table == "schema_versions" || expectedTables[table] {
			continue
		}
		report.ExtraTables = append(report.ExtraTables, table)
	}

	report.Existing = len(report.ExistingTables) > 0
	report.MissingCore = coreSchemaMissing(tableSet, report.AddedColumns)
	report.HasSuperAdmin = hasActiveSuperAdmin(db, tableSet["sys_users"])
	report.Required = len(report.MissingTables) > 0 ||
		len(report.AddedColumns) > 0 ||
		len(report.ModifiedColumns) > 0 ||
		len(report.AddedIndexes) > 0

	if len(report.ModifiedColumns) > 0 {
		report.Warnings = append(report.Warnings, "部分列的类型或可空性将被调整，极端情况下可能影响现有数据，请先备份数据库。")
	}
	if len(report.ExtraTables) > 0 || len(report.ExtraColumns) > 0 {
		report.Warnings = append(report.Warnings, "模型之外的表或列不会被自动删除。")
	}
	if report.MissingCore {
		report.Warnings = append(report.Warnings, "核心用户表或关键字段缺失，当前无法通过网页登录；请恢复数据库，或删除 .install.txt 后重新安装。")
	} else if report.Required {
		report.Warnings = append(report.Warnings, "检测到数据库结构与当前程序不一致，仅超级管理员可登录确认升级。")
	}
	if !report.Required {
		if report.HasVersionTable && report.DatabaseVersion != "" && report.DatabaseVersion != Version {
			report.Warnings = append(report.Warnings, "数据库记录的结构版本与当前程序不同，但表结构一致，将自动更新结构版本记录。")
		} else if !report.HasVersionTable || report.DatabaseVersion == "" {
			report.Warnings = append(report.Warnings, "数据库尚未记录结构版本，将自动记录当前版本。")
		}
	}

	sort.Strings(report.ExistingTables)
	sort.Strings(report.MissingTables)
	sort.Strings(report.ExtraTables)
	sort.Slice(report.AddedColumns, func(i, j int) bool {
		return lessColumn(report.AddedColumns[i], report.AddedColumns[j])
	})
	sort.Slice(report.ModifiedColumns, func(i, j int) bool {
		return lessColumn(report.ModifiedColumns[i], report.ModifiedColumns[j])
	})
	sort.Slice(report.ExtraColumns, func(i, j int) bool {
		return lessColumn(report.ExtraColumns[i], report.ExtraColumns[j])
	})
	sort.Slice(report.AddedIndexes, func(i, j int) bool {
		if report.AddedIndexes[i].Table != report.AddedIndexes[j].Table {
			return report.AddedIndexes[i].Table < report.AddedIndexes[j].Table
		}
		return report.AddedIndexes[i].Name < report.AddedIndexes[j].Name
	})
	return report, nil
}

// Apply performs the non-destructive GORM migration and records the version.
func Apply(db *gorm.DB) error {
	migrateDB := db
	if db.Dialector.Name() == "mysql" {
		migrateDB = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci")
	}
	if err := migrateDB.AutoMigrate(AllModels()...); err != nil {
		return fmt.Errorf("应用数据库结构失败: %w", err)
	}
	return EnsureVersion(db)
}

// EnsureVersion creates the metadata table and records the current schema
// version. It is safe to call on an existing database.
func EnsureVersion(db *gorm.DB) error {
	if err := db.AutoMigrate(&model.SchemaVersion{}); err != nil {
		return fmt.Errorf("创建结构版本表失败: %w", err)
	}
	var existing model.SchemaVersion
	result := db.Where("version = ?", Version).Order("id desc").Limit(1).Find(&existing)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return db.Create(&model.SchemaVersion{
			Version: Version, AppVersion: appversion.Version, AppliedAt: time.Now(),
		}).Error
	}
	return db.Model(&model.SchemaVersion{}).Where("id = ?", existing.ID).
		Update("app_version", appversion.Version).Error
}

// Reset drops every known application table (not unrelated tables) so the
// installer can recreate a clean schema. Call with an explicit admin choice.
func Reset(db *gorm.DB) error {
	tables := AllTableNames()
	if db.Dialector.Name() == "postgres" {
		return db.Transaction(func(tx *gorm.DB) error {
			for _, table := range tables {
				if err := tx.Exec(`DROP TABLE IF EXISTS "` + table + `" CASCADE`).Error; err != nil {
					return fmt.Errorf("删除表 %s 失败: %w", table, err)
				}
			}
			return nil
		})
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET FOREIGN_KEY_CHECKS=0").Error; err != nil {
			return err
		}
		defer tx.Exec("SET FOREIGN_KEY_CHECKS=1")
		for _, table := range tables {
			if err := tx.Exec("DROP TABLE IF EXISTS `" + table + "`").Error; err != nil {
				return fmt.Errorf("删除表 %s 失败: %w", table, err)
			}
		}
		return nil
	})
}

func AppTableNames() []string {
	names := make([]string, 0, len(AppModels()))
	for _, dest := range AppModels() {
		if parsed, err := parseModel(dest); err == nil {
			names = append(names, parsed.Table)
		}
	}
	sort.Strings(names)
	return names
}

func AllTableNames() []string {
	names := make([]string, 0, len(AllModels()))
	for _, dest := range AllModels() {
		if parsed, err := parseModel(dest); err == nil {
			names = append(names, parsed.Table)
		}
	}
	sort.Strings(names)
	return names
}

func parseModel(dest any) (*gschema.Schema, error) {
	parsed, err := gschema.Parse(dest, &sync.Map{}, gschema.NamingStrategy{})
	if err != nil {
		return nil, fmt.Errorf("解析模型表结构失败: %w", err)
	}
	return parsed, nil
}

func readVersion(db *gorm.DB, exists bool) string {
	if !exists {
		return ""
	}
	var version string
	_ = db.Model(&model.SchemaVersion{}).Order("id desc").Limit(1).Pluck("version", &version).Error
	return version
}

func coreSchemaMissing(tables map[string]bool, added []ColumnChange) bool {
	if !tables["sys_users"] {
		return true
	}
	coreColumns := map[string]bool{
		"id": false, "username": false, "password_hash": false,
		"role": false, "status": false, "token_version": false,
	}
	for _, change := range added {
		if change.Table == "sys_users" {
			if _, ok := coreColumns[change.Column]; ok {
				coreColumns[change.Column] = true
			}
		}
	}
	for _, missing := range coreColumns {
		if missing {
			return true
		}
	}
	return false
}

func hasActiveSuperAdmin(db *gorm.DB, usersTable bool) bool {
	if !usersTable {
		return false
	}
	var count int64
	if err := db.Model(&model.User{}).
		Where("role = ? AND status = ?", model.RoleSuperAdmin, "ACTIVE").
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func columnTypeString(column gorm.ColumnType) string {
	if value, ok := column.ColumnType(); ok && strings.TrimSpace(value) != "" {
		return value
	}
	return column.DatabaseTypeName()
}

// expectedDataType mirrors what GORM will create for the field, including
// custom GormDBDataType implementations (for example JSONText).
func expectedDataType(db *gorm.DB, field *gschema.Field) string {
	fieldValue := reflect.New(field.IndirectFieldType)
	if dataTyper, ok := fieldValue.Interface().(interface {
		GormDBDataType(*gorm.DB, *gschema.Field) string
	}); ok {
		if dataType := dataTyper.GormDBDataType(db, field); dataType != "" {
			return dataType
		}
	}
	return db.Dialector.DataTypeOf(field)
}

func indexColumns(index gschema.Index) string {
	parts := make([]string, 0, len(index.Fields))
	for _, option := range index.Fields {
		if option.Field != nil && option.DBName != "" {
			parts = append(parts, option.DBName)
			continue
		}
		if option.Expression != "" {
			parts = append(parts, option.Expression)
		}
	}
	return strings.Join(parts, ", ")
}

func nullableLabel(nullable bool) string {
	if nullable {
		return "可空"
	}
	return "非空"
}

func lessColumn(left, right ColumnChange) bool {
	if left.Table != right.Table {
		return left.Table < right.Table
	}
	return left.Column < right.Column
}

var integerWidthPattern = regexp.MustCompile(`(?i)\b(tinyint|smallint|mediumint|int|integer|bigint)\(\d+\)`)

func normalizeSQLTypeFor(driver, value string) string {
	if driver == "postgres" {
		return normalizePostgresSQLType(value)
	}
	return normalizeSQLType(value)
}

var (
	pgTimestampTZPattern = regexp.MustCompile(`timestamp(?:\(\d+\))? with time zone`)
	pgTimestampPattern   = regexp.MustCompile(`timestamp(?:\(\d+\))? without time zone`)
	pgTimeTZPattern      = regexp.MustCompile(`\btime(?:\(\d+\))? with time zone`)
	pgTimePattern        = regexp.MustCompile(`\btime(?:\(\d+\))? without time zone`)
)

// normalizePostgresSQLType folds PostgreSQL's canonical type names and GORM's
// generated names (serial, timestamptz, ...) into one comparable form.
func normalizePostgresSQLType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, suffix := range []string{" auto_increment", " not null", " null", " default ", " comment ", " on update "} {
		if index := strings.Index(value, suffix); index >= 0 {
			value = value[:index]
		}
	}
	value = pgTimestampTZPattern.ReplaceAllString(value, "timestamptz")
	value = pgTimestampPattern.ReplaceAllString(value, "timestamp")
	value = pgTimeTZPattern.ReplaceAllString(value, "timetz")
	value = pgTimePattern.ReplaceAllString(value, "time")
	replacer := strings.NewReplacer(
		"character varying", "varchar",
		"double precision", "float8",
		"bigserial", "bigint",
		"smallserial", "smallint",
		"serial", "integer",
		"int8", "bigint",
		"int4", "integer",
		"int2", "smallint",
		"boolean", "bool",
		"decimal", "numeric",
	)
	value = replacer.Replace(value)
	value = strings.ReplaceAll(value, "integer", "int")
	value = strings.ReplaceAll(value, " ", "")
	return value
}

func normalizeSQLType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, suffix := range []string{" auto_increment", " not null", " null", " default ", " comment ", " on update "} {
		if index := strings.Index(value, suffix); index >= 0 {
			value = value[:index]
		}
	}
	value = integerWidthPattern.ReplaceAllString(value, "$1")
	value = strings.ReplaceAll(value, "integer", "int")
	value = strings.ReplaceAll(value, "boolean", "tinyint")
	value = strings.ReplaceAll(value, "bool", "tinyint")
	value = strings.ReplaceAll(value, " ", "")
	return value
}
