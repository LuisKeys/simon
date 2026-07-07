// Package simonerr defines the Simon SDK error hierarchy.
//
// Python's simon/exceptions.py uses multiple inheritance so a caller can
// catch by domain (ProviderError) or by stdlib convention (RuntimeError).
// Go has no inheritance, so the same "catch by either identity" behavior is
// reproduced with sentinel errors plus multi-unwrap (Unwrap() []error,
// Go 1.20+): errors.Is(err, ErrProvider) and errors.Is(err, ErrRuntime) both
// succeed for the same wrapped error.
package simonerr

import "errors"

// Sentinels mirroring the *SimonError* side of each Python exception.
var (
	ErrSimon      = errors.New("simon")
	ErrProvider   = errors.New("provider")
	ErrTool       = errors.New("tool")
	ErrKnowledge  = errors.New("knowledge")
	ErrPermission = errors.New("permission")
	ErrStructured = errors.New("structured output")
)

// Sentinels mirroring the stdlib-convention side of each dual-inheritance
// Python exception (RuntimeError, ValueError, PermissionError).
var (
	ErrRuntime = errors.New("runtime")
	ErrValue   = errors.New("value")
	ErrPermOS  = errors.New("permission (os)")
)

// Error is a domain error carrying both a domain sentinel (e.g. ErrProvider)
// and a stdlib-convention sentinel (e.g. ErrRuntime), replicating Python's
// ProviderError(SimonError, RuntimeError)-style dual inheritance.
type Error struct {
	domain error
	kind   error
	msg    string
	cause  error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return e.msg + ": " + e.cause.Error()
	}
	return e.msg
}

// Unwrap exposes both sentinels plus the wrapped cause, so errors.Is matches
// ErrSimon, the domain sentinel (e.g. ErrProvider), the stdlib-convention
// sentinel (e.g. ErrRuntime), and the original cause.
func (e *Error) Unwrap() []error {
	errs := []error{ErrSimon, e.domain, e.kind}
	if e.cause != nil {
		errs = append(errs, e.cause)
	}
	return errs
}

func newError(domain, kind error, msg string, cause error) *Error {
	return &Error{domain: domain, kind: kind, msg: msg, cause: cause}
}

// NewProviderError reports that a model provider failed or its package is
// not installed. Mirrors ProviderError(SimonError, RuntimeError).
func NewProviderError(msg string, cause error) error {
	return newError(ErrProvider, ErrRuntime, msg, cause)
}

// NewToolError reports that a tool call was malformed or its execution
// failed. Mirrors ToolError(SimonError, ValueError).
func NewToolError(msg string, cause error) error {
	return newError(ErrTool, ErrValue, msg, cause)
}

// NewKnowledgeError reports that knowledge base ingestion or configuration
// failed. Mirrors KnowledgeError(SimonError, RuntimeError).
func NewKnowledgeError(msg string, cause error) error {
	return newError(ErrKnowledge, ErrRuntime, msg, cause)
}

// NewPermissionDeniedError reports that a sensor tried to start without a
// granted permission scope. Mirrors PermissionDeniedError(SimonError,
// PermissionError).
func NewPermissionDeniedError(msg string) error {
	return newError(ErrPermission, ErrPermOS, msg, nil)
}

// StructuredOutputError reports that the model never produced output
// matching the requested schema. Mirrors StructuredOutputError, including
// its raw_text/attempts fields, recoverable via errors.As.
type StructuredOutputError struct {
	Msg      string
	RawText  string
	Attempts int
}

func (e *StructuredOutputError) Error() string { return e.Msg }

func (e *StructuredOutputError) Unwrap() []error { return []error{ErrSimon, ErrStructured} }

// NewStructuredOutputError constructs a StructuredOutputError.
func NewStructuredOutputError(msg, rawText string, attempts int) error {
	return &StructuredOutputError{Msg: msg, RawText: rawText, Attempts: attempts}
}
