package model

import (
	"bytes"
	"encoding/json"
)

func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func unmarshalJSON(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec.Decode(v)
}

// ParseJSON decodes a JSONText into an arbitrary value. It returns nil for
// empty values without treating that as an error.
func ParseJSON(j JSONText) any {
	if len(j) == 0 {
		return nil
	}
	var v any
	if err := unmarshalJSON(j, &v); err != nil {
		return nil
	}
	return v
}

func DecodeJSON(j JSONText, v any) error {
	if len(j) == 0 {
		return nil
	}
	return unmarshalJSON(j, v)
}
