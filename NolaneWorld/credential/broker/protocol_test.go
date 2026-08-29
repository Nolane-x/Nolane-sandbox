package broker

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/delegation"
)

func TestDecodeResponseAcceptsOnlyCanonicalSuccess(t *testing.T) {
	secret, err := decodeResponse([]byte(`{"version":1,"secret_b64":"U0VDUkVU"}`), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if string(secret) != "SECRET" {
		t.Fatalf("secret=%q", secret)
	}
}

func TestDecodeResponseRejectsAmbiguousOrInvalidMessages(t *testing.T) {
	cases := [][]byte{
		[]byte(`{"version":1,"secret_b64":"U0VDUkVU","extra":1}`),
		[]byte(`{"version":1,"secret_b64":"U0VDUkVU","secret_b64":"U0VDUkVU"}`),
		[]byte(`{ "version":1,"secret_b64":"U0VDUkVU"}`),
		[]byte(`{"secret_b64":"U0VDUkVU","version":1}`),
		[]byte(`{"version":1,"secret_b64":"%%%"}`),
		[]byte(`{"version":1,"secret_b64":""}`),
		[]byte(`{"version":1,"secret_b64":"U0VDUkVU","error_code":"denied"}`),
		[]byte(`{"version":1,"error_code":"invented"}`),
		[]byte(`{"version":2,"error_code":"not_found"}`),
		[]byte(`{"version":1}`),
	}
	for i, raw := range cases {
		if secret, err := decodeResponse(raw, 1<<20); !errors.Is(err, ErrBrokerProtocol) {
			t.Fatalf("case %d secret=%q err=%v", i, secret, err)
		}
	}
}

func TestDecodeResponseMapsBrokerDenialsToSecretUnavailable(t *testing.T) {
	for _, code := range []string{"not_found", "denied", "unavailable"} {
		raw := []byte(`{"version":1,"error_code":"` + code + `"}`)
		if _, err := decodeResponse(raw, 1<<20); !errors.Is(err, delegation.ErrSecretUnavailable) {
			t.Fatalf("code=%s err=%v", code, err)
		}
	}
}

func TestDecodeResponseRejectsDecodedSecretAboveConfiguredBound(t *testing.T) {
	if _, err := decodeResponse([]byte(`{"version":1,"secret_b64":"U0VDUkVU"}`), 5); !errors.Is(err, ErrBrokerResponseTooLarge) {
		t.Fatalf("err=%v", err)
	}
}

func TestEncodeRequestIsCanonicalHandleOnly(t *testing.T) {
	raw, err := encodeRequest(delegation.SecretHandle("kms/github/repo-a"))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"version":1,"handle":"kms/github/repo-a"}`
	if string(raw) != want {
		t.Fatalf("raw=%s", raw)
	}
	for _, forbidden := range []string{"world", "resource", "action", "provider", "github"} {
		if forbidden != "github" && strings.Contains(string(raw), forbidden) {
			t.Fatalf("request contains %q: %s", forbidden, raw)
		}
	}
}

func TestReadFrameRejectsOversizeAndTrailingBytes(t *testing.T) {
	var over bytes.Buffer
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], maxResponseFrame+1)
	over.Write(hdr[:])
	if _, err := readFrame(&over, maxResponseFrame); !errors.Is(err, ErrBrokerResponseTooLarge) {
		t.Fatalf("oversize err=%v", err)
	}

	var trailing bytes.Buffer
	body := []byte(`{"version":1,"error_code":"not_found"}`)
	binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
	trailing.Write(hdr[:])
	trailing.Write(body)
	trailing.WriteByte('x')
	if _, err := readFrame(&trailing, maxResponseFrame); !errors.Is(err, ErrBrokerProtocol) {
		t.Fatalf("trailing err=%v", err)
	}
}
