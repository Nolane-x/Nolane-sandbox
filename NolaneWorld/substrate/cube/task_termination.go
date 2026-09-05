package cube

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	taskRealizationOOMMetric = "cubesandbox_task_realization_oom_info"
	taskRealizationOOMSignal = "kernel.cgroup.memory.oom_kill"
)

// RealizationOOMProof proves only that the normalized kernel cgroup OOM-kill
// counter changed during the same controller-local realization as Outcome. It
// does not identify which process was killed and is not a main-task cause.
type RealizationOOMProof struct {
	SandboxID        string
	Generation       uint64
	CGroupPath       string
	Signal           string
	BaselineOOMKills uint64
	FinalOOMKills    uint64
	OOMKills         uint64
	BaselineAt       time.Time
	ObservedAt       time.Time
	ExitedAt         time.Time
	OutcomeSource    TaskOutcomeProofSource
}

type TaskTerminationEvidence struct {
	Outcome             TaskOutcomeProof
	RealizationOOM      *RealizationOOMProof
	HostProcessIdentity *HostSandboxProcessIdentityProof
}

// KernelOOMObservedDuringRealization reports whether a kernel cgroup OOM kill
// was observed inside the trusted realization window. known=false means the
// realization-scoped OOM proof is absent. This method does not identify the
// OOM victim and must not be used as a main-task OOM-killed classification.
func (e TaskTerminationEvidence) KernelOOMObservedDuringRealization() (observed bool, known bool) {
	if e.RealizationOOM == nil {
		return false, false
	}
	return e.RealizationOOM.OOMKills > 0, true
}

type TaskTerminationObserver struct {
	endpoint string
	http     *http.Client
}

func NewTaskTerminationObserver(cfg TaskOutcomeConfig) (*TaskTerminationObserver, error) {
	outcome, err := NewTaskOutcomeObserver(cfg)
	if err != nil {
		return nil, err
	}
	return &TaskTerminationObserver{endpoint: outcome.endpoint, http: outcome.http}, nil
}

