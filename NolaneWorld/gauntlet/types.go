package gauntlet

import (
	"context"
	"errors"
	"strings"
	"time"
)

const ProductNolaneSandbox = "nolane-sandbox"
const ReportVersion = 1

var (
	ErrInvalidScenario   = errors.New("gauntlet: invalid scenario")
	ErrInvalidPolicy     = errors.New("gauntlet: invalid policy")
	ErrDuplicateScenario = errors.New("gauntlet: duplicate scenario")
	ErrInvalidEvent      = errors.New("gauntlet: invalid event")
	ErrProbeSealed       = errors.New("gauntlet: probe sealed")
	ErrInvalidReport     = errors.New("gauntlet: invalid report")
)

type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

func (s Severity) valid() bool {
	switch s {
	case SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical:
		return true
	default:
		return false
	}
}

type Outcome string

const (
	OutcomePass Outcome = "pass"
	OutcomeFail Outcome = "fail"
)

type EventKind string

const (
	EventAttack      EventKind = "attack"
	EventBoundary    EventKind = "boundary"
	EventDenial      EventKind = "denial"
	EventObservation EventKind = "observation"
)

func (k EventKind) valid() bool {
	switch k {
	case EventAttack, EventBoundary, EventDenial, EventObservation:
		return true
	default:
		return false
	}
}

type Event struct {
	Marker string    `json:"marker"`
	Kind   EventKind `json:"kind"`
	Detail string    `json:"detail"`
}

type ScenarioSpec struct {
	ID              string   `json:"id"`
	Invariant       string   `json:"invariant"`
	Attack          string   `json:"attack"`
	ExpectedDefense string   `json:"expected_defense"`
	Severity        Severity `json:"severity"`
	RequiredMarkers []string `json:"required_markers,omitempty"`
}

func (s ScenarioSpec) Validate() error {
	if !nonBlank(s.ID) || !nonBlank(s.Invariant) || !nonBlank(s.Attack) || !nonBlank(s.ExpectedDefense) || !s.Severity.valid() {
		return ErrInvalidScenario
	}
	if len(s.ID) > 256 || len(s.Invariant) > 2048 || len(s.Attack) > 2048 || len(s.ExpectedDefense) > 2048 {
		return ErrInvalidScenario
	}
	seen := make(map[string]struct{}, len(s.RequiredMarkers))
	for _, marker := range s.RequiredMarkers {
		if !nonBlank(marker) || len(marker) > 256 {
			return ErrInvalidScenario
		}
		if _, ok := seen[marker]; ok {
			return ErrInvalidScenario
		}
		seen[marker] = struct{}{}
	}
	return nil
}

func nonBlank(s string) bool { return s != "" && strings.TrimSpace(s) == s }

type Scenario interface {
	Spec() ScenarioSpec
	Run(context.Context, *Probe) error
}

type ScenarioFunc struct {
	Definition ScenarioSpec
	Execute    func(context.Context, *Probe) error
}

func (s ScenarioFunc) Spec() ScenarioSpec {
	out := s.Definition
	out.RequiredMarkers = append([]string(nil), out.RequiredMarkers...)
	return out
}
func (s ScenarioFunc) Run(ctx context.Context, p *Probe) error {
	if s.Execute == nil {
		return ErrInvalidScenario
	}
	return s.Execute(ctx, p)
}

type Policy struct {
	ScenarioTimeout time.Duration `json:"scenario_timeout_ns"`
	ProductID       string        `json:"product_id"`
}

func (p Policy) validate() error {
	if p.ProductID != ProductNolaneSandbox || p.ScenarioTimeout <= 0 || p.ScenarioTimeout > 10*time.Minute {
		return ErrInvalidPolicy
	}
	return nil
}

type FailureCode string

const (
	FailureNone          FailureCode = ""
	FailureExecution     FailureCode = "execution_error"
	FailurePanic         FailureCode = "panic"
	FailureTimeout       FailureCode = "timeout"
	FailureProofMissing  FailureCode = "proof_missing"
	FailureMarkerMissing FailureCode = "required_marker_missing"
)

func (c FailureCode) valid() bool {
	switch c {
	case FailureNone, FailureExecution, FailurePanic, FailureTimeout, FailureProofMissing, FailureMarkerMissing:
		return true
	default:
		return false
	}
}

type ScenarioEvidence struct {
	ID              string      `json:"id"`
	Invariant       string      `json:"invariant"`
	Attack          string      `json:"attack"`
	ExpectedDefense string      `json:"expected_defense"`
	Severity        Severity    `json:"severity"`
	RequiredMarkers []string    `json:"required_markers,omitempty"`
	Outcome         Outcome     `json:"outcome"`
	Events          []Event     `json:"events"`
	FailureCode     FailureCode `json:"failure_code,omitempty"`
	FailureMessage  string      `json:"failure_message,omitempty"`
	EvidenceDigest  string      `json:"evidence_digest"`
}

type Report struct {
	Version      int                `json:"version"`
	ProductID    string             `json:"product_id"`
	Policy       Policy             `json:"policy"`
	PolicyDigest string             `json:"policy_digest"`
	Approved     bool               `json:"approved"`
	Scenarios    []ScenarioEvidence `json:"scenarios"`
	ReportDigest string             `json:"report_digest"`
}
