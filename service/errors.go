// Package service is the canonical Go API for folio operations. CLI, MCP,
// HTTP, and ACP surfaces all wrap this layer; nothing here knows about
// terminals, prompts, or wire protocols.
package service

import "fmt"

// ErrorCode identifies a class of service-layer failure. Codes are stable
// strings that future MCP / HTTP envelopes will surface unchanged, so
// callers (including agents) can pattern-match without parsing English.
type ErrorCode string

// Stable error-code values. The set is intentionally small for v0; new
// codes get added as new failure surfaces appear.
const (
	ErrPresetNotFound ErrorCode = "preset_not_found"
	ErrPresetInvalid  ErrorCode = "preset_invalid"
	ErrInputMissing   ErrorCode = "input_missing"
	ErrInputInvalid   ErrorCode = "input_invalid"
	ErrComputeFailed  ErrorCode = "compute_failed"
	ErrRenderFailed   ErrorCode = "render_failed"
	ErrTargetExists   ErrorCode = "target_exists"
	ErrWriteFailed    ErrorCode = "write_failed"
	ErrInternal       ErrorCode = "internal"
)

// Error is the typed service error. It carries an ErrorCode plus a human
// message and (optionally) a wrapped underlying error.
type Error struct {
	Code    ErrorCode
	Message string
	Err     error
}

// Error renders the error in a "code: message" shape; the wrapped error is
// appended when present.
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap supports errors.Is / errors.As.
func (e *Error) Unwrap() error { return e.Err }

func newErr(code ErrorCode, msg string, err error) *Error {
	return &Error{Code: code, Message: msg, Err: err}
}
