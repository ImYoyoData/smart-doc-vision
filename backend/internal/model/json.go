package model

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	gschema "gorm.io/gorm/schema"
)

// JSONText stores JSON as longtext on MySQL (5.7 compatibility) and text on
// PostgreSQL while still marshalling as raw JSON in API responses.
type JSONText []byte

// GormDBDataType picks the largest text column available on each dialect while
// still honouring an explicit `gorm:"type:..."` tag.
func (JSONText) GormDBDataType(db *gorm.DB, field *gschema.Field) string {
	return textColumnType(db, field)
}

// LongText is a string stored as longtext on MySQL and text on PostgreSQL.
type LongText string

func (LongText) GormDBDataType(db *gorm.DB, field *gschema.Field) string {
	return textColumnType(db, field)
}

func (t LongText) Value() (driver.Value, error) { return string(t), nil }

func (t *LongText) Scan(value any) error {
	switch v := value.(type) {
	case nil:
		*t = ""
	case string:
		*t = LongText(v)
	case []byte:
		*t = LongText(v)
	default:
		return fmt.Errorf("unsupported LongText type %T", value)
	}
	return nil
}

func textColumnType(db *gorm.DB, field *gschema.Field) string {
	if field != nil {
		if tag, ok := field.TagSettings["TYPE"]; ok && strings.TrimSpace(tag) != "" {
			return tag
		}
	}
	if isPostgres(db) {
		return "text"
	}
	return "longtext"
}

func isPostgres(db *gorm.DB) bool {
	return db != nil && db.Dialector != nil && db.Dialector.Name() == "postgres"
}

func (j JSONText) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return string(j), nil
}

func (j *JSONText) Scan(value any) error {
	if value == nil {
		*j = nil
		return nil
	}
	switch v := value.(type) {
	case []byte:
		*j = append((*j)[0:0], v...)
	case string:
		*j = append((*j)[0:0], v...)
	default:
		return fmt.Errorf("unsupported JSONText type %T", value)
	}
	return nil
}

func (j JSONText) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	if !isJSON(j) {
		return nil, errors.New("invalid stored json")
	}
	return j, nil
}

func (j *JSONText) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*j = nil
		return nil
	}
	if !isJSON(data) {
		return errors.New("invalid json")
	}
	*j = append((*j)[0:0], data...)
	return nil
}

func (j JSONText) String() string { return string(j) }

func JSONBytes(v any) JSONText {
	b, err := marshalJSON(v)
	if err != nil {
		return JSONText("null")
	}
	return JSONText(b)
}

func isJSON(data []byte) bool {
	var v any
	return unmarshalJSON(data, &v) == nil
}
