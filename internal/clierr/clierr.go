package clierr

import (
	"errors"
	"strings"
)

type Code int

const (
	CodeGeneric    Code = 1
	CodeUsage      Code = 2
	CodeAuthConfig Code = 3
	CodeNotFound   Code = 4
	CodeUpstream   Code = 5
)

type Error struct {
	code Code
	msg  string
	err  error
}

func (e *Error) Error() string {
	switch {
	case e.msg != "" && e.err != nil:
		return e.msg + ": " + e.err.Error()
	case e.msg != "":
		return e.msg
	case e.err != nil:
		return e.err.Error()
	default:
		return "cli error"
	}
}

func (e *Error) Unwrap() error {
	return e.err
}

func (e *Error) ExitCode() int {
	return int(e.code)
}

func New(code Code, msg string) error {
	return &Error{code: code, msg: msg}
}

func Wrap(code Code, err error, msg string) error {
	if err == nil {
		return nil
	}
	return &Error{code: code, msg: msg, err: err}
}

func CodeOf(err error) int {
	if err == nil {
		return 0
	}
	if HasExitCode(err) {
		var exitCoder interface{ ExitCode() int }
		_ = errors.As(err, &exitCoder)
		return exitCoder.ExitCode()
	}
	return int(CodeGeneric)
}

func HasExitCode(err error) bool {
	if err == nil {
		return false
	}
	var exitCoder interface{ ExitCode() int }
	return errors.As(err, &exitCoder)
}

func LooksLikeUsage(err error, extraSubstrings ...string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, fragment := range []string{
		"unknown command",
		"unknown flag",
		"accepts ",
		"requires at least",
		"requires exactly",
		"required flag",
	} {
		if strings.Contains(msg, fragment) {
			return true
		}
	}
	for _, fragment := range extraSubstrings {
		if strings.Contains(msg, fragment) {
			return true
		}
	}
	return false
}
