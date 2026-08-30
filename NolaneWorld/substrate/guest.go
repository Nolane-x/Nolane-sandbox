package substrate

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrInvalidProcessRequest = errors.New("substrate: invalid process request")

type ProcessRequest struct {
	Command        string        `json:"command"`
	Timeout        time.Duration `json:"timeout"`
	MaxOutputBytes int64         `json:"max_output_bytes"`
}

func (r ProcessRequest) Validate(maxOutput int64) error {
	if r.Command == "" || len(r.Command) > 16*1024 || strings.IndexByte(r.Command, 0) >= 0 || r.Timeout <= 0 || r.Timeout > 30*time.Minute || r.MaxOutputBytes <= 0 || maxOutput <= 0 || r.MaxOutputBytes > maxOutput {
		return ErrInvalidProcessRequest
	}
	return nil
}

type ProcessObservation struct {
	ExitCode          int    `json:"exit_code"`
	Stdout            []byte `json:"stdout,omitempty"`
	Stderr            []byte `json:"stderr,omitempty"`
	StdoutTruncated   bool   `json:"stdout_truncated"`
	StderrTruncated   bool   `json:"stderr_truncated"`
	ObservationDigest string `json:"observation_digest"`
}

type GuestRuntime interface {
	Exec(context.Context, Handle, ProcessRequest) (ProcessObservation, error)
}
