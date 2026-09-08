package cube

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrRuntimeRealizationUnavailable = errors.New("cube runtime realization observation unavailable")
	ErrInvalidRuntimeRealizationProof = errors.New("cube: invalid runtime realization proof")
	ErrStaleRuntimeRealizationProof   = errors.New("cube: stale runtime realization proof")
)

type runtimeRealizationProofSeal struct{}

var currentRuntimeRealizationProofSeal = &runtimeRealizationProofSeal{}

type RuntimeRealizationConfig struct {
	BaseURL    string
	HTTPClient *http.Client
}

// RuntimeRealizationObserver binds one current Cubelet realization epoch and
// one host runtime process identity from the same bounded management scrape.
// The exact observer instance is part of proof provenance.
type RuntimeRealizationObserver struct {
	endpoint string
	http     *http.Client
}

// RuntimeRealizationProof is an opaque Wave27 capability. Its descriptive
// inputs remain private and JSON cannot restore the seal or observer binding.
type RuntimeRealizationProof struct {
	epoch    RealizationEpochProof
	process  HostSandboxProcessIdentityProof
	observer *RuntimeRealizationObserver
	seal     *runtimeRealizationProofSeal
}

func NewRuntimeRealizationObserver(cfg RuntimeRealizationConfig) (*RuntimeRealizationObserver, error) {
	raw := strings.TrimSpace(cfg.BaseURL)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("%w: invalid Cubelet management endpoint", ErrRuntimeRealizationUnavailable)
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &RuntimeRealizationObserver{
		endpoint: strings.TrimRight(raw, "/") + hostResourceMetricsPath,
		http:     client,
	}, nil
}

func (p RuntimeRealizationProof) Valid() bool {
	if p.seal != currentRuntimeRealizationProofSeal || p.observer == nil || !p.epoch.Valid() {
		return false
	}
	if p.process.SandboxID == "" || strings.TrimSpace(p.process.SandboxID) != p.process.SandboxID ||
		p.process.Generation == 0 || p.process.HostPID == 0 || p.process.StartTimeTicks == 0 ||
		!canonicalLowerUUID(p.process.BootID) || p.process.CGroupPath == "" ||
		p.process.RuntimeRole != HostSandboxProcessRuntimeRoleCubeShimVMM ||
		p.process.Source != HostSandboxProcessIdentitySourceCubeBoxAddProc ||
		p.process.PlacedAt.IsZero() || p.process.BoundAt.IsZero() || p.process.BoundAt.Before(p.process.PlacedAt) {
		return false
	}
	return p.epoch.sandboxID == p.process.SandboxID && p.epoch.generation == p.process.Generation
}

func (p RuntimeRealizationProof) Epoch() (RealizationEpochProof, bool) {
	if !p.Valid() {
		return RealizationEpochProof{}, false
	}
	return p.epoch, true
}

func (p RuntimeRealizationProof) ProcessIdentity() (HostSandboxProcessIdentityProof, bool) {
	if !p.Valid() {
		return HostSandboxProcessIdentityProof{}, false
	}
	return p.process, true
}