func (o *TaskTerminationObserver) Observe(ctx context.Context, binding ResourceBinding) (TaskTerminationEvidence, bool, error) {
	if o == nil || o.http == nil {
		return TaskTerminationEvidence{}, false, ErrTaskOutcomeUnavailable
	}
	sandboxID := strings.TrimSpace(binding.sandboxID)
	if sandboxID == "" {
		return TaskTerminationEvidence{}, false, ErrInvalidResourceBinding
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.endpoint, nil)
	if err != nil {
		return TaskTerminationEvidence{}, false, fmt.Errorf("%w: %v", ErrTaskOutcomeUnavailable, err)
	}
	resp, err := o.http.Do(req)
	if err != nil {
		return TaskTerminationEvidence{}, false, fmt.Errorf("%w: %v", ErrTaskOutcomeUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TaskTerminationEvidence{}, false, fmt.Errorf("%w: metrics HTTP status %d", ErrTaskOutcomeUnavailable, resp.StatusCode)
	}

	return parseTaskTerminationMetrics(io.LimitReader(resp.Body, 1<<20), sandboxID)
}

func parseTaskTerminationMetrics(r io.Reader, sandboxID string) (TaskTerminationEvidence, bool, error) {
	var outcome TaskOutcomeProof
	var oom RealizationOOMProof
	var identity HostSandboxProcessIdentityProof
	outcomeFound := false
	oomFound := false
	identityFound := false

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
		case isTaskOutcomeMetricToken(fields[0]):
			name, labels, ok := splitMetricToken(fields[0])
			if !ok || name != taskOutcomeMetric || len(fields) != 2 {
				return TaskTerminationEvidence{}, false, fmt.Errorf("%w: malformed task outcome metric", ErrTaskOutcomeUnavailable)
			}
			metricSandboxID, hasSandboxID := labels["sandbox_id"]
			if !hasSandboxID || strings.TrimSpace(metricSandboxID) == "" {
				return TaskTerminationEvidence{}, false, fmt.Errorf("%w: task outcome metric has no sandbox identity", ErrTaskOutcomeUnavailable)
			}
			if metricSandboxID != sandboxID {
				continue
			}
			if outcomeFound {
				return TaskTerminationEvidence{}, false, fmt.Errorf("%w: duplicate task outcome proof for sandbox %q", ErrTaskOutcomeUnavailable, sandboxID)
			}
			proof, err := exactTaskOutcomeFromSample(labels, fields[1])
			if err != nil {
				return TaskTerminationEvidence{}, false, err
			}
			outcome = proof
			outcomeFound = true

		case isTaskRealizationOOMMetricToken(fields[0]):
			name, labels, ok := splitMetricToken(fields[0])
			if !ok || name != taskRealizationOOMMetric || len(fields) != 2 {
				return TaskTerminationEvidence{}, false, fmt.Errorf("%w: malformed task realization OOM metric", ErrTaskOutcomeUnavailable)
			}
			metricSandboxID, hasSandboxID := labels["sandbox_id"]
			if !hasSandboxID || strings.TrimSpace(metricSandboxID) == "" {
				return TaskTerminationEvidence{}, false, fmt.Errorf("%w: task realization OOM metric has no sandbox identity", ErrTaskOutcomeUnavailable)
			}
			if metricSandboxID != sandboxID {
				continue
			}
			if oomFound {
				return TaskTerminationEvidence{}, false, fmt.Errorf("%w: duplicate task realization OOM proof for sandbox %q", ErrTaskOutcomeUnavailable, sandboxID)
			}
			proof, err := exactRealizationOOMFromSample(labels, fields[1])
			if err != nil {
				return TaskTerminationEvidence{}, false, err
			}
			oom = proof
			oomFound = true

		case isHostSandboxProcessIdentityMetricToken(fields[0]):
			name, labels, ok := splitMetricToken(fields[0])
			if !ok || name != hostSandboxProcessIdentityMetric || len(fields) != 2 {
				return TaskTerminationEvidence{}, false, fmt.Errorf("%w: malformed host process identity metric", ErrTaskOutcomeUnavailable)
			}
			metricSandboxID, hasSandboxID := labels["sandbox_id"]
			if !hasSandboxID || strings.TrimSpace(metricSandboxID) == "" {
				return TaskTerminationEvidence{}, false, fmt.Errorf("%w: host process identity metric has no sandbox identity", ErrTaskOutcomeUnavailable)
			}
			if metricSandboxID != sandboxID {
				continue
			}
			if identityFound {
				return TaskTerminationEvidence{}, false, fmt.Errorf("%w: duplicate host process identity proof for sandbox %q", ErrTaskOutcomeUnavailable, sandboxID)
			}
			proof, err := exactHostSandboxProcessIdentityFromSample(labels, fields[1])
			if err != nil {
				return TaskTerminationEvidence{}, false, err
			}
			identity = proof
			identityFound = true
		}
	}
	if err := s.Err(); err != nil {
		return TaskTerminationEvidence{}, false, fmt.Errorf("%w: %v", ErrTaskOutcomeUnavailable, err)
	}

	if !outcomeFound {
		if oomFound {
			return TaskTerminationEvidence{}, false, fmt.Errorf("%w: realization OOM proof has no exact task outcome", ErrTaskOutcomeUnavailable)
		}
		if identityFound {
			return TaskTerminationEvidence{}, false, fmt.Errorf("%w: host process identity proof has no exact task outcome", ErrTaskOutcomeUnavailable)
		}
		return TaskTerminationEvidence{}, false, nil
	}

	evidence := TaskTerminationEvidence{Outcome: outcome}
	if oomFound {
		if err := correlateRealizationOOM(outcome, oom); err != nil {
			return TaskTerminationEvidence{}, false, err
		}
		proof := oom
		evidence.RealizationOOM = &proof
	}
	if identityFound {
		if err := correlateHostSandboxProcessIdentity(outcome, evidence.RealizationOOM, identity); err != nil {
			return TaskTerminationEvidence{}, false, err
		}
		proof := identity
		evidence.HostProcessIdentity = &proof
	}
	return evidence, true, nil
}

func isTaskRealizationOOMMetricToken(token string) bool {
	return token == taskRealizationOOMMetric || strings.HasPrefix(token, taskRealizationOOMMetric+"{")
}

