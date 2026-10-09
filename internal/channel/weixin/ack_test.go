package weixin

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSendTextAcknowledgesAbsentRetPositiveUint64(t *testing.T) {
	calls := 0
	c := fixture(t, func(r *http.Request) (*http.Response, error) {
		calls++
		return response(`{"message_id":18446744073709551615}`), nil
	})
	if err := c.SendText(context.Background(), "peer", "ctx", "stable", "synthetic"); err != nil {
		t.Fatalf("positive uint64 acknowledgement rejected: %v", err)
	}
	if calls != 1 {
		t.Fatalf("send requests = %d, want 1", calls)
	}
}

func TestSendTextAcknowledgementMatrix(t *testing.T) {
	cases := []struct {
		name, body string
		want       error
	}{
		{"numeric_one", `{"message_id":1}`, nil},
		{"string_one", `{"message_id":"1"}`, nil},
		{"string_max", `{"message_id":"18446744073709551615"}`, nil},
		{"escaped_digit", `{"message_id":"\u0031"}`, nil},
		{"decimal_string_leading_zero", `{"message_id":"0001"}`, nil},
		{"id_with_zero_code", `{"errcode":0,"message_id":1}`, nil},
		{"explicit_ret", `{"ret":0}`, nil},
		{"explicit_ret_optional_invalid_id", `{"ret":0,"message_id":null}`, nil},
		{"explicit_both", `{"ret":0,"errcode":0}`, nil},
		{"unknown_nested", `{"unused":{"ret":-14,"items":[{},[],"},\\\"",true,null]},"message_id":1}`, nil},
		{"case_keys", `{"MESSAGE_ID":"1","ERRCODE":0}`, nil},
		{"unicode_key", `{"meſſage_id":1}`, nil},
		{"empty", ``, ErrOutcomeUnknown},
		{"empty_object", `{}`, ErrOutcomeUnknown},
		{"zero_code_no_id", `{"errcode":0}`, ErrOutcomeUnknown},
		{"unknown_id_spelling", `{"messageid":1}`, ErrOutcomeUnknown},
		{"null_body", `null`, ErrOutcomeUnknown},
		{"array", `[]`, ErrOutcomeUnknown},
		{"string_body", `"fixture"`, ErrOutcomeUnknown},
		{"primitive", `0`, ErrOutcomeUnknown},
		{"truncated", `{"message_id":1`, ErrOutcomeUnknown},
		{"trailing", `{"message_id":1} {}`, ErrOutcomeUnknown},
		{"trailing_comma", `{"message_id":1,}`, ErrOutcomeUnknown},
		{"invalid_utf8", "{\"message_id\":1,\"extra\":\"\xff\"}", ErrOutcomeUnknown},
		{"null_ret", `{"ret":null,"message_id":1}`, ErrOutcomeUnknown},
		{"string_ret", `{"ret":"0","message_id":1}`, ErrOutcomeUnknown},
		{"float_ret", `{"ret":0.0,"message_id":1}`, ErrOutcomeUnknown},
		{"exponent_ret", `{"ret":0e0,"message_id":1}`, ErrOutcomeUnknown},
		{"bool_ret", `{"ret":false,"message_id":1}`, ErrOutcomeUnknown},
		{"object_ret", `{"ret":{},"message_id":1}`, ErrOutcomeUnknown},
		{"array_ret", `{"ret":[],"message_id":1}`, ErrOutcomeUnknown},
		{"overflow_ret", `{"ret":9223372036854775808,"message_id":1}`, ErrOutcomeUnknown},
		{"null_code", `{"ret":0,"errcode":null}`, ErrOutcomeUnknown},
		{"string_code", `{"message_id":1,"errcode":"0"}`, ErrOutcomeUnknown},
		{"float_code", `{"message_id":1,"errcode":0.0}`, ErrOutcomeUnknown},
		{"array_code", `{"message_id":1,"errcode":[]}`, ErrOutcomeUnknown},
		{"service_ret", `{"ret":7,"message_id":1,"errmsg":"synthetic-private"}`, ErrService},
		{"service_code", `{"ret":0,"errcode":7}`, ErrService},
		{"expired_ret", `{"ret":-14,"errcode":7}`, ErrAuthExpired},
		{"expired_code", `{"ret":7,"errcode":-14}`, ErrAuthExpired},
		{"expired_with_bad_ret", `{"ret":null,"errcode":-14}`, ErrAuthExpired},
		{"expired_with_bad_code", `{"ret":-14,"errcode":{}}`, ErrAuthExpired},
		{"duplicate_ret", `{"ret":0,"ret":0}`, ErrOutcomeUnknown},
		{"conflict_ret", `{"ret":7,"ret":0}`, ErrOutcomeUnknown},
		{"duplicate_code", `{"ret":0,"errcode":0,"ERRCODE":0}`, ErrOutcomeUnknown},
		{"duplicate_id", `{"ret":0,"message_id":1,"message_id":1}`, ErrOutcomeUnknown},
		{"unicode_duplicate_id", `{"message_id":1,"meſſage_id":1}`, ErrOutcomeUnknown},
		{"escaped_duplicate_ret", `{"ret":0,"\u0072et":0}`, ErrOutcomeUnknown},
		{"duplicate_expiry_first", `{"ret":-14,"RET":0}`, ErrAuthExpired},
		{"duplicate_expiry_last", `{"ret":0,"RET":-14}`, ErrAuthExpired},
		{"expired_duplicate_id", `{"errcode":-14,"message_id":1,"message_id":2}`, ErrAuthExpired},
		{"malformed_expiry_document", `{"ret":-14} trailing`, ErrOutcomeUnknown},
	}
	for _, id := range []string{`0`, `-0`, `-1`, `1.0`, `1e0`, `18446744073709551616`, `null`, `true`, `{}`, `[]`, `""`, `"0"`, `"-1"`, `"+1"`, `"1.0"`, `"1e0"`, `" 1"`, `"1 "`, `"18446744073709551616"`, `"000000000000000000001"`, `"１"`, `"\u0020\u0031"`} {
		cases = append(cases, struct {
			name, body string
			want       error
		}{"invalid_id_" + strconv.Itoa(len(cases)), `{"message_id":` + id + `}`, ErrOutcomeUnknown})
	}
	cases = append(cases, struct {
		name, body string
		want       error
	}{"over_response_cap", `{"ret":0,"extra":"` + strings.Repeat("x", maxResponseBytes) + `"}`, ErrOutcomeUnknown})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := fixture(t, func(r *http.Request) (*http.Response, error) { calls++; return response(tc.body), nil })
			err := c.SendText(context.Background(), "peer", "ctx", "stable", "synthetic")
			if !errors.Is(err, tc.want) {
				t.Fatalf("classification = %v, want %v", err, tc.want)
			}
			if calls != 1 {
				t.Fatalf("send requests = %d, want 1", calls)
			}
			if err != nil && (strings.Contains(err.Error(), "synthetic-private") || strings.Contains(err.Error(), "18446744073709551615")) {
				t.Fatal("private response escaped through error")
			}
		})
	}
}

