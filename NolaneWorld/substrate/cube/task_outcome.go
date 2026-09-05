package cube

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const taskOutcomeMetric = "cubesandbox_task_outcome_info"

var ErrTaskOutcomeUnavailable = errors.New("cube exact task outcome observation unavailable")

type TaskOutcomeProofSource string

const (
	TaskOutcomeProofSourceWait  TaskOutcomeProofSource = "containerd.task.wait"
	TaskOutcomeProofSourceState TaskOutcomeProofSource = "containerd.task.state"
)

type TaskOutcomeProof struct {
	SandboxID  string
	Generation uint64
	ExitCode   uint32
	ExitedAt   time.Time
	Source     TaskOutcomeProofSource
}

type TaskOutcomeConfig struct {
	BaseURL    string
	HTTPClient *http.Client
}

type TaskOutcomeObserver struct {
	endpoint string
	http     *http.Client
}

func NewTaskOutcomeObserver(cfg TaskOutcomeConfig) (*TaskOutcomeObserver, error) {
	raw := strings.TrimSpace(cfg.BaseURL)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("%w: invalid Cubelet management endpoint", ErrTaskOutcomeUnavailable)
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &TaskOutcomeObserver{
		endpoint: strings.TrimRight(raw, "/") + hostResourceMetricsPath,
		http:     client,
	}, nil
}

func (o *TaskOutcomeObserver) Observe(ctx context.Context, binding ResourceBinding) (TaskOutcomeProof, bool, error) {
	if o == nil || o.http == nil {
		return TaskOutcomeProof{}, false, ErrTaskOutcomeUnavailable
	}
	sandboxID := strings.TrimSpace(binding.sandboxID)
	if sandboxID == "" {
		return TaskOutcomeProof{}, false, ErrInvalidResourceBinding
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.endpoint, nil)
	if err != nil {
		return TaskOutcomeProof{}, false, fmt.Errorf("%w: %v", ErrTaskOutcomeUnavailable, err)
	}
	resp, err := o.http.Do(req)
	if err != nil {
		return TaskOutcomeProof{}, false, fmt.Errorf("%w: %v", ErrTaskOutcomeUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TaskOutcomeProof{}, false, fmt.Errorf("%w: metrics HTTP status %d", ErrTaskOutcomeUnavailable, resp.StatusCode)
	}

	return parseTaskOutcomeMetrics(io.LimitReader(resp.Body, 1<<20), sandboxID)
}

func parseTaskOutcomeMetrics(r io.Reader, sandboxID string) (TaskOutcomeProof, bool, error) {
	var accepted TaskOutcomeProof
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
		if len(fields) == 0 || !isTaskOutcomeMetricToken(fields[0]) {
			continue
		}
		name, labels, ok := splitMetricToken(fields[0])
		if !ok || name != taskOutcomeMetric || len(fields) != 2 {
			return TaskOutcomeProof{}, false, fmt.Errorf("%w: malformed task outcome metric", ErrTaskOutcomeUnavailable)
		}
		metricSandboxID, hasSandboxID := labels["sandbox_id"]
		if !hasSandboxID || strings.TrimSpace(metricSandboxID) == "" {
			return TaskOutcomeProof{}, false, fmt.Errorf("%w: task outcome metric has no sandbox identity", ErrTaskOutcomeUnavailable)
		}
		if metricSandboxID != sandboxID {
			continue
		}
		if found {
			return TaskOutcomeProof{}, false, fmt.Errorf("%w: duplicate task outcome proof for sandbox %q", ErrTaskOutcomeUnavailable, sandboxID)
		}
		proof, err := exactTaskOutcomeFromSample(labels, fields[1])
		if err != nil {
			return TaskOutcomeProof{}, false, err
		}
		accepted = proof
		found = true
	}
	if err := s.Err(); err != nil {
		return TaskOutcomeProof{}, false, fmt.Errorf("%w: %v", ErrTaskOutcomeUnavailable, err)
	}
	if !found {
		return TaskOutcomeProof{}, false, nil
	}
	return accepted, true, nil
}

func isTaskOutcomeMetricToken(token string) bool {
	return token == taskOutcomeMetric || strings.HasPrefix(token, taskOutcomeMetric+"{")
}

func exactTaskOutcomeFromSample(labels map[string]string, rawValue string) (TaskOutcomeProof, error) {
	if len(labels) != 5 {
		return TaskOutcomeProof{}, fmt.Errorf("%w: task outcome metric must contain exactly five labels", ErrTaskOutcomeUnavailable)
	}
	for _, key := range []string{"sandbox_id", "generation", "source", "exit_code", "exited_at"} {
		if _, ok := labels[key]; !ok {
			return TaskOutcomeProof{}, fmt.Errorf("%w: task outcome metric missing %s", ErrTaskOutcomeUnavailable, key)
		}
	}

	value, err := strconv.ParseFloat(rawValue, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value != 1 {
		return TaskOutcomeProof{}, fmt.Errorf("%w: task outcome metric value must be exactly one", ErrTaskOutcomeUnavailable)
	}

	generation, err := parseCanonicalUint(labels["generation"], 64)
	if err != nil || generation == 0 {
		return TaskOutcomeProof{}, fmt.Errorf("%w: invalid task outcome generation", ErrTaskOutcomeUnavailable)
	}
	exitCode, err := parseCanonicalUint(labels["exit_code"], 32)
	if err != nil {
		return TaskOutcomeProof{}, fmt.Errorf("%w: invalid task outcome exit code", ErrTaskOutcomeUnavailable)
	}

	source := TaskOutcomeProofSource(labels["source"])
	if source != TaskOutcomeProofSourceWait && source != TaskOutcomeProofSourceState {
		return TaskOutcomeProof{}, fmt.Errorf("%w: unsupported task outcome source %q", ErrTaskOutcomeUnavailable, source)
	}

	exitedAt, err := time.Parse(time.RFC3339Nano, labels["exited_at"])
	if err != nil || exitedAt.IsZero() {
		return TaskOutcomeProof{}, fmt.Errorf("%w: invalid task outcome exit timestamp", ErrTaskOutcomeUnavailable)
	}
	exitedAt = exitedAt.UTC()
	if exitedAt.Format(time.RFC3339Nano) != labels["exited_at"] {
		return TaskOutcomeProof{}, fmt.Errorf("%w: task outcome exit timestamp is not canonical UTC RFC3339Nano", ErrTaskOutcomeUnavailable)
	}

	return TaskOutcomeProof{
		SandboxID:  labels["sandbox_id"],
		Generation: generation,
		ExitCode:   uint32(exitCode),
		ExitedAt:   exitedAt,
		Source:     source,
	}, nil
}

func parseCanonicalUint(raw string, bits int) (uint64, error) {
	if raw == "" {
		return 0, errors.New("empty integer")
	}
	value, err := strconv.ParseUint(raw, 10, bits)
	if err != nil || strconv.FormatUint(value, 10) != raw {
		return 0, errors.New("non-canonical unsigned integer")
	}
	return value, nil
}
