package resourceproof

import live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"

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
	SchemaVersion int                `json:"schema_version"`
	Mode          live.Mode          `json:"mode"`
	Status        live.Status        `json:"status"`
	Reason        VerdictReason      `json:"reason"`
	Approved      bool               `json:"approved"`
	Binding       Binding            `json:"binding"`
	Requested     []ResourceDimension `json:"requested"`
	Results       []DimensionVerdict `json:"results"`
	Digest        string             `json:"digest"`
}

// ProjectVerdict is the public Wave 22 projection surface. This initial surface
// is fail-closed; semantic dimension reduction is added only after the API RED
// has been observed independently.
func (t TrustedReport) ProjectVerdict(req VerdictRequest) (Verdict, error) {
	verdict := Verdict{
		SchemaVersion: VerdictSchemaVersion,
		Mode:          req.Mode,
		Status:        live.StatusUnavailable,
		Reason:        VerdictReasonTrustedUnavailable,
		Requested:     append([]ResourceDimension(nil), req.Dimensions...),
	}
	if req.Mode == live.ModeRequireLive {
		return verdict, live.ErrLiveUnavailable
	}
	return verdict, nil
}
