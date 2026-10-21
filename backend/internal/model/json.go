package model

import (
	"database/sql/driver"
	"errors"
	"fmt"
)

// JSONText stores JSON as longtext for MySQL 5.7 compatibility while still
// marshalling as raw JSON in API responses.
type JSONText []byte

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
