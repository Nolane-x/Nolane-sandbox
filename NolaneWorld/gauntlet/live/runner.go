package live

import (
	"context"
	"errors"
	"fmt"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/authority"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type Runner struct {
	Mode    Mode
	Profile Profile
	Targets []Target
}

func (r Runner) Run(ctx context.Context, driver Driver) (Report, error) {
	mode := r.Mode
	if mode == "" {
		mode = ModeProbe
	}
	profile := r.Profile
	if profile == "" {
		profile = ProfileCore
	}
	if driver == nil {
		report := newUnavailableReport(profile, mode, ReasonConfigMissing, "", "")
		if mode == ModeRequireLive {
			return report, ErrLiveUnavailable
		}
		return report, nil
	}
	fp := driver.Fingerprint()
	if err := driver.Health(ctx); err != nil {
		report := newUnavailableReport(profile, mode, ReasonControlUnhealthy, fp.EndpointDigest, fp.TemplateDigest)
		if mode == ModeRequireLive {
			return report, errors.Join(ErrLiveUnavailable, err)
		}
		return report, nil
	}
	report := Report{SchemaVersion: 1, Profile: profile, Mode: mode, Substrate: "cubesandbox", Reason: ReasonNone, EndpointDigest: fp.EndpointDigest, TemplateDigest: fp.TemplateDigest, Capabilities: Capabilities{ControlPlane: true}}
	guest, gCaps := runGuestScenario(ctx, driver)
	report.Scenarios = append(report.Scenarios, guest)
	mergeCapabilities(&report.Capabilities, gCaps)
	if guest.Outcome == OutcomeFail {
		return finishFailure(report, guest.Reason)
	}
	if guest.Outcome == OutcomeUnavailable {
		return finishUnavailable(report, guest.Reason, mode)
	}

	snap, sCaps := runSnapshotAuthorityScenario(ctx, driver)
	report.Scenarios = append(report.Scenarios, snap)
	mergeCapabilities(&report.Capabilities, sCaps)
	if snap.Outcome == OutcomeFail {
		return finishFailure(report, snap.Reason)
	}
	if snap.Outcome == OutcomeUnavailable {
		return finishUnavailable(report, snap.Reason, mode)
	}

	if profile == ProfileFullEgress {
		egress, caps := runEgressScenarios(ctx, driver, r.Targets)
		report.Scenarios = append(report.Scenarios, egress...)
		mergeCapabilities(&report.Capabilities, caps)
		for _, ev := range egress {
			if ev.Outcome == OutcomeFail {
				return finishFailure(report, ev.Reason)
			}
			if ev.Outcome == OutcomeUnavailable {
				return finishUnavailable(report, ev.Reason, mode)
			}
		}
	}
	report.Status = StatusLivePass
	report.Approved = true
	report.Reason = ReasonNone
	sealReport(&report)
	if err := VerifyReport(report); err != nil {
		return report, errors.Join(ErrLiveFailed, err)
	}
	return report, nil
}

func finishFailure(report Report, reason ReasonCode) (Report, error) {
	report.Status = StatusLiveFail
	report.Reason = reason
	report.Approved = false
	sealReport(&report)
	return report, ErrLiveFailed
}
func finishUnavailable(report Report, reason ReasonCode, mode Mode) (Report, error) {
	report.Status = StatusUnavailable
	report.Reason = reason
	report.Approved = false
	sealReport(&report)
	if mode == ModeRequireLive {
		return report, ErrLiveUnavailable
	}
	return report, nil
}
func mergeCapabilities(dst *Capabilities, src Capabilities) {
	dst.ControlPlane = dst.ControlPlane || src.ControlPlane
	dst.GuestExecution = dst.GuestExecution || src.GuestExecution
	dst.SnapshotRollback = dst.SnapshotRollback || src.SnapshotRollback
	dst.CleanupObserved = dst.CleanupObserved || src.CleanupObserved
	dst.EgressHTTP = dst.EgressHTTP || src.EgressHTTP
	dst.EgressTCP = dst.EgressTCP || src.EgressTCP
	dst.EgressUDP = dst.EgressUDP || src.EgressUDP
	dst.EgressDNS = dst.EgressDNS || src.EgressDNS
}

func runGuestScenario(ctx context.Context, d Driver) (ScenarioEvidence, Capabilities) {
	ev := ScenarioEvidence{ID: ScenarioGuestExecution, Outcome: OutcomeFail, Reason: ReasonCreateFailed, Markers: []string{"control-plane"}}
	box, err := d.Create(ctx, world.ID("nolane-live-v5-guest"))
	if err != nil {
		if errors.Is(err, ErrCleanupFailed) {
			ev.Reason = ReasonCleanupFailed
			ev.Markers = append(ev.Markers, "cleanup-failed")
		}
		sealScenario(&ev)
		return ev, Capabilities{ControlPlane: true}
	}
	ev.RuntimeDigest = box.Digest()
	cleanup := func() error { return box.DestroyObserved(ctx) }
	if err := box.Canary(ctx); err != nil {
		ev.Reason = ReasonGuestFailed
		ev.Markers = append(ev.Markers, "guest-canary-failed")
		if cerr := cleanup(); cerr != nil {
			ev.Reason = ReasonCleanupFailed
			ev.Markers = append(ev.Markers, "cleanup-failed")
		} else {
			ev.Markers = append(ev.Markers, "cleanup-observed")
		}
		sealScenario(&ev)
		return ev, Capabilities{ControlPlane: true, CleanupObserved: ev.Reason != ReasonCleanupFailed}
	}
	ev.Markers = append(ev.Markers, "guest-canary")
	if err := cleanup(); err != nil {
		ev.Reason = ReasonCleanupFailed
		ev.Markers = append(ev.Markers, "cleanup-failed")
		sealScenario(&ev)
		return ev, Capabilities{ControlPlane: true, GuestExecution: true}
	}
	ev.Markers = append(ev.Markers, "cleanup-observed")
	ev.Outcome = OutcomePass
	ev.Reason = ReasonNone
	sealScenario(&ev)
	return ev, Capabilities{ControlPlane: true, GuestExecution: true, CleanupObserved: true}
}

