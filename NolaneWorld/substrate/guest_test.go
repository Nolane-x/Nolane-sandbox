package substrate

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestProcessObservationProjectionHasNoRealizationSecrets(t *testing.T) {
	obs := ProcessObservation{ExitCode: 0, Stdout: []byte("ok"), ObservationDigest: strings.Repeat("a", 64)}
	raw, err := json.Marshal(obs)
	if err != nil { t.Fatal(err) }
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"sandbox", "token", "credential", "secret", "handle", "envd", "traffic"} {
		if strings.Contains(lower, forbidden) { t.Fatalf("projection contains forbidden realization field %q: %s", forbidden, raw) }
	}
}

func TestProcessRequestValidation(t *testing.T) {
	valid := ProcessRequest{Command: "printf ok", Timeout: time.Second, MaxOutputBytes: 4096}
	if err := valid.Validate(1 << 20); err != nil { t.Fatal(err) }
	for _, bad := range []ProcessRequest{
		{},
		{Command: "x\x00y", Timeout: time.Second, MaxOutputBytes: 1},
		{Command: "x", Timeout: 0, MaxOutputBytes: 1},
		{Command: "x", Timeout: time.Second, MaxOutputBytes: 0},
		{Command: "x", Timeout: time.Second, MaxOutputBytes: (1 << 20) + 1},
	} {
		if err := bad.Validate(1 << 20); err == nil { t.Fatalf("invalid request accepted: %+v", bad) }
	}
}
