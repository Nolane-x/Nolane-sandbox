package capabilityproof

import (
	"context"
	"errors"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live/realmproof"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

func (r Runner) Run(ctx context.Context, driver live.Driver) (Report, error) {
	mode := r.Mode
	if mode == "" {
		mode = live.ModeProbe
	}
	if mode != live.ModeProbe && mode != live.ModeRequireLive {
		return Report{}, ErrInvalidReport
	}

	profile := r.Profile
	if profile == "" {
		profile = realm.R0InternalOnly
	}
	if profile != realm.R0InternalOnly && profile != realm.R1PublicRead && profile != realm.R2SupplyChain {
		return Report{}, ErrInvalidReport
	}

	report := Report{
		SchemaVersion: 1,
		Profile:       profile,
		Mode:          mode,
		Substrate:     "cubesandbox",
	}

	if driver == nil {
		substrateReport, realmReport, err := unavailableNestedProofs(ctx, profile, r.RawPublicTarget)
		if err != nil {
			return report, errors.Join(ErrFailed, err)
		}
		report.SubstrateProof = substrateReport
		report.RealmProof = realmReport
		return finish(report, live.StatusUnavailable, ReasonConfigMissing, mode)
	}

	locked := driver.Fingerprint()
	report.EndpointDigest = locked.EndpointDigest
	report.TemplateDigest = locked.TemplateDigest
	if locked.EndpointDigest == "" || locked.TemplateDigest == "" {
		substrateReport, realmReport, err := unavailableNestedProofs(ctx, profile, r.RawPublicTarget)
		if err != nil {
			return report, errors.Join(ErrFailed, err)
		}
		report.SubstrateProof = substrateReport
		report.RealmProof = realmReport
		return finish(report, live.StatusLiveFail, ReasonFingerprintInvalid, mode)
	}

	substrateReport, substrateErr := (live.Runner{
		Mode:    live.ModeProbe,
		Profile: live.ProfileCore,
	}).Run(ctx, driver)

	realmReport, realmErr := (realmproof.Runner{
		Mode:            live.ModeProbe,
		Profile:         profile,
		RawPublicTarget: r.RawPublicTarget,
	}).Run(ctx, driver)

	report.SubstrateProof = substrateReport
	report.RealmProof = realmReport

	if err := live.VerifyReport(substrateReport); err != nil {
		return report, errors.Join(ErrFailed, ErrInvalidReport, err)
	}
	if err := realmproof.VerifyReport(realmReport); err != nil {
		return report, errors.Join(ErrFailed, ErrInvalidReport, err)
	}

	if substrateReport.EndpointDigest != locked.EndpointDigest ||
		substrateReport.TemplateDigest != locked.TemplateDigest ||
		realmReport.EndpointDigest != locked.EndpointDigest {
		return finish(report, live.StatusLiveFail, ReasonEvidenceMismatch, mode)
	}

	// A valid nested LIVE_FAIL takes precedence over UNAVAILABLE so an observed
	// violation cannot be hidden by a second component that could not run.
	if substrateReport.Status == live.StatusLiveFail {
		return finish(report, live.StatusLiveFail, ReasonSubstrateFailed, mode)
	}
	if realmReport.Status == live.StatusLiveFail {
		return finish(report, live.StatusLiveFail, ReasonRealmFailed, mode)
	}
	if substrateReport.Status == live.StatusUnavailable {
		return finish(report, live.StatusUnavailable, ReasonSubstrateUnavailable, mode)
	}
	if realmReport.Status == live.StatusUnavailable {
		return finish(report, live.StatusUnavailable, ReasonRealmUnavailable, mode)
	}

	if substrateReport.Status != live.StatusLivePass || realmReport.Status != live.StatusLivePass || substrateErr != nil || realmErr != nil {
		return report, ErrFailed
	}
	capabilities, ok := deriveCapabilities(substrateReport, realmReport)
	if !ok {
		return report, ErrFailed
	}
	report.Capabilities = capabilities
	return finish(report, live.StatusLivePass, ReasonNone, mode)
}

func unavailableNestedProofs(ctx context.Context, profile realm.NetworkProfile, target live.Target) (live.Report, realmproof.Report, error) {
	substrateReport, substrateErr := (live.Runner{Mode: live.ModeProbe, Profile: live.ProfileCore}).Run(ctx, nil)
	if substrateErr != nil {
		return live.Report{}, realmproof.Report{}, substrateErr
	}
	realmReport, realmErr := (realmproof.Runner{
		Mode:            live.ModeProbe,
		Profile:         profile,
		RawPublicTarget: target,
	}).Run(ctx, nil)
	if realmErr != nil {
		return live.Report{}, realmproof.Report{}, realmErr
	}
	return substrateReport, realmReport, nil
}

func finish(report Report, status live.Status, reason ReasonCode, mode live.Mode) (Report, error) {
	report.Status = status
	report.Reason = reason
	report.Approved = status == live.StatusLivePass
	if !report.Approved {
		report.Capabilities = Capabilities{}
	}
	if err := sealReport(&report); err != nil {
		return report, errors.Join(ErrFailed, err)
	}
	switch status {
	case live.StatusLivePass:
		return report, nil
	case live.StatusUnavailable:
		if mode == live.ModeRequireLive {
			return report, ErrUnavailable
		}
		return report, nil
	case live.StatusLiveFail:
		return report, ErrFailed
	default:
		return report, ErrFailed
	}
}
