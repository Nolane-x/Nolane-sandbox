package cube

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
)

var ErrInvalidGuestRuntime = errors.New("cube: invalid guest runtime")

type GuestRuntime struct {
	client *Client
}

func NewGuestRuntime(client *Client) (*GuestRuntime, error) {
	if client == nil || client.dataHTTP == nil || client.maxBytes <= 0 {
		return nil, ErrInvalidGuestRuntime
	}
	return &GuestRuntime{client: client}, nil
}

func (r *GuestRuntime) Exec(ctx context.Context, h substrate.Handle, req substrate.ProcessRequest) (substrate.ProcessObservation, error) {
	if r == nil || r.client == nil || h == "" {
		return substrate.ProcessObservation{}, ErrInvalidGuestRuntime
	}
	if err := req.Validate(r.client.maxBytes); err != nil {
		return substrate.ProcessObservation{}, err
	}
	execCtx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()
	session, err := r.client.ConnectGuest(execCtx, h)
	if err != nil {
		return substrate.ProcessObservation{}, ErrGuestUnavailable
	}
	obs, err := session.RunCommand(execCtx, req.Command)
	if err != nil {
		if errors.Is(err, ErrGuestProtocol) || errors.Is(err, ErrResponseTooLarge) {
			return substrate.ProcessObservation{}, ErrGuestProtocol
		}
		return substrate.ProcessObservation{}, ErrGuestUnavailable
	}
	return projectProcessObservation(obs, req.MaxOutputBytes), nil
}

func projectProcessObservation(obs GuestObservation, max int64) substrate.ProcessObservation {
	stdout := []byte(obs.Stdout)
	stderr := []byte(obs.Stderr)
	stdoutTruncated := false
	stderrTruncated := false
	remaining := max
	if int64(len(stdout)) > remaining {
		stdout = append([]byte(nil), stdout[:remaining]...)
		stdoutTruncated = true
		remaining = 0
	} else {
		stdout = append([]byte(nil), stdout...)
		remaining -= int64(len(stdout))
	}
	if int64(len(stderr)) > remaining {
		stderr = append([]byte(nil), stderr[:remaining]...)
		stderrTruncated = true
	} else {
		stderr = append([]byte(nil), stderr...)
	}
	out := substrate.ProcessObservation{
		ExitCode: obs.ExitCode, Stdout: stdout, Stderr: stderr,
		StdoutTruncated: stdoutTruncated, StderrTruncated: stderrTruncated,
	}
	out.ObservationDigest = processObservationDigest(out)
	return out
}

func processObservationDigest(obs substrate.ProcessObservation) string {
	h := sha256.New()
	_, _ = h.Write([]byte("nolane.guest-observation.v1\x00"))	
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(int64(obs.ExitCode)))
	_, _ = h.Write(n[:])
	writeDigestField(h, obs.Stdout)
	writeDigestField(h, obs.Stderr)
	if obs.StdoutTruncated { _, _ = h.Write([]byte{1}) } else { _, _ = h.Write([]byte{0}) }
	if obs.StderrTruncated { _, _ = h.Write([]byte{1}) } else { _, _ = h.Write([]byte{0}) }
	return hex.EncodeToString(h.Sum(nil))
}

func writeDigestField(h interface{ Write([]byte) (int, error) }, b []byte) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(b)))
	_, _ = h.Write(n[:])
	_, _ = h.Write(b)
}
