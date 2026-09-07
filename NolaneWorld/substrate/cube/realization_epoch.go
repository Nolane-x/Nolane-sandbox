package cube

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const realizationEpochMetric = "cubesandbox_realization_epoch_info"

var ErrRealizationEpochUnavailable = errors.New("cube realization epoch observation unavailable")

type realizationEpochProofSeal struct{}

var currentRealizationEpochProofSeal = &realizationEpochProofSeal{}

// RealizationEpochProof is an opaque package-owned capability minted only from
// a strict observation of Cubelet's existing trusted management metrics path.
// Its descriptive tuple is intentionally unexported and cannot reconstruct the
// seal outside package cube.
type RealizationEpochProof struct {
	sandboxID  string
	generation uint64
	token      [32]byte
	seal       *realizationEpochProofSeal
}

func (p RealizationEpochProof) Valid() bool {
	return p.seal == currentRealizationEpochProofSeal &&
		p.sandboxID != "" && strings.TrimSpace(p.sandboxID) == p.sandboxID &&
		p.generation != 0 && p.token != ([32]byte{})
}

func (p RealizationEpochProof) SandboxID() (string, bool) {
	if !p.Valid() {
		return "", false
	}
	return p.sandboxID, true
}

func (p RealizationEpochProof) Generation() (uint64, bool) {
	if !p.Valid() {
		return 0, false
	}
	return p.generation, true
}

func (p RealizationEpochProof) TokenHex() (string, bool) {
	if !p.Valid() {
		return "", false
	}
	return hex.EncodeToString(p.token[:]), true
}

type RealizationEpochConfig struct {
	BaseURL    string
	HTTPClient *http.Client
}

type RealizationEpochObserver struct {
	endpoint string
	http     *http.Client
}

func NewRealizationEpochObserver(cfg RealizationEpochConfig) (*RealizationEpochObserver, error) {
	raw := strings.TrimSpace(cfg.BaseURL)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("%w: invalid Cubelet management endpoint", ErrRealizationEpochUnavailable)
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &RealizationEpochObserver{
		endpoint: strings.TrimRight(raw, "/") + hostResourceMetricsPath,
		http:     client,
	}, nil
}

func (o *RealizationEpochObserver) Observe(ctx context.Context, binding ResourceBinding) (RealizationEpochProof, bool, error) {
	if o == nil || o.http == nil {
		return RealizationEpochProof{}, false, ErrRealizationEpochUnavailable
	}
	sandboxID := binding.sandboxID
	if sandboxID == "" || strings.TrimSpace(sandboxID) != sandboxID {
		return RealizationEpochProof{}, false, ErrInvalidResourceBinding
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.endpoint, nil)
	if err != nil {
		return RealizationEpochProof{}, false, fmt.Errorf("%w: %v", ErrRealizationEpochUnavailable, err)
	}
	resp, err := o.http.Do(req)
	if err != nil {
		return RealizationEpochProof{}, false, fmt.Errorf("%w: %v", ErrRealizationEpochUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return RealizationEpochProof{}, false, fmt.Errorf("%w: metrics HTTP status %d", ErrRealizationEpochUnavailable, resp.StatusCode)
	}
	return parseRealizationEpochMetrics(io.LimitReader(resp.Body, 1<<20), sandboxID)
}

func parseRealizationEpochMetrics(r io.Reader, sandboxID string) (RealizationEpochProof, bool, error) {
	if r == nil || sandboxID == "" || strings.TrimSpace(sandboxID) != sandboxID {
		return RealizationEpochProof{}, false, fmt.Errorf("%w: invalid sandbox identity", ErrRealizationEpochUnavailable)
	}

	var accepted RealizationEpochProof
	found := false
	s := bufio.NewScanner(r)
	buf := make([]byte, 64*1024)
	s.Buffer(buf, 1<<20)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 || !isRealizationEpochMetricToken(fields[0]) {
			continue
		}
		name, labels, ok := splitMetricToken(fields[0])
		if !ok || name != realizationEpochMetric || len(fields) != 2 {
			return RealizationEpochProof{}, false, fmt.Errorf("%w: malformed realization epoch metric", ErrRealizationEpochUnavailable)
		}
		metricSandboxID, hasSandboxID := labels["sandbox_id"]
		if !hasSandboxID || metricSandboxID == "" || strings.TrimSpace(metricSandboxID) != metricSandboxID {
			return RealizationEpochProof{}, false, fmt.Errorf("%w: invalid realization epoch sandbox identity", ErrRealizationEpochUnavailable)
		}
		if metricSandboxID != sandboxID {
			continue
		}
		if found {
			return RealizationEpochProof{}, false, fmt.Errorf("%w: duplicate realization epoch proof for sandbox %q", ErrRealizationEpochUnavailable, sandboxID)
		}
		proof, err := exactRealizationEpochFromSample(labels, fields[1])
		if err != nil {
			return RealizationEpochProof{}, false, err
		}
		accepted = proof
		found = true
	}
	if err := s.Err(); err != nil {
		return RealizationEpochProof{}, false, fmt.Errorf("%w: %v", ErrRealizationEpochUnavailable, err)
	}
	if !found {
		return RealizationEpochProof{}, false, nil
	}
	return accepted, true, nil
}

func isRealizationEpochMetricToken(token string) bool {
	return token == realizationEpochMetric || strings.HasPrefix(token, realizationEpochMetric+"{")
}

func exactRealizationEpochFromSample(labels map[string]string, rawValue string) (RealizationEpochProof, error) {
	if len(labels) != 3 {
		return RealizationEpochProof{}, fmt.Errorf("%w: realization epoch metric must contain exactly three labels", ErrRealizationEpochUnavailable)
	}
	for _, key := range []string{"sandbox_id", "generation", "token"} {
		if _, ok := labels[key]; !ok {
			return RealizationEpochProof{}, fmt.Errorf("%w: realization epoch metric missing %s", ErrRealizationEpochUnavailable, key)
		}
	}
	if rawValue != "1" {
		return RealizationEpochProof{}, fmt.Errorf("%w: realization epoch metric value must be canonical one", ErrRealizationEpochUnavailable)
	}

	sandboxID := labels["sandbox_id"]
	if sandboxID == "" || strings.TrimSpace(sandboxID) != sandboxID {
		return RealizationEpochProof{}, fmt.Errorf("%w: invalid realization epoch sandbox identity", ErrRealizationEpochUnavailable)
	}
	generation, err := parseCanonicalUint(labels["generation"], 64)
	if err != nil || generation == 0 {
		return RealizationEpochProof{}, fmt.Errorf("%w: invalid realization epoch generation", ErrRealizationEpochUnavailable)
	}
	token, err := canonicalRealizationEpochToken(labels["token"])
	if err != nil {
		return RealizationEpochProof{}, err
	}

	return RealizationEpochProof{
		sandboxID:  sandboxID,
		generation: generation,
		token:      token,
		seal:       currentRealizationEpochProofSeal,
	}, nil
}

func canonicalRealizationEpochToken(raw string) ([32]byte, error) {
	if len(raw) != 64 {
		return [32]byte{}, fmt.Errorf("%w: realization epoch token must be exactly 64 lowercase hex characters", ErrRealizationEpochUnavailable)
	}
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != raw {
		return [32]byte{}, fmt.Errorf("%w: realization epoch token is not canonical lowercase hex", ErrRealizationEpochUnavailable)
	}
	var token [32]byte
	copy(token[:], decoded)
	if token == ([32]byte{}) {
		return [32]byte{}, fmt.Errorf("%w: realization epoch token must be non-zero", ErrRealizationEpochUnavailable)
	}
	return token, nil
}
