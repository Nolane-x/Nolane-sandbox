package resourceproof

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
)

const VerdictSchemaVersion = 22

type ResourceDimension string

type DimensionVerdictState string

type VerdictReason string

const (
	ResourceCPU    ResourceDimension = "cpu"
	ResourceMemory ResourceDimension = "memory"
	ResourceDisk   ResourceDimension = "disk"

	DimensionVerified     DimensionVerdictState = "VERIFIED"
	DimensionUnavailable  DimensionVerdictState = "UNAVAILABLE"
	DimensionContradicted DimensionVerdictState = "CONTRADICTED"

	VerdictReasonNone                VerdictReason = "none"
	VerdictReasonTrustedUnavailable  VerdictReason = "trusted_report_unavailable"
	VerdictReasonBindingMismatch     VerdictReason = "binding_mismatch"
	VerdictReasonEvidenceUnavailable VerdictReason = "evidence_unavailable"
	VerdictReasonContradicted        VerdictReason = "evidence_contradicted"
)

var (
	ErrInvalidVerdict        = errors.New("live resource verdict: invalid verdict")
	ErrInvalidVerdictRequest = errors.New("live resource verdict: invalid request")
)

type VerdictRequest struct {
	Mode            live.Mode
	Dimensions      []ResourceDimension
	ExpectedBinding *Binding
}

type DimensionVerdict struct {
	Dimension ResourceDimension     `json:"dimension"`
	State     DimensionVerdictState `json:"state"`
	Reason    ReasonCode             `json:"reason"`
	Evidence  string                 `json:"evidence,omitempty"`
}

type Verdict struct {
	SchemaVersion int                 `json:"schema_version"`
	Mode          live.Mode           `json:"mode"`
	Status        live.Status         `json:"status"`
	Reason        VerdictReason       `json:"reason"`
	Approved      bool                `json:"approved"`
	Binding       Binding             `json:"binding"`
	Requested     []ResourceDimension `json:"requested"`
	Results       []DimensionVerdict  `json:"results"`
	Digest        string              `json:"digest"`
}

// ProjectVerdict is the public Wave 22 projection surface. It consumes only the
// package-owned TrustedReport wrapper. Public Report bytes, source labels, and
// caller-selected observations never enter this authority path.
func (t TrustedReport) ProjectVerdict(req VerdictRequest) (Verdict, error) {
	dimensions, err := canonicalVerdictDimensions(req.Mode, req.Dimensions)
	if err != nil {
		return Verdict{}, err
	}

	if err := VerifyTrustedReport(t); err != nil {
		return finishUnavailableVerdict(req.Mode, dimensions, Binding{}, VerdictReasonTrustedUnavailable)
	}

	report := t.Report()
	if !bindingValid(report.Binding) {
		return finishUnavailableVerdict(req.Mode, dimensions, report.Binding, VerdictReasonTrustedUnavailable)
	}
	if req.ExpectedBinding != nil && *req.ExpectedBinding != report.Binding {
		return finishUnavailableVerdict(req.Mode, dimensions, report.Binding, VerdictReasonBindingMismatch)
	}

	verdict := Verdict{
		SchemaVersion: VerdictSchemaVersion,
		Mode:          req.Mode,
		Binding:       report.Binding,
		Requested:     append([]ResourceDimension(nil), dimensions...),
		Results:       make([]DimensionVerdict, 0, len(dimensions)),
	}

	hasUnavailable := false
	hasContradiction := false
	for _, dimension := range dimensions {
		result := classifyTrustedDimension(report, dimension)
		verdict.Results = append(verdict.Results, result)
		switch result.State {
		case DimensionUnavailable:
			hasUnavailable = true
		case DimensionContradicted:
			hasContradiction = true
		}
	}

	switch {
	case hasContradiction:
		verdict.Status = live.StatusLiveFail
		verdict.Reason = VerdictReasonContradicted
		verdict.Approved = false
		verdict = sealVerdict(verdict)
		return verdict, live.ErrLiveFailed
	case hasUnavailable:
		verdict.Status = live.StatusUnavailable
		verdict.Reason = VerdictReasonEvidenceUnavailable
		verdict.Approved = false
		verdict = sealVerdict(verdict)
		if req.Mode == live.ModeRequireLive {
			return verdict, live.ErrLiveUnavailable
		}
		return verdict, nil
	default:
		verdict.Status = live.StatusLivePass
		verdict.Reason = VerdictReasonNone
		verdict.Approved = true
		verdict = sealVerdict(verdict)
		return verdict, nil
	}
}

