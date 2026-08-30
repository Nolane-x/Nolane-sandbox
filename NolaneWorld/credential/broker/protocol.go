package broker

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/delegation"
)

const (
	maxRequestFrame  uint32 = 4 * 1024
	maxResponseFrame uint32 = (1 << 20) + 4*1024
	hardMaxSecret           = 1 << 20
)

type requestMessage struct {
	Version int    `json:"version"`
	Handle  string `json:"handle"`
}

type responseMessage struct {
	Version   int    `json:"version"`
	SecretB64 string `json:"secret_b64,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

func encodeRequest(handle delegation.SecretHandle) ([]byte, error) {
	if err := delegation.ValidateSecretHandle(handle); err != nil {
		return nil, delegation.ErrSecretUnavailable
	}
	raw, err := json.Marshal(requestMessage{Version: 1, Handle: string(handle)})
	if err != nil || len(raw) == 0 || len(raw) > int(maxRequestFrame) {
		return nil, ErrBrokerProtocol
	}
	return raw, nil
}

func decodeResponse(raw []byte, maxSecret int) ([]byte, error) {
	if len(raw) == 0 || len(raw) > int(maxResponseFrame) || maxSecret <= 0 || maxSecret > hardMaxSecret {
		return nil, ErrBrokerProtocol
	}
	var msg responseMessage
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&msg); err != nil {
		return nil, ErrBrokerProtocol
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, ErrBrokerProtocol
	}
	canonical, err := json.Marshal(msg)
	if err != nil || !bytes.Equal(canonical, raw) {
		return nil, ErrBrokerProtocol
	}
	if msg.Version != 1 {
		return nil, ErrBrokerProtocol
	}
	if (msg.SecretB64 == "") == (msg.ErrorCode == "") {
		return nil, ErrBrokerProtocol
	}
	if msg.ErrorCode != "" {
		switch msg.ErrorCode {
		case "not_found", "denied", "unavailable":
			return nil, delegation.ErrSecretUnavailable
		default:
			return nil, ErrBrokerProtocol
		}
	}
	secret, err := base64.StdEncoding.DecodeString(msg.SecretB64)
	if err != nil || len(secret) == 0 {
		return nil, ErrBrokerProtocol
	}
	if len(secret) > maxSecret {
		zero(secret)
		return nil, ErrBrokerResponseTooLarge
	}
	return secret, nil
}

func writeFrame(w io.Writer, body []byte, max uint32) error {
	if w == nil || len(body) == 0 || uint64(len(body)) > uint64(max) {
		return ErrBrokerProtocol
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(body)))
	if err := writeAll(w, header[:]); err != nil {
		return ErrBrokerUnavailable
	}
	if err := writeAll(w, body); err != nil {
		return ErrBrokerUnavailable
	}
	return nil
}

func readFrame(r io.Reader, max uint32) ([]byte, error) {
	if r == nil || max == 0 {
		return nil, ErrBrokerProtocol
	}
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, ErrBrokerUnavailable
	}
	n := binary.BigEndian.Uint32(header[:])
	if n == 0 {
		return nil, ErrBrokerProtocol
	}
	if n > max {
		return nil, ErrBrokerResponseTooLarge
	}
	body := make([]byte, int(n))
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, ErrBrokerUnavailable
	}
	var extra [1]byte
	nExtra, err := r.Read(extra[:])
	if nExtra != 0 || !errors.Is(err, io.EOF) {
		zero(body)
		return nil, ErrBrokerProtocol
	}
	return body, nil
}

func writeAll(w io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := w.Write(p)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrUnexpectedEOF
		}
		p = p[n:]
	}
	return nil
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
