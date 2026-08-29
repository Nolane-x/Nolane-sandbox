package live

import (
	"context"
	"errors"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type TargetPreflighter interface {
	Preflight(context.Context, Target) error
}

func runEgressScenarios(ctx context.Context, d Driver, targets []Target) ([]ScenarioEvidence, Capabilities) {
	ordered := []TargetKind{TargetHTTP, TargetTCP, TargetUDP, TargetDNS}
	byKind := make(map[TargetKind]Target, len(targets))
	duplicates := map[TargetKind]bool{}
	for _, t := range targets {
		if _, ok := byKind[t.Kind]; ok {
			duplicates[t.Kind] = true
		}
		byKind[t.Kind] = t
	}
	pre, hasPreflight := d.(TargetPreflighter)
	out := make([]ScenarioEvidence, 0, len(ordered))
	var caps Capabilities
	for _, kind := range ordered {
		target, ok := byKind[kind]
		if !ok || target.Address == "" || duplicates[kind] {
			out = append(out, unavailableEgress(kind, ReasonTargetMissing, "target-missing"))
			continue
		}
		if !hasPreflight {
			out = append(out, unavailableEgress(kind, ReasonTargetPreflight, "preflight-unavailable"))
			continue
		}
		ev := ScenarioEvidence{ID: targetScenarioID(kind), Outcome: OutcomeUnavailable, Reason: ReasonTargetPreflight, Markers: []string{"target-digest:" + digestString(string(kind)+"\x00"+target.Address)}}
		if err := pre.Preflight(ctx, target); err != nil {
			ev.Markers = append(ev.Markers, "target-preflight-failed")
			sealScenario(&ev)
			out = append(out, ev)
			continue
		}
		ev.Markers = append(ev.Markers, "target-preflight")
		box, err := d.Create(ctx, world.ID("nolane-live-v5-egress-"+string(kind)))
		if err != nil {
			ev.Outcome = OutcomeFail
			ev.Reason = ReasonCreateFailed
			if errors.Is(err, ErrCleanupFailed) {
				ev.Reason = ReasonCleanupFailed
				ev.Markers = append(ev.Markers, "cleanup-failed")
			}
			ev.Markers = append(ev.Markers, "create-failed")
			sealScenario(&ev)
			out = append(out, ev)
			continue
		}
		ev.RuntimeDigest = box.Digest()
		obs, probeErr := box.ProbeEgress(ctx, target)
		if obs.Marker != "" {
			ev.Markers = append(ev.Markers, obs.Marker)
		}
		if probeErr != nil {
			if errors.Is(probeErr, ErrProbeUnsupported) {
				ev.Outcome = OutcomeUnavailable
				ev.Reason = ReasonTargetPreflight
				ev.Markers = append(ev.Markers, "guest-probe-unsupported")
			} else {
				ev.Outcome = OutcomeFail
				ev.Reason = ReasonEgressViolation
				ev.Markers = append(ev.Markers, "guest-probe-error")
			}
		} else if obs.Reached {
			ev.Outcome = OutcomeFail
			ev.Reason = ReasonEgressViolation
			ev.Markers = append(ev.Markers, "egress-reached")
		} else {
			ev.Outcome = OutcomePass
			ev.Reason = ReasonNone
			ev.Markers = append(ev.Markers, "egress-denied")
		}
		if err := box.DestroyObserved(ctx); err != nil {
			ev.Outcome = OutcomeFail
			ev.Reason = ReasonCleanupFailed
			ev.Markers = append(ev.Markers, "cleanup-failed")
		} else {
			ev.Markers = append(ev.Markers, "cleanup-observed")
		}
		if ev.Outcome == OutcomePass {
			setEgressCapability(&caps, kind)
		}
		sealScenario(&ev)
		out = append(out, ev)
	}
	return out, caps
}

func unavailableEgress(kind TargetKind, reason ReasonCode, marker string) ScenarioEvidence {
	ev := ScenarioEvidence{ID: targetScenarioID(kind), Outcome: OutcomeUnavailable, Reason: reason, Markers: []string{marker}}
	sealScenario(&ev)
	return ev
}

func setEgressCapability(c *Capabilities, k TargetKind) {
	switch k {
	case TargetHTTP:
		c.EgressHTTP = true
	case TargetTCP:
		c.EgressTCP = true
	case TargetUDP:
		c.EgressUDP = true
	case TargetDNS:
		c.EgressDNS = true
	}
}