func classifyTrustedDimension(report Report, dimension ResourceDimension) DimensionVerdict {
	switch dimension {
	case ResourceCPU:
		if report.CPU.Observation.Source != SourceLiveHost {
			return unavailableDimension(dimension)
		}
		reason := verifyCPU(report.CPU.Observation)
		if reason != ReasonNone {
			return DimensionVerdict{Dimension: dimension, State: DimensionContradicted, Reason: reason}
		}
		return DimensionVerdict{
			Dimension: dimension,
			State:     DimensionVerified,
			Reason:    ReasonNone,
			Evidence:  evidenceDigest("cpu", report.Binding, report.CPU.Observation),
		}
	case ResourceMemory:
		if report.Memory.Observation.Source != SourceLiveHost {
			return unavailableDimension(dimension)
		}
		reason := verifyMemory(report.Memory.Observation)
		if reason != ReasonNone {
			return DimensionVerdict{Dimension: dimension, State: DimensionContradicted, Reason: reason}
		}
		return DimensionVerdict{
			Dimension: dimension,
			State:     DimensionVerified,
			Reason:    ReasonNone,
			Evidence:  evidenceDigest("memory", report.Binding, report.Memory.Observation),
		}
	case ResourceDisk:
		return unavailableDimension(dimension)
	default:
		return unavailableDimension(dimension)
	}
}

func unavailableDimension(dimension ResourceDimension) DimensionVerdict {
	return DimensionVerdict{
		Dimension: dimension,
		State:     DimensionUnavailable,
		Reason:    ReasonEvidenceUnavailable,
	}
}

func finishUnavailableVerdict(mode live.Mode, dimensions []ResourceDimension, binding Binding, reason VerdictReason) (Verdict, error) {
	verdict := Verdict{
		SchemaVersion: VerdictSchemaVersion,
		Mode:          mode,
		Status:        live.StatusUnavailable,
		Reason:        reason,
		Approved:      false,
		Binding:       binding,
		Requested:     append([]ResourceDimension(nil), dimensions...),
		Results:       make([]DimensionVerdict, 0, len(dimensions)),
	}
	for _, dimension := range dimensions {
		verdict.Results = append(verdict.Results, unavailableDimension(dimension))
	}
	verdict = sealVerdict(verdict)
	if mode == live.ModeRequireLive {
		return verdict, live.ErrLiveUnavailable
	}
	return verdict, nil
}

func canonicalVerdictDimensions(mode live.Mode, dimensions []ResourceDimension) ([]ResourceDimension, error) {
	if mode != live.ModeProbe && mode != live.ModeRequireLive {
		return nil, ErrInvalidVerdictRequest
	}
	if len(dimensions) == 0 || len(dimensions) > 3 {
		return nil, ErrInvalidVerdictRequest
	}

	rank := map[ResourceDimension]int{
		ResourceCPU:    0,
		ResourceMemory: 1,
		ResourceDisk:   2,
	}
	out := append([]ResourceDimension(nil), dimensions...)
	seen := make(map[ResourceDimension]struct{}, len(out))
	for _, dimension := range out {
		if _, ok := rank[dimension]; !ok {
			return nil, ErrInvalidVerdictRequest
		}
		if _, duplicate := seen[dimension]; duplicate {
			return nil, ErrInvalidVerdictRequest
		}
		seen[dimension] = struct{}{}
	}
	sort.Slice(out, func(i, j int) bool { return rank[out[i]] < rank[out[j]] })
	return out, nil
}

