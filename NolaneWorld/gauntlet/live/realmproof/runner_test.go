package realmproof

import (
	"context"
	"errors"
	"testing"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type fakeRealmBox struct {
	applyErr   error
	canaryErr  error
	egress     live.EgressObservation
	egressErr  error
	ingress    IngressObservation
	ingressErr error
	destroyErr error
}

func (b *fakeRealmBox) Digest() string { return "runtime-digest-not-exported" }
func (b *fakeRealmBox) Canary(context.Context) error { return b.canaryErr }
func (b *fakeRealmBox) PutSentinel(context.Context, string) error { return nil }
func (b *fakeRealmBox) ReadSentinel(context.Context) (string, error) { return "", nil }
func (b *fakeRealmBox) Snapshot(context.Context) (substrate.Snapshot, error) { return "", nil }
func (b *fakeRealmBox) Rollback(context.Context, substrate.Snapshot) error { return nil }
func (b *fakeRealmBox) DestroyObserved(context.Context) error { return b.destroyErr }
func (b *fakeRealmBox) ProbeEgress(context.Context, live.Target) (live.EgressObservation, error) {
	return b.egress, b.egressErr
}
func (b *fakeRealmBox) ApplyRealmProfile(context.Context, realm.NetworkProfile) error { return b.applyErr }
func (b *fakeRealmBox) ProbePublicIngress(context.Context) (IngressObservation, error) {
	return b.ingress, b.ingressErr
}

type plainBox struct{ inner *fakeRealmBox }
func (b plainBox) Digest() string { return b.inner.Digest() }
func (b plainBox) Canary(ctx context.Context) error { return b.inner.Canary(ctx) }
func (b plainBox) PutSentinel(ctx context.Context, s string) error { return b.inner.PutSentinel(ctx, s) }
func (b plainBox) ReadSentinel(ctx context.Context) (string, error) { return b.inner.ReadSentinel(ctx) }
func (b plainBox) Snapshot(ctx context.Context) (substrate.Snapshot, error) { return b.inner.Snapshot(ctx) }
func (b plainBox) Rollback(ctx context.Context, s substrate.Snapshot) error { return b.inner.Rollback(ctx, s) }
func (b plainBox) DestroyObserved(ctx context.Context) error { return b.inner.DestroyObserved(ctx) }
func (b plainBox) ProbeEgress(ctx context.Context, target live.Target) (live.EgressObservation, error) {
	return b.inner.ProbeEgress(ctx, target)
}

type fakeRealmDriver struct {
	box          live.Sandbox
	healthErr    error
	preflightErr error
	mesh         MeshObservation
	meshErr      error
	meshEnabled  bool
}

func (d *fakeRealmDriver) Fingerprint() live.Fingerprint {
	return live.Fingerprint{EndpointDigest: "endpoint-digest", TemplateDigest: "template-digest"}
}
func (d *fakeRealmDriver) Health(context.Context) error { return d.healthErr }
func (d *fakeRealmDriver) Create(context.Context, world.ID) (live.Sandbox, error) { return d.box, nil }
func (d *fakeRealmDriver) Preflight(context.Context, live.Target) error { return d.preflightErr }
func (d *fakeRealmDriver) ProbeInternalMesh(context.Context, live.Sandbox, realm.NetworkProfile) (MeshObservation, error) {
	if !d.meshEnabled {
		return MeshObservation{}, live.ErrProbeUnsupported
	}
	return d.mesh, d.meshErr
}

type fakeRealmDriverWithoutMesh struct{ base *fakeRealmDriver }
func (d fakeRealmDriverWithoutMesh) Fingerprint() live.Fingerprint { return d.base.Fingerprint() }
func (d fakeRealmDriverWithoutMesh) Health(ctx context.Context) error { return d.base.Health(ctx) }
func (d fakeRealmDriverWithoutMesh) Create(ctx context.Context, id world.ID) (live.Sandbox, error) { return d.base.Create(ctx, id) }
func (d fakeRealmDriverWithoutMesh) Preflight(ctx context.Context, target live.Target) error { return d.base.Preflight(ctx, target) }

func defaultRunner() Runner {
	return Runner{
		Mode:    live.ModeProbe,
		Profile: realm.R0InternalOnly,
		RawPublicTarget: live.Target{Kind: live.TargetHTTP, Address: "https://public.example.invalid/probe"},
	}
}

func passingBox() *fakeRealmBox {
	return &fakeRealmBox{
		egress:  live.EgressObservation{Reached: false, Marker: "guest-probe-exercised"},
		ingress: IngressObservation{Denied: true, Marker: "external-probe-denied"},
	}
}

func TestRunnerMissingDriverIsUnavailable(t *testing.T) {
	report, err := defaultRunner().Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != live.StatusUnavailable || report.Approved || report.Reason != ReasonConfigMissing {
		t.Fatalf("report=%+v", report)
	}
}

func TestRunnerWithoutRealmProfileSupportIsUnavailableAndCleansUp(t *testing.T) {
	box := passingBox()
	driver := &fakeRealmDriver{box: plainBox{inner: box}}
	report, err := defaultRunner().Run(context.Background(), driver)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != live.StatusUnavailable || report.Approved || report.Reason != ReasonDriverUnsupported {
		t.Fatalf("report=%+v", report)
	}
}

func TestRunnerFailsWhenRawPublicEgressRemainsReachable(t *testing.T) {
	box := passingBox()
	box.egress.Reached = true
	driver := fakeRealmDriverWithoutMesh{base: &fakeRealmDriver{box: box}}
	report, err := defaultRunner().Run(context.Background(), driver)
	if !errors.Is(err, ErrFailed) || report.Status != live.StatusLiveFail || report.Approved || report.Reason != ReasonRawPublicReachable {
		t.Fatalf("err=%v report=%+v", err, report)
	}
}

func TestRunnerFailsWhenUnauthenticatedPublicIngressReachesCanary(t *testing.T) {
	box := passingBox()
	box.ingress.Denied = false
	driver := fakeRealmDriverWithoutMesh{base: &fakeRealmDriver{box: box}}
	report, err := defaultRunner().Run(context.Background(), driver)
	if !errors.Is(err, ErrFailed) || report.Status != live.StatusLiveFail || report.Approved || report.Reason != ReasonIngressViolation {
		t.Fatalf("err=%v report=%+v", err, report)
	}
}

func TestRunnerPassesMandatoryProfileProofWithoutSynthesizingMesh(t *testing.T) {
	box := passingBox()
	driver := fakeRealmDriverWithoutMesh{base: &fakeRealmDriver{box: box}}
	report, err := defaultRunner().Run(context.Background(), driver)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != live.StatusLivePass || !report.Approved {
		t.Fatalf("report=%+v", report)
	}
	if !report.Capabilities.GuestExecution || !report.Capabilities.RawPublicDenied || !report.Capabilities.PublicIngressDenied {
		t.Fatalf("mandatory capability proof missing: %+v", report.Capabilities)
	}
	if report.Capabilities.InternalMeshVerified {
		t.Fatal("unsupported mesh was upgraded to verified")
	}
	mesh, ok := scenarioByID(report, ScenarioInternalMesh)
	if !ok || mesh.Outcome != live.OutcomeUnavailable || mesh.Reason != ReasonMeshUnsupported {
		t.Fatalf("mesh=%+v ok=%v", mesh, ok)
	}
}

func TestRunnerCanVerifyGenuinePrivateMeshWhenDriverObservesIt(t *testing.T) {
	box := passingBox()
	driver := &fakeRealmDriver{
		box:         box,
		meshEnabled: true,
		mesh:        MeshObservation{Reached: true, Marker: "private-route-observed"},
	}
	report, err := defaultRunner().Run(context.Background(), driver)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Approved || !report.Capabilities.InternalMeshVerified {
		t.Fatalf("report=%+v", report)
	}
	mesh, ok := scenarioByID(report, ScenarioInternalMesh)
	if !ok || mesh.Outcome != live.OutcomePass {
		t.Fatalf("mesh=%+v ok=%v", mesh, ok)
	}
}

func TestRunnerCleanupFailureCannotBecomePass(t *testing.T) {
	box := passingBox()
	box.destroyErr = errors.New("cleanup not observed")
	driver := fakeRealmDriverWithoutMesh{base: &fakeRealmDriver{box: box}}
	report, err := defaultRunner().Run(context.Background(), driver)
	if !errors.Is(err, ErrFailed) || report.Status != live.StatusLiveFail || report.Reason != ReasonCleanupFailed || report.Approved {
		t.Fatalf("err=%v report=%+v", err, report)
	}
}

func scenarioByID(report Report, id string) (ScenarioEvidence, bool) {
	for _, scenario := range report.Scenarios {
		if scenario.ID == id {
			return scenario, true
		}
	}
	return ScenarioEvidence{}, false
}
