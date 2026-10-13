package executor

import "fmt"

type ErrorKind string

const (
	ErrorInvalid          ErrorKind = "invalid_request"
	ErrorUnauthorized     ErrorKind = "unauthorized"
	ErrorUnavailable      ErrorKind = "unavailable"
	ErrorOverloaded       ErrorKind = "overloaded"
	ErrorDeadline         ErrorKind = "deadline"
	ErrorCanceled         ErrorKind = "canceled"
	ErrorProtocol         ErrorKind = "protocol"
	ErrorExecutionUnknown ErrorKind = "execution_unknown"
)

// CallError describes transport failure; user program verdicts are returned in RunResult.
type CallError struct {
	Kind  ErrorKind
	Cause error
}

func (e *CallError) Error() string { return fmt.Sprintf("executor %s: %v", e.Kind, e.Cause) }
func (e *CallError) Unwrap() error { return e.Cause }
func (e *CallError) Retryable() bool {
	return e.Kind != ErrorInvalid && e.Kind != ErrorUnauthorized && e.Kind != ErrorProtocol && e.Kind != ErrorCanceled
}
