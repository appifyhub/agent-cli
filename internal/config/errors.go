package config

import (
	"errors"
	"io/fs"
	"strings"
)

var (
	// ErrHomeUnavailable means neither THE_AGENT_CLI_HOME nor the user home directory is known.
	ErrHomeUnavailable = errors.New("the user home directory is unavailable")
	// ErrNoAPIURL means no environment variable, config file or build default names a backend.
	ErrNoAPIURL = errors.New("no backend URL is configured")
	// ErrInvalidDeviceID is the reason a stored device_id is rejected.
	ErrInvalidDeviceID = errors.New("device_id is not a valid UUID")
)

// HomeError is a home directory that cannot be created or written.
type HomeError struct {
	Path string
	Err  error
}

func (e *HomeError) Error() string { return e.Path + ": " + e.Err.Error() }
func (e *HomeError) Unwrap() error { return e.Err }

// Reason is the underlying cause without the path the operating system repeats in its messages.
func (e *HomeError) Reason() string { return reason(e.Err) }

// ReadError is a config file that exists but cannot be used.
type ReadError struct {
	Path string
	Err  error
}

func (e *ReadError) Error() string  { return e.Path + ": " + e.Err.Error() }
func (e *ReadError) Unwrap() error  { return e.Err }
func (e *ReadError) Reason() string { return reason(e.Err) }

// WriteError is a config file that could not be saved; the previous file is untouched.
type WriteError struct {
	Path string
	Err  error
}

func (e *WriteError) Error() string  { return e.Path + ": " + e.Err.Error() }
func (e *WriteError) Unwrap() error  { return e.Err }
func (e *WriteError) Reason() string { return reason(e.Err) }

// URLProblem classifies why an API URL was rejected.
type URLProblem int

const (
	URLMalformed URLProblem = iota
	URLInsecure
	URLCredentials
	URLQueryOrFragment
)

// InvalidURLError is an API URL that must not be used. URL is the rejected text with any
// embedded credentials removed, so the error is always safe to print.
type InvalidURLError struct {
	URL     string
	Problem URLProblem
}

func (e *InvalidURLError) Error() string { return "invalid API URL " + e.URL }

// InvalidPlatformError is a THE_AGENT_CLI_DEFAULT_PLATFORM value that is neither telegram nor whatsapp.
type InvalidPlatformError struct {
	Value string
}

func (e *InvalidPlatformError) Error() string { return "invalid default platform " + e.Value }

// reason is the first line of the cause, without the path of an operating-system error and without
// the "toml: " prefix of the decoder.
func reason(err error) string {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		err = pathErr.Err
	}
	line, _, _ := strings.Cut(strings.TrimPrefix(err.Error(), "toml: "), "\n")
	return strings.TrimSpace(line)
}
