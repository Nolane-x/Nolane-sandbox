package realmproof

import (
	"context"
	"errors"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type IngressObservation struct {
	Denied bool
	Marker string
}

type MeshObservation struct {
	Reached bool
	Marker  string
}

type RealmProfileSandbox interface {
	live.Sandbox
	ApplyRealmProfile(context.Context, realm.NetworkProfile) error
	ProbePublicIngress(context.Context) (IngressObservation, error)
}

type targetPreflighter interface {
	Preflight(context.Context, live.Target) error
}

type InternalMeshProber interface {
	ProbeInternalMesh(context.Context, live.Sandbox, realm.NetworkProfile) (MeshObservation, error)
}

type Runner struct {
	Mode            live.Mode
	Profile         realm.NetworkProfile
	RawPublicTarget live.Target
}

func (r Runner) Run(ctx context.Context, driver live.Driver) (Report, error) {
	mode := r.Mode
	if mode == "" {
		mode = live.ModeProbe
	}
	profile := r.Profile
	if profile == "" {
		profile = realm.R0InternalOnly
	}
	report := Report{
		SchemaVersion: 1,
		Profile:       profile,
		Mode:          mode,
		Substrate:     "cubesandbox",
		Status:        live.StatusUnavailable,
		Reason:        ReasonConfigMissing,
		Scenarios:     []ScenarioEvidence{},
	}
	if r.RawPublicTarget.Address != "" {
		report.TargetDigest = targetDigest(r.RawPublicTarget)
	}
	if driver == nil {
		return finishWithoutSandbox(report, live.StatusUnavailable, ReasonConfigMissing, mode)
	}

	fingerprint := driver.Fingerprint()
	report.EndpointDigest = fingerprint.EndpointDigest
	if err := driver.Health(ctx); err != nil {
		return finishWithoutSandbox(report, live.StatusUnavailable, ReasonControlUnhealthy, mode)
	}
	if r.RawPublicTarget.Address == "" {
		return finishWithoutSandbox(report, live.StatusUnavailable, ReasonTargetMissing, mode)
	}
	preflight, ok := driver.(targetPreflighter)
	if !ok {
		return finishWithoutSandbox(report, live.StatusUnavailable, ReasonDriverUnsupported, mode)
	}
	if err := preflight.Preflight(ctx, r.RawPublicTarget); err != nil {
		return finishWithoutSandbox(report, live.StatusUnavailable, ReasonTargetPreflight, mode)
	}

	box, err := driver.Create(ctx, world.ID("nolane-live-v9-realm-profile"))
	if err != nil || box == nil {
		return finishWithoutSandbox(report, live.StatusLiveFail, ReasonCreateFailed, mode)
	}
	profileBox, ok := box.(RealmProfileSandbox)
	if !ok {
		return finishWithCleanup(ctx, report, box, live.StatusUnavailable, ReasonDriverUnsupported, mode)
	}

	if err := profileBox.ApplyRealmProfile(ctx, profile); err != nil {
		report.Scenarios = append(report.Scenarios, failedScenario(ScenarioProfileApply, ReasonProfileApply, "profile-apply-failed"))
		return finishWithCleanup(ctx, report, box, live.StatusLiveFail, ReasonProfileApply, mode)
	}
	report.Scenarios = append(report.Scenarios, passedScenario(ScenarioProfileApply, "profile-applied"))

	if err := profileBox.Canary(ctx); err != nil {
		report.Scenarios = append(report.Scenarios, failedScenario(ScenarioGuestAfterProfile, ReasonGuestFailed, "guest-after-profile-failed"))
		return finishWithCleanup(ctx, report, box, live.StatusLiveFail, ReasonGuestFailed, mode)
	}
	report.Capabilities.GuestExecution = true
	report.Scenarios = append(report.Scenarios, passedScenario(ScenarioGuestAfterProfile, "guest-after-profile"))

	egress, err := profileBox.ProbeEgress(ctx, r.RawPublicTarget)
	if err != nil {
		reason := ReasonTargetPreflight
		marker := "raw-public-probe-unavailable"
		if errors.Is(err, live.ErrProbeUnsupported) {
			marker = "raw-public-probe-unsupported"
		}
		report.Scenarios = append(report.Scenarios, unavailableScenario(ScenarioRawPublicDenied, reason, marker))
		return finishWithCleanup(ctx, report, box, live.StatusUnavailable, reason, mode)
	}
	if egress.Reached {
		report.Scenarios = append(report.Scenarios, failedScenario(ScenarioRawPublicDenied, ReasonRawPublicReachable, "raw-public-reachable"))
		return finishWithCleanup(ctx, report, box, live.StatusLiveFail, ReasonRawPublicReachable, mode)
	}
	report.Capabilities.RawPublicDenied = true
	report.Scenarios = append(report.Scenarios, passedScenario(ScenarioRawPublicDenied, "raw-public-denied"))

	ingress, err := profileBox.ProbePublicIngress(ctx)
	if err != nil {
		report.Scenarios = append(report.Scenarios, unavailableScenario(ScenarioPublicIngressDenied, ReasonIngressUnavailable, "public-ingress-probe-unavailable"))
		return finishWithCleanup(ctx, report, box, live.StatusUnavailable, ReasonIngressUnavailable, mode)
	}
	if !ingress.Denied {
		report.Scenarios = append(report.Scenarios, failedScenario(ScenarioPublicIngressDenied, ReasonIngressViolation, "unauthenticated-ingress-reached-canary"))
		return finishWithCleanup(ctx, report, box, live.StatusLiveFail, ReasonIngressViolation, mode)
	}
	report.Capabilities.PublicIngressDenied = true
	report.Scenarios = append(report.Scenarios, passedScenario(ScenarioPublicIngressDenied, "unauthenticated-ingress-denied"))

	if mesh, ok := driver.(InternalMeshProber); ok {
		observation, meshErr := mesh.ProbeInternalMesh(ctx, box, profile)
		switch {
		case meshErr == nil && observation.Reached:
			report.Capabilities.InternalMeshVerified = true
			report.Scenarios = append(report.Scenarios, passedScenario(ScenarioInternalMesh, "private-mesh-observed"))
		case meshErr != nil && errors.Is(meshErr, live.ErrProbeUnsupported):
			report.Scenarios = append(report.Scenarios, unavailableScenario(ScenarioInternalMesh, ReasonMeshUnsupported, "private-mesh-unavailable"))
		default:
			report.Scenarios = append(report.Scenarios, failedScenario(ScenarioInternalMesh, ReasonMeshFailed, "private-mesh-not-observed"))
		}
	} else {
		report.Scenarios = append(report.Scenarios, unavailableScenario(ScenarioInternalMesh, ReasonMeshUnsupported, "private-mesh-unavailable"))
	}

	return finishWithCleanup(ctx, report, box, live.StatusLivePass, ReasonNone, mode)
}

func finishWithCleanup(ctx context.Context, report Report, box live.Sandbox, status live.Status, reason ReasonCode, mode live.Mode) (Report, error) {
	if err := box.DestroyObserved(ctx); err != nil {
		report.Scenarios = append(report.Scenarios, failedScenario(ScenarioCleanup, ReasonCleanupFailed, "cleanup-failed"))
		return finishWithoutSandbox(report, live.StatusLiveFail, ReasonCleanupFailed, mode)
	}
	report.Scenarios = append(report.Scenarios, passedScenario(ScenarioCleanup, "cleanup-observed"))
	return finishWithoutSandbox(report, status, reason, mode)
}

func finishWithoutSandbox(report Report, status live.Status, reason ReasonCode, mode live.Mode) (Report, error) {
	report.Status = status
	report.Reason = reason
	report.Approved = status == live.StatusLivePass
	if report.Approved {
		report.Reason = ReasonNone
	}
	if err := SealReport(&report); err != nil {
		return report, errors.Join(ErrInvalidReport, err)
	}
	switch status {
	case live.StatusLivePass:
		return report, nil
	case live.StatusUnavailable:
		if mode == live.ModeRequireLive {
			return report, ErrUnavailable
		}
		return report, nil
	default:
		return report, ErrFailed
	}
}

func passedScenario(id string, markers ...string) ScenarioEvidence {
	return ScenarioEvidence{ID: id, Outcome: live.OutcomePass, Reason: ReasonNone, Markers: markers}
}

func failedScenario(id string, reason ReasonCode, markers ...string) ScenarioEvidence {
	return ScenarioEvidence{ID: id, Outcome: live.OutcomeFail, Reason: reason, Markers: markers}
}

func unavailableScenario(id string, reason ReasonCode, markers ...string) ScenarioEvidence {
	return ScenarioEvidence{ID: id, Outcome: live.OutcomeUnavailable, Reason: reason, Markers: markers}
}

func targetDigest(target live.Target) string {
	return proofHash("nolane-live-realm-v9/target", string(target.Kind), target.Address, target.Expect)
}