func runSnapshotAuthorityScenario(ctx context.Context, d Driver) (ScenarioEvidence, Capabilities) {
	ev := ScenarioEvidence{ID: ScenarioSnapshotAuthority, Outcome: OutcomeFail, Reason: ReasonCreateFailed, Markers: []string{"control-plane"}}
	box, err := d.Create(ctx, world.ID("nolane-live-v5-snapshot"))
	if err != nil {
		if errors.Is(err, ErrCleanupFailed) {
			ev.Reason = ReasonCleanupFailed
			ev.Markers = append(ev.Markers, "cleanup-failed")
		}
		sealScenario(&ev)
		return ev, Capabilities{ControlPlane: true}
	}
	ev.RuntimeDigest = box.Digest()
	cleanupObserved := false
	cleanup := func() bool {
		if box.DestroyObserved(ctx) == nil {
			ev.Markers = append(ev.Markers, "cleanup-observed")
			cleanupObserved = true
			return true
		}
		ev.Markers = append(ev.Markers, "cleanup-failed")
		return false
	}
	fail := func(reason ReasonCode, marker string) (ScenarioEvidence, Capabilities) {
		ev.Reason = reason
		if marker != "" {
			ev.Markers = append(ev.Markers, marker)
		}
		if !cleanup() {
			ev.Reason = ReasonCleanupFailed
		}
		sealScenario(&ev)
		return ev, Capabilities{ControlPlane: true, CleanupObserved: cleanupObserved}
	}
	if err := box.PutSentinel(ctx, "A"); err != nil {
		return fail(ReasonSnapshotFailed, "sentinel-a-failed")
	}
	snap, err := box.Snapshot(ctx)
	if err != nil {
		return fail(ReasonSnapshotFailed, "snapshot-failed")
	}
	ev.Markers = append(ev.Markers, "snapshot-observed")
	if err := box.PutSentinel(ctx, "B"); err != nil {
		return fail(ReasonRollbackFailed, "sentinel-b-failed")
	}
	state, err := world.NewState(world.ID("nolane-live-v5-authority"))
	if err != nil {
		return fail(ReasonAuthorityFailed, "authority-state-failed")
	}
	oldEpoch := state.CurrentEpoch()
	state.AdvanceEpoch()
	if err := box.Rollback(ctx, snap); err != nil {
		return fail(ReasonRollbackFailed, "rollback-api-failed")
	}
	got, err := box.ReadSentinel(ctx)
	if err != nil || got != "A" {
		return fail(ReasonRollbackFailed, "rollback-state-not-restored")
	}
	ev.Markers = append(ev.Markers, "rollback-restored-a")
	broker, err := authority.NewBroker(state, allowPolicy{}, fixedExecutor{}, authority.NewMemoryLedger())
	if err != nil {
		return fail(ReasonAuthorityFailed, "authority-broker-failed")
	}
	_, err = broker.Execute(ctx, authority.Intent{WorldID: state.ID(), AuthorityEpoch: oldEpoch, ActionID: "stale-after-rollback", Kind: "gauntlet", Target: "proof"})
	if !errors.Is(err, world.ErrStaleEpoch) {
		return fail(ReasonAuthorityFailed, "stale-authority-accepted")
	}
	ev.Markers = append(ev.Markers, "stale-authority-denied")
	if !cleanup() {
		ev.Reason = ReasonCleanupFailed
		sealScenario(&ev)
		return ev, Capabilities{ControlPlane: true, SnapshotRollback: true}
	}
	ev.Outcome = OutcomePass
	ev.Reason = ReasonNone
	sealScenario(&ev)
	return ev, Capabilities{ControlPlane: true, SnapshotRollback: true, CleanupObserved: true}
}

type allowPolicy struct{}

func (allowPolicy) Evaluate(context.Context, authority.Intent) (authority.Decision, error) {
	return authority.Allow, nil
}

type fixedExecutor struct{}

func (fixedExecutor) Execute(context.Context, authority.Intent) ([]byte, error) {
	return []byte("effect"), nil
}

func targetScenarioID(k TargetKind) string {
	switch k {
	case TargetHTTP:
		return ScenarioEgressHTTP
	case TargetTCP:
		return ScenarioEgressTCP
	case TargetUDP:
		return ScenarioEgressUDP
	case TargetDNS:
		return ScenarioEgressDNS
	}
	return fmt.Sprintf("live.cube.egress-%s", k)
}