func exactRealizationOOMFromSample(labels map[string]string, rawValue string) (RealizationOOMProof, error) {
	if len(labels) != 11 {
		return RealizationOOMProof{}, fmt.Errorf("%w: realization OOM metric must contain exactly eleven labels", ErrTaskOutcomeUnavailable)
	}
	for _, key := range []string{
		"sandbox_id",
		"generation",
		"cgroup_path",
		"signal",
		"baseline_oom_kills",
		"final_oom_kills",
		"oom_kills",
		"baseline_at",
		"observed_at",
		"exited_at",
		"outcome_source",
	} {
		if _, ok := labels[key]; !ok {
			return RealizationOOMProof{}, fmt.Errorf("%w: realization OOM metric missing %s", ErrTaskOutcomeUnavailable, key)
		}
	}

	value, err := strconv.ParseFloat(rawValue, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value != 1 {
		return RealizationOOMProof{}, fmt.Errorf("%w: realization OOM metric value must be exactly one", ErrTaskOutcomeUnavailable)
	}

	sandboxID := labels["sandbox_id"]
	if strings.TrimSpace(sandboxID) == "" || strings.TrimSpace(sandboxID) != sandboxID {
		return RealizationOOMProof{}, fmt.Errorf("%w: invalid realization OOM sandbox identity", ErrTaskOutcomeUnavailable)
	}
	generation, err := parseCanonicalUint(labels["generation"], 64)
	if err != nil || generation == 0 {
		return RealizationOOMProof{}, fmt.Errorf("%w: invalid realization OOM generation", ErrTaskOutcomeUnavailable)
	}
	cgroupPath := labels["cgroup_path"]
	if strings.TrimSpace(cgroupPath) == "" || strings.TrimSpace(cgroupPath) != cgroupPath {
		return RealizationOOMProof{}, fmt.Errorf("%w: invalid realization OOM cgroup path", ErrTaskOutcomeUnavailable)
	}
	if labels["signal"] != taskRealizationOOMSignal {
		return RealizationOOMProof{}, fmt.Errorf("%w: unsupported realization OOM signal %q", ErrTaskOutcomeUnavailable, labels["signal"])
	}

	baselineOOMKills, err := parseCanonicalUint(labels["baseline_oom_kills"], 64)
	if err != nil {
		return RealizationOOMProof{}, fmt.Errorf("%w: invalid realization OOM baseline counter", ErrTaskOutcomeUnavailable)
	}
	finalOOMKills, err := parseCanonicalUint(labels["final_oom_kills"], 64)
	if err != nil {
		return RealizationOOMProof{}, fmt.Errorf("%w: invalid realization OOM final counter", ErrTaskOutcomeUnavailable)
	}
	oomKills, err := parseCanonicalUint(labels["oom_kills"], 64)
	if err != nil {
		return RealizationOOMProof{}, fmt.Errorf("%w: invalid realization OOM delta", ErrTaskOutcomeUnavailable)
	}
	if finalOOMKills < baselineOOMKills || oomKills != finalOOMKills-baselineOOMKills {
		return RealizationOOMProof{}, fmt.Errorf("%w: inconsistent realization OOM counters", ErrTaskOutcomeUnavailable)
	}

	baselineAt, err := parseCanonicalUTCTimestamp(labels["baseline_at"])
	if err != nil {
		return RealizationOOMProof{}, fmt.Errorf("%w: invalid realization OOM baseline timestamp", ErrTaskOutcomeUnavailable)
	}
	observedAt, err := parseCanonicalUTCTimestamp(labels["observed_at"])
	if err != nil {
		return RealizationOOMProof{}, fmt.Errorf("%w: invalid realization OOM observation timestamp", ErrTaskOutcomeUnavailable)
	}
	exitedAt, err := parseCanonicalUTCTimestamp(labels["exited_at"])
	if err != nil {
		return RealizationOOMProof{}, fmt.Errorf("%w: invalid realization OOM exit timestamp", ErrTaskOutcomeUnavailable)
	}
	if exitedAt.Before(baselineAt) || observedAt.Before(exitedAt) {
		return RealizationOOMProof{}, fmt.Errorf("%w: invalid realization OOM timestamp ordering", ErrTaskOutcomeUnavailable)
	}

	outcomeSource := TaskOutcomeProofSource(labels["outcome_source"])
	if outcomeSource != TaskOutcomeProofSourceWait && outcomeSource != TaskOutcomeProofSourceState {
		return RealizationOOMProof{}, fmt.Errorf("%w: unsupported realization OOM outcome source %q", ErrTaskOutcomeUnavailable, outcomeSource)
	}

	return RealizationOOMProof{
		SandboxID:        sandboxID,
		Generation:       generation,
		CGroupPath:       cgroupPath,
		Signal:           taskRealizationOOMSignal,
		BaselineOOMKills: baselineOOMKills,
		FinalOOMKills:    finalOOMKills,
		OOMKills:         oomKills,
		BaselineAt:       baselineAt,
		ObservedAt:       observedAt,
		ExitedAt:         exitedAt,
		OutcomeSource:    outcomeSource,
	}, nil
}

func parseCanonicalUTCTimestamp(raw string) (time.Time, error) {
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil || value.IsZero() {
		return time.Time{}, fmt.Errorf("invalid timestamp")
	}
	value = value.UTC()
	if value.Format(time.RFC3339Nano) != raw {
		return time.Time{}, fmt.Errorf("non-canonical UTC RFC3339Nano timestamp")
	}
	return value, nil
}

func correlateRealizationOOM(outcome TaskOutcomeProof, oom RealizationOOMProof) error {
	if oom.SandboxID != outcome.SandboxID ||
		oom.Generation != outcome.Generation ||
		oom.OutcomeSource != outcome.Source ||
		!oom.ExitedAt.Equal(outcome.ExitedAt) {
		return fmt.Errorf("%w: realization OOM proof does not match exact task outcome", ErrTaskOutcomeUnavailable)
	}
	return nil
}
