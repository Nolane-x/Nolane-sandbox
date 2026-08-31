package capabilityproof

import (
	"context"
	"errors"
	"strings"
	"testing"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live/realmproof"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type fusionDriver struct {
	fingerprints              []live.Fingerprint
	fingerprintCalls          int
	healthErr                 error
	preflightErr              error
	substrateRollbackNoRestore bool
	realmEgressReachable      bool
	realmIngressDenied        bool
	realmApplyErr             error
	realmCanaryErr            error
	cleanupErr                error
}

func newFusionDriver() *fusionDriver {
	return &fusionDriver{
		fingerprints: []live.Fingerprint{{
			EndpointDigest: strings.Repeat("e", 64),
			TemplateDigest: strings.Repeat("t", 64),
		}},
		realmIngressDenied: true,
	}
}

func (d *fusionDriver) Fingerprint() live.Fingerprint {
	if len(d.fingerprints) == 0 {
		return live.Fingerprint{}
	}
	idx := d.fingerprintCalls
	if idx >= len(d.fingerprints) {
		idx = len(d.fingerprints) - 1
	}
	d.fingerprintCalls++
	return d.fingerprints[idx]
}

func (d *fusionDriver) Health(context.Context) error { return d.healthErr }

func (d *fusionDriver) Create(_ context.Context, id world.ID) (live.Sandbox, error) {
	return &fusionBox{driver: d, id: id}, nil
}

func (d *fusionDriver) Preflight(context.Context, live.Target) error { return d.preflightErr }

type fusionBox struct {
	driver        *fusionDriver
	id            world.ID
	state         string
	snapshotState string
}

func (b *fusionBox) Digest() string { return "runtime:" + string(b.id) }

func (b *fusionBox) Canary(context.Context) error {
	if strings.Contains(string(b.id), "live-v9-realm-profile") {
		return b.driver.realmCanaryErr
	}
	return nil
}

func (b *fusionBox) PutSentinel(_ context.Context, value string) error {
	b.state = value
	return nil
}

func (b *fusionBox) ReadSentinel(context.Context) (string, error) { return b.state, nil }

func (b *fusionBox) Snapshot(context.Context) (substrate.Snapshot, error) {
	b.snapshotState = b.state
	return substrate.Snapshot("snapshot://v10"), nil
}

func (b *fusionBox) Rollback(context.Context, substrate.Snapshot) error {
	if !b.driver.substrateRollbackNoRestore {
		b.state = b.snapshotState
	}
	return nil
}

func (b *fusionBox) DestroyObserved(context.Context) error { return b.driver.cleanupErr }

func (b *fusionBox) ProbeEgress(context.Context, live.Target) (live.EgressObservation, error) {
	if strings.Contains(string(b.id), "live-v9-realm-profile") {
		return live.EgressObservation{Reached: b.driver.realmEgressReachable, Marker: "guest-probe-exercised"}, nil
	}
	return live.EgressObservation{}, live.ErrProbeUnsupported
}

func (b *fusionBox) ApplyRealmProfile(context.Context, realm.NetworkProfile) error {
	return b.driver.realmApplyErr
}

func (b *fusionBox) ProbePublicIngress(context.Context) (realmproof.IngressObservation, error) {
	return realmproof.IngressObservation{Denied: b.driver.realmIngressDenied, Marker: "external-probe"}, nil
}

func defaultFusionRunner() Runner {
	return Runner{
		Mode:    live.ModeProbe,
		Profile: realm.R0InternalOnly,
		RawPublicTarget: live.Target{
			Kind:    live.TargetHTTP,
			Address: "https://public.example.invalid/v10-probe",
		},
	}
}

func TestRunnerNilDriverIsUnavailable(t *testing.T) {
	report, err := defaultFusionRunner().Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != live.StatusUnavailable || report.Approved || report.Reason != ReasonConfigMissing {
		t.Fatalf("report=%+v", report)
	}
}

func TestRunnerRequireLiveRejectsUnavailable(t *testing.T) {
	runner := defaultFusionRunner()
	runner.Mode = live.ModeRequireLive
	report, err := runner.Run(context.Background(), nil)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err=%v report=%+v", err, report)
	}
	if report.Status != live.StatusUnavailable || report.Approved {
		t.Fatalf("report=%+v", report)
	}
}

func TestRunnerRejectsEmptyConfiguredFingerprint(t *testing.T) {
	driver := newFusionDriver()
	driver.fingerprints = []live.Fingerprint{{}}
	report, err := defaultFusionRunner().Run(context.Background(), driver)
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("err=%v report=%+v", err, report)
	}
	if report.Status != live.StatusLiveFail || report.Approved || report.Reason != ReasonFingerprintInvalid {
		t.Fatalf("report=%+v", report)
	}
}

func TestRunnerPropagatesSubstrateFailure(t *testing.T) {
	driver := newFusionDriver()
	driver.substrateRollbackNoRestore = true
	report, err := defaultFusionRunner().Run(context.Background(), driver)
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("err=%v report=%+v", err, report)
	}
	if report.Status != live.StatusLiveFail || report.Approved || report.Reason != ReasonSubstrateFailed {
		t.Fatalf("report=%+v", report)
	}
}

func TestRunnerPropagatesRealmFailure(t *testing.T) {
	driver := newFusionDriver()
	driver.realmEgressReachable = true
	report, err := defaultFusionRunner().Run(context.Background(), driver)
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("err=%v report=%+v", err, report)
	}
	if report.Status != live.StatusLiveFail || report.Approved || report.Reason != ReasonRealmFailed {
		t.Fatalf("report=%+v", report)
	}
}

func TestRunnerRejectsFingerprintMismatch(t *testing.T) {
	driver := newFusionDriver()
	driver.fingerprints = []live.Fingerprint{
		{EndpointDigest: strings.Repeat("a", 64), TemplateDigest: strings.Repeat("b", 64)},
		{EndpointDigest: strings.Repeat("c", 64), TemplateDigest: strings.Repeat("d", 64)},
	}
	report, err := defaultFusionRunner().Run(context.Background(), driver)
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("err=%v report=%+v", err, report)
	}
	if report.Status != live.StatusLiveFail || report.Approved || report.Reason != ReasonEvidenceMismatch {
		t.Fatalf("report=%+v", report)
	}
}

func TestRunnerPassesOnlyWhenBothNestedProofsPass(t *testing.T) {
	driver := newFusionDriver()
	runner := defaultFusionRunner()
	runner.Mode = live.ModeRequireLive
	report, err := runner.Run(context.Background(), driver)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != live.StatusLivePass || !report.Approved || report.Reason != ReasonNone {
		t.Fatalf("report=%+v", report)
	}
	if !report.Capabilities.GuestExecution || !report.Capabilities.SnapshotRollback || !report.Capabilities.PublicIngressDenied || !report.Capabilities.NetworkIsolation {
		t.Fatalf("capabilities=%+v", report.Capabilities)
	}
	if report.Capabilities.InternalMeshVerified {
		t.Fatal("unsupported private mesh was upgraded")
	}
	if report.SubstrateProof.Status != live.StatusLivePass || !report.SubstrateProof.Capabilities.SnapshotRollback {
		t.Fatalf("substrate proof=%+v", report.SubstrateProof)
	}
	if report.RealmProof.Status != live.StatusLivePass || !report.RealmProof.Capabilities.RawPublicDenied || !report.RealmProof.Capabilities.PublicIngressDenied {
		t.Fatalf("realm proof=%+v", report.RealmProof)
	}
	if report.EndpointDigest == "" || report.TemplateDigest == "" {
		t.Fatalf("fingerprint not locked: %+v", report)
	}
}