func (o *RuntimeRealizationObserver) Observe(ctx context.Context, binding ResourceBinding) (RuntimeRealizationProof, error) {
	if o == nil || o.http == nil {
		return RuntimeRealizationProof{}, ErrRuntimeRealizationUnavailable
	}
	sandboxID := binding.sandboxID
	if sandboxID == "" || strings.TrimSpace(sandboxID) != sandboxID {
		return RuntimeRealizationProof{}, ErrInvalidResourceBinding
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.endpoint, nil)
	if err != nil {
		return RuntimeRealizationProof{}, fmt.Errorf("%w: %v", ErrRuntimeRealizationUnavailable, err)
	}
	resp, err := o.http.Do(req)
	if err != nil {
		return RuntimeRealizationProof{}, fmt.Errorf("%w: %v", ErrRuntimeRealizationUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return RuntimeRealizationProof{}, fmt.Errorf("%w: metrics HTTP status %d", ErrRuntimeRealizationUnavailable, resp.StatusCode)
	}

	epoch, process, err := parseRuntimeRealizationMetrics(io.LimitReader(resp.Body, 1<<20), sandboxID)
	if err != nil {
		return RuntimeRealizationProof{}, err
	}
	if !epoch.Valid() || epoch.sandboxID != process.SandboxID || epoch.generation != process.Generation {
		return RuntimeRealizationProof{}, ErrRuntimeRealizationUnavailable
	}
	return RuntimeRealizationProof{
		epoch:    epoch,
		process:  process,
		observer: o,
		seal:     currentRuntimeRealizationProofSeal,
	}, nil
}

func parseRuntimeRealizationMetrics(r io.Reader, sandboxID string) (RealizationEpochProof, HostSandboxProcessIdentityProof, error) {
	if r == nil || sandboxID == "" || strings.TrimSpace(sandboxID) != sandboxID {
		return RealizationEpochProof{}, HostSandboxProcessIdentityProof{}, ErrRuntimeRealizationUnavailable
	}
	var epoch RealizationEpochProof
	var process HostSandboxProcessIdentityProof
	epochFound := false
	processFound := false

	s := bufio.NewScanner(r)
	buf := make([]byte, 64*1024)
	s.Buffer(buf, 1<<20)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		switch {
		case isRealizationEpochMetricToken(fields[0]):
			name, labels, ok := splitMetricToken(fields[0])
			if !ok || name != realizationEpochMetric || len(fields) != 2 {
				return RealizationEpochProof{}, HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: malformed realization epoch metric", ErrRuntimeRealizationUnavailable)
			}
			metricSandboxID, ok := labels["sandbox_id"]
			if !ok || metricSandboxID == "" || strings.TrimSpace(metricSandboxID) != metricSandboxID {
				return RealizationEpochProof{}, HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: invalid realization epoch sandbox identity", ErrRuntimeRealizationUnavailable)
			}
			if metricSandboxID != sandboxID {
				continue
			}
			if epochFound {
				return RealizationEpochProof{}, HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: duplicate realization epoch", ErrRuntimeRealizationUnavailable)
			}
			parsed, err := exactRealizationEpochFromSample(labels, fields[1])
			if err != nil {
				return RealizationEpochProof{}, HostSandboxProcessIdentityProof{}, errors.Join(ErrRuntimeRealizationUnavailable, err)
			}
			epoch = parsed
			epochFound = true

		case isHostSandboxProcessIdentityMetricToken(fields[0]):
			name, labels, ok := splitMetricToken(fields[0])
			if !ok || name != hostSandboxProcessIdentityMetric || len(fields) != 2 {
				return RealizationEpochProof{}, HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: malformed host process identity metric", ErrRuntimeRealizationUnavailable)
			}
			metricSandboxID, ok := labels["sandbox_id"]
			if !ok || metricSandboxID == "" || strings.TrimSpace(metricSandboxID) != metricSandboxID {
				return RealizationEpochProof{}, HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: invalid host process sandbox identity", ErrRuntimeRealizationUnavailable)
			}
			if metricSandboxID != sandboxID {
				continue
			}
			if processFound {
				return RealizationEpochProof{}, HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: duplicate host process identity", ErrRuntimeRealizationUnavailable)
			}
			parsed, err := exactHostSandboxProcessIdentityFromSample(labels, fields[1])
			if err != nil {
				return RealizationEpochProof{}, HostSandboxProcessIdentityProof{}, errors.Join(ErrRuntimeRealizationUnavailable, err)
			}
			process = parsed
			processFound = true
		}
	}
	if err := s.Err(); err != nil {
		return RealizationEpochProof{}, HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: %v", ErrRuntimeRealizationUnavailable, err)
	}
	if !epochFound || !processFound {
		return RealizationEpochProof{}, HostSandboxProcessIdentityProof{}, ErrRuntimeRealizationUnavailable
	}
	if epoch.sandboxID != process.SandboxID || epoch.generation != process.Generation {
		return RealizationEpochProof{}, HostSandboxProcessIdentityProof{}, ErrRuntimeRealizationUnavailable
	}
	return epoch, process, nil
}

func (o *RuntimeRealizationObserver) ValidateCurrent(ctx context.Context, binding ResourceBinding, proof RuntimeRealizationProof) error {
	if o == nil || !proof.Valid() || proof.observer != o {
		return ErrInvalidRuntimeRealizationProof
	}
	if binding.sandboxID == "" || binding.sandboxID != proof.epoch.sandboxID {
		return ErrStaleRuntimeRealizationProof
	}
	current, err := o.Observe(ctx, binding)
	if err != nil {
		return err
	}
	if !sameRuntimeRealizationProof(current, proof) {
		return ErrStaleRuntimeRealizationProof
	}
	return nil
}

func sameRuntimeRealizationProof(a, b RuntimeRealizationProof) bool {
	return a.Valid() && b.Valid() &&
		a.observer == b.observer &&
		sameRealizationEpochProof(a.epoch, b.epoch) &&
		a.process == b.process
}
