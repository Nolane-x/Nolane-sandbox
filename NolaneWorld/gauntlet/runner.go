package gauntlet

import (
	"context"
	"errors"
	"sort"
)

type Runner struct{ policy Policy }

func NewRunner(policy Policy) *Runner { return &Runner{policy: policy} }

type runResult struct {
	err       error
	panicSeen bool
}

func (r *Runner) Run(ctx context.Context, scenarios []Scenario) (Report, error) {
	if r == nil || r.policy.validate() != nil || ctx == nil || len(scenarios) == 0 {
		return Report{}, ErrInvalidPolicy
	}
	type item struct {
		spec     ScenarioSpec
		scenario Scenario
	}
	items := make([]item, 0, len(scenarios))
	seen := make(map[string]struct{}, len(scenarios))
	for _, scenario := range scenarios {
		if scenario == nil {
			return Report{}, ErrInvalidScenario
		}
		spec := scenario.Spec()
		if err := spec.Validate(); err != nil {
			return Report{}, err
		}
		if _, ok := seen[spec.ID]; ok {
			return Report{}, ErrDuplicateScenario
		}
		seen[spec.ID] = struct{}{}
		items = append(items, item{spec: spec, scenario: scenario})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].spec.ID < items[j].spec.ID })

	report := Report{Version: ReportVersion, ProductID: r.policy.ProductID, Policy: r.policy, PolicyDigest: policyDigest(r.policy), Approved: true}
	for _, it := range items {
		evidence := r.runOne(ctx, it.spec, it.scenario)
		if evidence.Outcome != OutcomePass {
			report.Approved = false
		}
		report.Scenarios = append(report.Scenarios, evidence)
	}
	report.ReportDigest = reportDigest(report)
	if err := VerifyReport(report); err != nil {
		return Report{}, err
	}
	return report, nil
}

func (r *Runner) runOne(parent context.Context, spec ScenarioSpec, scenario Scenario) ScenarioEvidence {
	ctx, cancel := context.WithTimeout(parent, r.policy.ScenarioTimeout)
	defer cancel()
	probe := newProbe()
	ch := make(chan runResult, 1)
	go func() {
		res := runResult{}
		func() {
			defer func() {
				if recover() != nil {
					res.panicSeen = true
				}
			}()
			res.err = scenario.Run(ctx, probe)
		}()
		ch <- res
	}()

	var result runResult
	var code FailureCode
	var message string
	select {
	case result = <-ch:
		if result.panicSeen {
			code, message = FailurePanic, stableFailureMessage(FailurePanic)
		} else if result.err != nil {
			if errors.Is(result.err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				code, message = FailureTimeout, stableFailureMessage(FailureTimeout)
			} else {
				code, message = FailureExecution, stableFailureMessage(FailureExecution)
			}
		}
	case <-ctx.Done():
		code, message = FailureTimeout, stableFailureMessage(FailureTimeout)
	}
	probe.seal()
	events := probe.Events()
	if code == FailureNone {
		if !proofSatisfied(events) {
			code, message = FailureProofMissing, stableFailureMessage(FailureProofMissing)
		} else if missingMarker(spec.RequiredMarkers, events) {
			code, message = FailureMarkerMissing, stableFailureMessage(FailureMarkerMissing)
		}
	}
	outcome := OutcomePass
	if code != FailureNone {
		outcome = OutcomeFail
	}
	e := ScenarioEvidence{ID: spec.ID, Invariant: spec.Invariant, Attack: spec.Attack, ExpectedDefense: spec.ExpectedDefense, Severity: spec.Severity, RequiredMarkers: append([]string(nil), spec.RequiredMarkers...), Outcome: outcome, Events: events, FailureCode: code, FailureMessage: message}
	e.EvidenceDigest = scenarioDigest(e)
	return e
}

func proofSatisfied(events []Event) bool {
	var attack, boundary, denial bool
	for _, e := range events {
		switch e.Kind {
		case EventAttack:
			attack = true
		case EventBoundary:
			boundary = true
		case EventDenial:
			denial = true
		}
	}
	return attack && boundary && denial
}

func missingMarker(required []string, events []Event) bool {
	seen := make(map[string]struct{}, len(events))
	for _, e := range events {
		seen[e.Marker] = struct{}{}
	}
	for _, marker := range required {
		if _, ok := seen[marker]; !ok {
			return true
		}
	}
	return false
}

func stableFailureMessage(code FailureCode) string {
	switch code {
	case FailureExecution:
		return "scenario execution returned error"
	case FailurePanic:
		return "scenario panicked"
	case FailureTimeout:
		return "scenario timed out"
	case FailureProofMissing:
		return "proof-of-exercise incomplete"
	case FailureMarkerMissing:
		return "required marker missing"
	default:
		return ""
	}
}
