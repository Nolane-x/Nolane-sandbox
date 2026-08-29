package delegation

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"time"
)

func GrantDigest(in Grant) (string, error) {
	g, err := canonicalGrant(in)
	if err != nil {
		return "", err
	}
	fields := [][]byte{
		[]byte(g.ID),
		[]byte(g.WorldID),
		u64(uint64(g.AuthorityEpoch)),
		[]byte(g.Adapter),
		[]byte(g.Resource),
		[]byte(g.SecretHandle),
		timeBytes(g.IssuedAt),
		timeBytes(g.ExpiresAt),
	}
	for _, op := range g.Operations {
		fields = append(fields, []byte(op))
	}
	return hashFields("nolane-delegation-grant-v1", fields...), nil
}

func SecretHandleDigest(handle SecretHandle) string {
	return hashFields("nolane-delegation-secret-handle-v1", []byte(handle))
}

func IntentDigest(in Intent) (string, error) {
	if err := validateIntent(in); err != nil {
		return "", err
	}
	return hashFields(
		"nolane-delegation-intent-v1",
		[]byte(in.WorldID),
		u64(uint64(in.AuthorityEpoch)),
		[]byte(in.ActionID),
		[]byte(in.DelegationID),
		[]byte(in.Operation),
		[]byte(in.Resource),
		in.Payload,
	), nil
}

func requestDigest(in Intent, g Grant) (string, string, string, error) {
	intentDigest, err := IntentDigest(in)
	if err != nil {
		return "", "", "", err
	}
	grantDigest, err := GrantDigest(g)
	if err != nil {
		return "", "", "", err
	}
	handleDigest := SecretHandleDigest(g.SecretHandle)
	return hashFields("nolane-delegation-request-v1", []byte(intentDigest), []byte(grantDigest), []byte(handleDigest)), grantDigest, handleDigest, nil
}

func effectDigest(evidence []byte) string {
	return hashFields("nolane-delegation-effect-v1", evidence)
}

func hashFields(domain string, fields ...[]byte) string {
	h := sha256.New()
	write := func(b []byte) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(b)))
		_, _ = h.Write(n[:])
		_, _ = h.Write(b)
	}
	write([]byte(domain))
	for _, field := range fields {
		write(field)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func u64(v uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	return b[:]
}

func timeBytes(t time.Time) []byte {
	return []byte(t.UTC().Format(time.RFC3339Nano))
}