func TestSendTextExchangeOutcomesAndWire(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		transport error
		want      error
	}{
		{"unauthorized", 401, nil, ErrUnauthorized}, {"forbidden", 403, nil, ErrUnauthorized},
		{"rate_limit", 429, nil, ErrRateLimited}, {"server_500", 500, nil, ErrOutcomeUnknown},
		{"server_503", 503, nil, ErrOutcomeUnknown}, {"redirect", 302, nil, ErrHTTP},
		{"bad_request", 400, nil, ErrHTTP}, {"transport", 0, errors.New("synthetic-private"), ErrOutcomeUnknown},
		{"accepted", 200, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := fixture(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "POST" || r.URL.Path != "/ilink/bot/sendmessage" {
					t.Error("wrong send route")
				}
				if r.Header.Get("Authorization") != "Bearer synthetic-token" || r.Header.Get("AuthorizationType") != "ilink_bot_token" || r.Header.Get("X-WECHAT-UIN") == "" || r.Header.Get("iLink-App-Id") != "bot" || r.Header.Get("iLink-App-ClientVersion") != "132105" || r.Header.Get("Content-Type") != "application/json" {
					t.Error("send wire headers changed")
				}
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 15*time.Second {
					t.Error("send deadline missing or exceeds 15 seconds")
				}
				if tc.transport != nil {
					return nil, tc.transport
				}
				res := response(`{"message_id":1}`)
				res.StatusCode = tc.status
				return res, nil
			})
			err := c.SendText(context.Background(), "peer", "ctx", "stable", "synthetic")
			if !errors.Is(err, tc.want) || calls != 1 {
				t.Fatalf("classification %v, calls %d", err, calls)
			}
		})
	}
}

func TestSendTextCanceledWhileSlotOccupiedDoesNotRequest(t *testing.T) {
	calls := 0
	c := fixture(t, func(*http.Request) (*http.Response, error) { calls++; return response(`{"message_id":1}`), nil })
	c.slot <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := c.SendText(ctx, "peer", "ctx", "stable", "synthetic")
	<-c.slot
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("classification %v, calls %d", err, calls)
	}
}
