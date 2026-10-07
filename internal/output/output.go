// Package output renders command results for agents (JSON) and humans (plain text).
//
// Every command result goes through a Printer so the agent-first rules hold
// everywhere: --json gives a stable schema on stdout, errors go to stderr as a
// JSON envelope, and plain output carries no color when not on a TTY.
package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// Exit codes are part of the public contract; never renumber them.
const (
	ExitOK    = 0
	ExitError = 1 // the requested state was not reached
	ExitUsage = 2 // bad command, flag, or argument
)

// Error is the only error type commands return. Code is a stable,
// machine-readable identifier (e.g. "usage"); Message is for humans.
type Error struct {
	Code    string
	Message string
	Exit    int
}

func (e *Error) Error() string { return e.Message }

// Errorf builds an *Error with the given code and exit status.
func Errorf(code string, exit int, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Exit: exit}
}

// Printer writes results in JSON or plain mode.
type Printer struct {
	JSON  bool
	Color bool
	Out   io.Writer
	Err   io.Writer
}

// New returns a Printer. Color is enabled only in plain mode, on a terminal,
// with NO_COLOR unset.
func New(jsonMode bool, out, errw io.Writer) *Printer {
	return &Printer{
		JSON:  jsonMode,
		Color: !jsonMode && isTerminal(out) && os.Getenv("NO_COLOR") == "",
		Out:   out,
		Err:   errw,
	}
}

// Result prints v as indented JSON in JSON mode, otherwise calls plain.
func (p *Printer) Result(v any, plain func(w io.Writer)) error {
	if p.JSON {
		return writeJSON(p.Out, v)
	}
	plain(p.Out)
	return nil
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Error reports err on stderr and returns the process exit code. Errors that
// are not *Error are treated as usage errors (they come from argument parsing).
func (p *Printer) Error(err error) int {
	var e *Error
	if !errors.As(err, &e) {
		e = &Error{Code: "usage", Message: err.Error(), Exit: ExitUsage}
	}
	if p.JSON {
		var env errorEnvelope
		env.Error.Code, env.Error.Message = e.Code, e.Message
		_ = writeJSON(p.Err, env)
	} else {
		fmt.Fprintf(p.Err, "jingle: %s\n", e.Message)
	}
	return e.Exit
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
