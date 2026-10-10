package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"unicode/utf8"
)

// strictRequestObject validates complete bounded input before materializing only
// known required string fields. Decoded keys detect escaped aliases.
func strictRequestObject(r io.Reader, capBytes int64, fields ...string) (map[string]string, error) {
	object, err := strictRawRequestObject(r, capBytes, fields...)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(object))
	for name, raw := range object {
		var value string
		if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &value) != nil {
			return nil, errors.New("web_json")
		}
		result[name] = value
	}
	return result, nil
}

// strictRawRequestObject shares only complete, bounded exact-object traversal.
// Each caller retains responsibility for the allowed scalar types and ranges.
func strictRawRequestObject(r io.Reader, capBytes int64, fields ...string) (map[string]json.RawMessage, error) {
	invalid := errors.New("web_json")
	if r == nil || capBytes < 1 || capBytes > 1<<20 {
		return nil, invalid
	}
	data, err := io.ReadAll(io.LimitReader(r, capBytes+1))
	if err != nil || int64(len(data)) > capBytes || !utf8.Valid(data) || !json.Valid(data) || !validSurrogates(data) {
		return nil, invalid
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return nil, invalid
	}
	allowed := make(map[string]bool, len(fields))
	for _, field := range fields {
		allowed[field] = true
	}
	result := make(map[string]json.RawMessage, len(fields))
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, invalid
		}
		name, ok := key.(string)
		if !ok || !allowed[name] {
			return nil, invalid
		}
		if _, exists := result[name]; exists {
			return nil, invalid
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, invalid
		}
		result[name] = value
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') || len(result) != len(fields) {
		return nil, invalid
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, invalid
	}
	return result, nil
}

// encoding/json replaces unpaired UTF-16 escapes. Refuse silent credential changes.
func validSurrogates(data []byte) bool {
	quoted := false
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			quoted = !quoted
			continue
		}
		if !quoted || data[i] != 0x5c {
			continue
		}
		i++
		if i >= len(data) {
			return false
		}
		if data[i] != 'u' {
			continue
		}
		if i+4 >= len(data) {
			return false
		}
		code, err := strconv.ParseUint(string(data[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if code >= 0xdc00 && code <= 0xdfff {
			return false
		}
		if code >= 0xd800 && code <= 0xdbff {
			if i+6 >= len(data) || data[i+1] != 0x5c || data[i+2] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(data[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}

func safeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, `{"error":"`+code+`"}`)
}
func tokenResponse(w http.ResponseWriter, name, token string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"`+name+`":"`+token+`"}`)
}
