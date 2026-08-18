package fault

import (
	"errors"
	"fmt"
)

type Error struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	Hint      string `json:"hint,omitempty"`
	Details   any    `json:"details,omitempty"`
	ExitCode  int    `json:"-"`
}

func (e *Error) Error() string { return e.Message }

func E(kind, subtype, code, message string, exitCode int) *Error {
	return &Error{Type: kind, Subtype: subtype, Code: code, Message: message, ExitCode: exitCode}
}

func NotFound(resource string) *Error {
	return E("not_found", "resource", "RESOURCE_NOT_FOUND", fmt.Sprintf("%s 不存在或无权访问", resource), 4)
}

func IsNotFound(err error) bool {
	var value *Error
	return errors.As(err, &value) && value.Type == "not_found"
}

func Invalid(code, message string) *Error {
	return E("validation", "input", code, message, 2)
}

func Conflict(code, message string) *Error {
	return E("conflict", "state", code, message, 6)
}

func Policy(code, message, hint string) *Error {
	err := E("policy", "business_rule", code, message, 4)
	err.Hint = hint
	return err
}
