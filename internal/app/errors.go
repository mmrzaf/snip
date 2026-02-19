package app

import "fmt"

// Exit codes per ARCHITECTURE.md.
const (
	ExitOK      = 0
	ExitUsage   = 2
	ExitIO      = 3
	ExitPartial = 4
)

// Error wraps an error with an exit code.
type Error struct {
	code int
	err  error
}

// Error returns a printable message.
func (e *Error) Error() string { return e.err.Error() }

// Unwrap exposes the wrapped error.
func (e *Error) Unwrap() error { return e.err }

// ExitCode returns the process exit code.
func (e *Error) ExitCode() int { return e.code }

// Wrap wraps err with the given exit code.
func Wrap(code int, err error) error {
	if err == nil {
		return nil
	}
	return &Error{code: code, err: err}
}

// Wrapf wraps err with formatted context.
func Wrapf(code int, err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return &Error{code: code, err: fmt.Errorf(format+": %w", append(args, err)...)}
}
