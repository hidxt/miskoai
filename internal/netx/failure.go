package netx

import (
	"context"
	"errors"
)

// categorizedFailure never renders its cause. Unwrap preserves error identity
// for cancellation, HTTP status and existing retry admission.
type categorizedFailure struct {
	code  string
	cause error
}

func (e *categorizedFailure) Error() string { return "remote failure: " + e.code }
func (e *categorizedFailure) Unwrap() error { return e.cause }

func NormalizeFailureCode(code string) string {
	switch code {
	case "", "request_invalid", "authentication", "balance", "rate_limit", "unavailable", "timeout", "canceled", "transport", "payload_limit", "response_invalid", "response_empty", "response_finish", "unknown":
		return code
	default:
		return "unknown"
	}
}

// WithFailure is for an explicit originating branch, never parsed error text.
// No error stays nil; an empty/invalid category on an actual error is unknown.
func WithFailure(code string, cause error) error {
	if cause == nil {
		return nil
	}
	code = NormalizeFailureCode(code)
	if code == "" {
		code = "unknown"
	}
	return &categorizedFailure{code, cause}
}

// Only received HTTP response statuses may reach retry admission. An injected
// transport/body error must not smuggle an HTTPError through Unwrap. Retain
// known context identity while discarding all other private transport details.
func transportFailure(err error) error {
	cause := errors.New("remote transport failure")
	if errors.Is(err, context.Canceled) {
		cause = context.Canceled
	} else if errors.Is(err, context.DeadlineExceeded) {
		cause = context.DeadlineExceeded
	}
	return WithFailure("transport", cause)
}

// FailureCode accepts only known concrete types and finite categories. It does
// not call Error on unknown/injected errors or copy a remote response body.
func FailureCode(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrLimit):
		return "payload_limit"
	}
	var status *HTTPError
	if errors.As(err, &status) && status != nil {
		switch status.Status {
		case 400, 422:
			return "request_invalid"
		case 401, 403:
			return "authentication"
		case 402:
			return "balance"
		case 429:
			return "rate_limit"
		case 500, 502, 503, 504:
			return "unavailable"
		default:
			return "unknown"
		}
	}
	var failure *categorizedFailure
	if errors.As(err, &failure) && failure != nil {
		return NormalizeFailureCode(failure.code)
	}
	return "unknown"
}
