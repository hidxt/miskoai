package weixin

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"
)

// sendAcknowledgement examines only the bounded response, never exposes the
// server ID, and never performs another request. Known duplicates are uncertain,
// including encoding/json's Unicode case-fold aliases.
func sendAcknowledgement(raw []byte) error {
	if len(raw) > maxResponseBytes || !utf8.Valid(raw) || !json.Valid(raw) {
		return ErrOutcomeUnknown
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return ErrOutcomeUnknown
	}
	var retSeen, codeSeen, idSeen, uncertain, expired, rejected, retZero, validID bool
	for pos := ackWhitespace(raw, 1); raw[pos] != '}'; {
		end := ackStringEnd(raw, pos)
		var name string
		// Valid JSON already guarantees a string key and colon.
		_ = json.Unmarshal(raw[pos:end], &name)
		pos = ackWhitespace(raw, end)
		pos = ackWhitespace(raw, pos+1)
		end = ackValueEnd(raw, pos)
		value := bytes.TrimSpace(raw[pos:end])
		switch {
		case strings.EqualFold(name, "ret"), strings.EqualFold(name, "errcode"):
			seen := &codeSeen
			isRet := strings.EqualFold(name, "ret")
			if isRet {
				seen = &retSeen
			}
			uncertain = uncertain || *seen
			*seen = true
			code, err := strconv.ParseInt(string(value), 10, 64)
			if err != nil {
				uncertain = true
			} else {
				expired = expired || code == -14
				rejected = rejected || code != 0
				retZero = retZero || (isRet && code == 0)
			}
		case strings.EqualFold(name, "message_id"):
			uncertain = uncertain || idSeen
			idSeen = true
			validID = ackPositiveID(value)
		}
		pos = ackWhitespace(raw, end)
		if raw[pos] == ',' {
			pos = ackWhitespace(raw, pos+1)
		}
	}
	if expired {
		return ErrAuthExpired
	}
	if uncertain {
		return ErrOutcomeUnknown
	}
	if rejected {
		return ErrService
	}
	if retZero || (!retSeen && idSeen && validID) {
		return nil
	}
	return ErrOutcomeUnknown
}

func ackPositiveID(raw []byte) bool {
	var digits string
	if len(raw) > 0 && raw[0] == '"' {
		// Each decimal digit can occupy at most six JSON escape bytes. Bound
		// decoding before allocating even when an untrusted string is huge.
		if len(raw) > 20*6+2 || json.Unmarshal(raw, &digits) != nil {
			return false
		}
	} else {
		if len(raw) > 20 {
			return false
		}
		digits = string(raw)
	}
	if len(digits) == 0 || len(digits) > 20 {
		return false
	}
	for i := range digits {
		if digits[i] < '0' || digits[i] > '9' {
			return false
		}
	}
	id, err := strconv.ParseUint(digits, 10, 64)
	return err == nil && id != 0
}

// The following scanners operate only after json.Valid. Values remain slices
// into the capped body: nested unknown objects/arrays are skipped without maps,
// typed arrays, recursion, or per-element allocations.
func ackWhitespace(raw []byte, pos int) int {
	for raw[pos] == ' ' || raw[pos] == '\t' || raw[pos] == '\n' || raw[pos] == '\r' {
		pos++
	}
	return pos
}

func ackStringEnd(raw []byte, pos int) int {
	for pos++; ; pos++ {
		switch raw[pos] {
		case '\\':
			pos++
		case '"':
			return pos + 1
		}
	}
}

func ackValueEnd(raw []byte, pos int) int {
	depth := 0
	for ; ; pos++ {
		switch raw[pos] {
		case '"':
			pos = ackStringEnd(raw, pos) - 1
		case '{', '[':
			depth++
		case '}', ']':
			if depth == 0 {
				return pos
			}
			depth--
		case ',':
			if depth == 0 {
				return pos
			}
		}
	}
}
