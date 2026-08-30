package broker

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/delegation"
)

var (
	ErrBrokerUnavailable     = errors.New("credential broker: unavailable")
	ErrBrokerPeerMismatch    = errors.New("credential broker: peer identity mismatch")
	ErrBrokerUnsupported     = errors.New("credential broker: peer authentication unsupported")
	ErrBrokerProtocol        = errors.New("credential broker: invalid protocol")
	ErrBrokerResponseTooLarge = errors.New("credential broker: response too large")
)

type Config struct {
	SocketPath      string
	ExpectedPeerUID uint32
	MaxSecretBytes  int
}

type Vault struct {
	socketPath      string
	expectedPeerUID uint32
	maxSecretBytes  int
}

func New(cfg Config) (*Vault, error) {
	if cfg.SocketPath == "" || !filepath.IsAbs(cfg.SocketPath) || filepath.Clean(cfg.SocketPath) != cfg.SocketPath {
		return nil, ErrBrokerProtocol
	}
	if cfg.MaxSecretBytes == 0 {
		cfg.MaxSecretBytes = hardMaxSecret
	}
	if cfg.MaxSecretBytes < 0 || cfg.MaxSecretBytes > hardMaxSecret {
		return nil, ErrBrokerProtocol
	}
	return &Vault{socketPath: cfg.SocketPath, expectedPeerUID: cfg.ExpectedPeerUID, maxSecretBytes: cfg.MaxSecretBytes}, nil
}

func (v *Vault) Use(ctx context.Context, handle delegation.SecretHandle, fn func(delegation.Secret) error) error {
	if v == nil || ctx == nil || fn == nil {
		return ErrBrokerProtocol
	}
	if err := delegation.ValidateSecretHandle(handle); err != nil {
		return delegation.ErrSecretUnavailable
	}
	select {
	case <-ctx.Done():
		return ErrBrokerUnavailable
	default:
	}

	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", v.socketPath)
	if err != nil {
		return ErrBrokerUnavailable
	}
	defer conn.Close()
	if err := verifyPeer(conn, v.expectedPeerUID); err != nil {
		return err
	}

	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return ErrBrokerUnavailable
		}
	}
	stopCancelWatch := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.SetDeadline(time.Now())
		case <-stopCancelWatch:
		}
	}()
	defer close(stopCancelWatch)

	rawRequest, err := encodeRequest(handle)
	if err != nil {
		return err
	}
	if err := writeFrame(conn, rawRequest, maxRequestFrame); err != nil {
		return err
	}
	if unixConn, ok := conn.(*net.UnixConn); ok {
		if err := unixConn.CloseWrite(); err != nil {
			return ErrBrokerUnavailable
		}
	}

	rawResponse, err := readFrame(conn, maxResponseFrame)
	if err != nil {
		return err
	}
	defer zero(rawResponse)
	secret, err := decodeResponse(rawResponse, v.maxSecretBytes)
	if err != nil {
		return err
	}
	defer zero(secret)
	return delegation.WithSecretLease(secret, fn)
}