// VerifyVerdict validates only the public serialized verdict domain. It proves
// canonical document integrity and fail-honest reduction; it does not recreate
// TrustedReport provenance or any capability authority.
func VerifyVerdict(verdict Verdict) error {
	if verdict.SchemaVersion != VerdictSchemaVersion {
		return ErrInvalidVerdict
	}
	dimensions, err := canonicalVerdictDimensions(verdict.Mode, verdict.Requested)
	if err != nil || !sameDimensions(dimensions, verdict.Requested) {
		return ErrInvalidVerdict
	}
	if len(verdict.Results) != len(dimensions) {
		return ErrInvalidVerdict
	}

	hasUnavailable := false
	hasContradiction := false
	for i, result := range verdict.Results {
		if result.Dimension != dimensions[i] {
			return ErrInvalidVerdict
		}
		switch result.State {
		case DimensionVerified:
			if result.Dimension == ResourceDisk || result.Reason != ReasonNone || strings.TrimSpace(result.Evidence) == "" {
				return ErrInvalidVerdict
			}
		case DimensionUnavailable:
			if result.Reason != ReasonEvidenceUnavailable || result.Evidence != "" {
				return ErrInvalidVerdict
			}
			hasUnavailable = true
		case DimensionContradicted:
			if result.Evidence != "" || !validContradictionReason(result.Dimension, result.Reason) {
				return ErrInvalidVerdict
			}
			hasContradiction = true
		default:
			return ErrInvalidVerdict
		}
	}

	switch {
	case hasContradiction:
		if verdict.Status != live.StatusLiveFail || verdict.Approved || verdict.Reason != VerdictReasonContradicted || !bindingValid(verdict.Binding) {
			return ErrInvalidVerdict
		}
	case hasUnavailable:
		if verdict.Status != live.StatusUnavailable || verdict.Approved || !validUnavailableVerdictReason(verdict.Reason) {
			return ErrInvalidVerdict
		}
		if verdict.Reason != VerdictReasonTrustedUnavailable && !bindingValid(verdict.Binding) {
			return ErrInvalidVerdict
		}
	default:
		if verdict.Status != live.StatusLivePass || !verdict.Approved || verdict.Reason != VerdictReasonNone || !bindingValid(verdict.Binding) {
			return ErrInvalidVerdict
		}
	}

	if verdict.Digest == "" || verdict.Digest != verdictDigest(verdict) {
		return ErrInvalidVerdict
	}
	return nil
}

func validContradictionReason(dimension ResourceDimension, reason ReasonCode) bool {
	switch dimension {
	case ResourceCPU:
		return reason == ReasonCPULimitMismatch || reason == ReasonCPUThrottleMissing
	case ResourceMemory:
		return reason == ReasonMemoryLimitMismatch || reason == ReasonMemoryOOMMissing
	default:
		return false
	}
}

func validUnavailableVerdictReason(reason VerdictReason) bool {
	return reason == VerdictReasonTrustedUnavailable || reason == VerdictReasonBindingMismatch || reason == VerdictReasonEvidenceUnavailable
}

func sameDimensions(a, b []ResourceDimension) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func verdictDigest(verdict Verdict) string {
	copy := verdict
	copy.Digest = ""
	raw, _ := json.Marshal(copy)
	h := sha256.Sum256(append([]byte("nolane.live-resource-verdict.v22\x00"), raw...))
	return hex.EncodeToString(h[:])
}

func sealVerdict(verdict Verdict) Verdict {
	verdict.Requested = append([]ResourceDimension(nil), verdict.Requested...)
	verdict.Results = append([]DimensionVerdict(nil), verdict.Results...)
	verdict.Digest = verdictDigest(verdict)
	return verdict
}

// MarshalVerdict emits verified canonical JSON. forbidden values are scanned
// verbatim so callers can ensure credentials or private locators never enter an
// artifact. The bytes remain descriptive evidence and carry no opaque authority.
func MarshalVerdict(verdict Verdict, forbidden ...string) ([]byte, error) {
	if err := VerifyVerdict(verdict); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(verdict, "", "  ")
	if err != nil {
		return nil, ErrInvalidVerdict
	}
	for _, secret := range forbidden {
		if secret != "" && strings.Contains(string(encoded), secret) {
			return nil, ErrInvalidVerdict
		}
	}
	return append(encoded, '\n'), nil
}
