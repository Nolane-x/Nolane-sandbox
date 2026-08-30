package github

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"
)

type contentsPayloadWire struct {
	ContentB64      string `json:"content_b64"`
	CommitMessage   string `json:"commit_message"`
	ExpectedBlobSHA string `json:"expected_blob_sha,omitempty"`
}

type commentPayloadWire struct {
	Body string `json:"body"`
}

func decodeContentsPayload(raw []byte) (ContentsWritePayload, error) {
	if len(raw) == 0 || len(raw) > 2<<20 {
		return ContentsWritePayload{}, ErrInvalidProviderPayload
	}
	var wire contentsPayloadWire
	if err := decodeCanonicalJSON(raw, &wire); err != nil {
		return ContentsWritePayload{}, ErrInvalidProviderPayload
	}
	canonical, err := json.Marshal(wire)
	if err != nil || !bytes.Equal(canonical, raw) {
		return ContentsWritePayload{}, ErrInvalidProviderPayload
	}
	content, err := base64.StdEncoding.DecodeString(wire.ContentB64)
	if err != nil || base64.StdEncoding.EncodeToString(content) != wire.ContentB64 || len(content) > maxContentBytes {
		zeroBytes(content)
		return ContentsWritePayload{}, ErrInvalidProviderPayload
	}
	if !validText(wire.CommitMessage, 4096) || containsReservedMarker(wire.CommitMessage) {
		zeroBytes(content)
		return ContentsWritePayload{}, ErrInvalidProviderPayload
	}
	if wire.ExpectedBlobSHA != "" && !validLowerHexSHA(wire.ExpectedBlobSHA) {
		zeroBytes(content)
		return ContentsWritePayload{}, ErrInvalidProviderPayload
	}
	return ContentsWritePayload{Content: content, CommitMessage: wire.CommitMessage, ExpectedBlobSHA: wire.ExpectedBlobSHA}, nil
}

func decodeCommentPayload(raw []byte) (CommentPayload, error) {
	if len(raw) == 0 || len(raw) > maxCommentBytes+1024 {
		return CommentPayload{}, ErrInvalidProviderPayload
	}
	var wire commentPayloadWire
	if err := decodeCanonicalJSON(raw, &wire); err != nil {
		return CommentPayload{}, ErrInvalidProviderPayload
	}
	canonical, err := json.Marshal(wire)
	if err != nil || !bytes.Equal(canonical, raw) || !validText(wire.Body, maxCommentBytes) || containsReservedMarker(wire.Body) {
		return CommentPayload{}, ErrInvalidProviderPayload
	}
	return CommentPayload{Body: wire.Body}, nil
}

func decodeCanonicalJSON(raw []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return ErrInvalidProviderPayload
	}
	return nil
}

func validText(s string, max int) bool {
	if len(s) < 1 || len(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r == 0 || r == 0x7f || r < 0x20 && r != '\n' && r != '\t' {
			return false
		}
	}
	return true
}

func validLowerHexSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			if c < 'a' || c > 'f' {
				return false
			}
		}
	}
	return true
}

func containsReservedMarker(s string) bool {
	return strings.Contains(strings.ToLower(s), markerPrefix)
}

func actionMarker(idempotencyKey string) string {
	sum := sha256.Sum256([]byte(idempotencyKey))
	return markerPrefix + hex.EncodeToString(sum[:])
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
