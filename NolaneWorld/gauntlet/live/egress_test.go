package live

import (
	"context"
	"errors"
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type egressDriver struct {
	preflightErr map[TargetKind]error
	reached      map[TargetKind]bool
}

func (d *egressDriver) Fingerprint() Fingerprint {
	return Fingerprint{EndpointDigest: digestString("e"), TemplateDigest: digestString("t")}
}
func (d *egressDriver) Health(context.Context) error                { return nil }
func (d *egressDriver) Preflight(_ context.Context, t Target) error { return d.preflightErr[t.Kind] }
func (d *egressDriver) Create(_ context.Context, _ world.ID) (Sandbox, error) {
	return &egressBox{d: d, state: ""}, nil
}

type egressBox struct {
	d     *egressDriver
	state string
}

func (b *egressBox) Digest() string                                       { return digestString("box") }
func (b *egressBox) Canary(context.Context) error                         { return nil }
func (b *egressBox) PutSentinel(_ context.Context, v string) error        { b.state = v; return nil }
func (b *egressBox) ReadSentinel(context.Context) (string, error)         { return b.state, nil }
func (b *egressBox) Snapshot(context.Context) (substrate.Snapshot, error) { return "snap", nil }
func (b *egressBox) Rollback(context.Context, substrate.Snapshot) error   { b.state = "A"; return nil }
func (b *egressBox) DestroyObserved(context.Context) error                { return nil }
func (b *egressBox) ProbeEgress(_ context.Context, t Target) (EgressObservation, error) {
	return EgressObservation{Reached: b.d.reached[t.Kind], Marker: "guest-probe-exercised"}, nil
}

func allTargets() []Target {
	return []Target{{Kind: TargetHTTP, Address: "https://target.test"}, {Kind: TargetTCP, Address: "target.test:443"}, {Kind: TargetUDP, Address: "target.test:9000", Expect: "echo"}, {Kind: TargetDNS, Address: "target.test", Expect: "203.0.113.10"}}
}

func TestFullEgressMissingTargetsIsUnavailableNeverPass(t *testing.T) {
	r, err := Runner{Mode: ModeRequireLive, Profile: ProfileFullEgress}.Run(context.Background(), &egressDriver{})
	if !errors.Is(err, ErrLiveUnavailable) {
		t.Fatalf("err=%v report=%+v", err, r)
	}
	if r.Status != StatusUnavailable || r.Approved {
		t.Fatalf("report=%+v", r)
	}
}
func TestFullEgressHostPreflightFailureIsUnavailable(t *testing.T) {
	d := &egressDriver{preflightErr: map[TargetKind]error{TargetTCP: errors.New("dead")}, reached: map[TargetKind]bool{}}
	r, err := Runner{Mode: ModeRequireLive, Profile: ProfileFullEgress, Targets: allTargets()}.Run(context.Background(), d)
	if !errors.Is(err, ErrLiveUnavailable) || r.Reason != ReasonTargetPreflight {
		t.Fatalf("err=%v report=%+v", err, r)
	}
}
func TestFullEgressReachableFromGuestIsLiveFail(t *testing.T) {
	d := &egressDriver{preflightErr: map[TargetKind]error{}, reached: map[TargetKind]bool{TargetTCP: true}}
	r, err := Runner{Mode: ModeRequireLive, Profile: ProfileFullEgress, Targets: allTargets()}.Run(context.Background(), d)
	if !errors.Is(err, ErrLiveFailed) || r.Reason != ReasonEgressViolation {
		t.Fatalf("err=%v report=%+v", err, r)
	}
}
func TestFullEgressAllPreflightedTargetsDeniedIsLivePass(t *testing.T) {
	d := &egressDriver{preflightErr: map[TargetKind]error{}, reached: map[TargetKind]bool{}}
	r, err := Runner{Mode: ModeRequireLive, Profile: ProfileFullEgress, Targets: allTargets()}.Run(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusLivePass || !r.Approved {
		t.Fatalf("report=%+v", r)
	}
	if !r.Capabilities.EgressHTTP || !r.Capabilities.EgressTCP || !r.Capabilities.EgressUDP || !r.Capabilities.EgressDNS {
		t.Fatalf("caps=%+v", r.Capabilities)
	}
	if err := VerifyReport(r); err != nil {
		t.Fatal(err)
	}
}

func TestTargetEvidenceDigestDoesNotDependOnExpectationSecret(t *testing.T) {
	d := &egressDriver{preflightErr: map[TargetKind]error{}, reached: map[TargetKind]bool{}}
	base := []Target{{Kind: TargetHTTP, Address: "https://target.test", Expect: "secret-one"}, {Kind: TargetTCP, Address: "target.test:443"}, {Kind: TargetUDP, Address: "target.test:9000"}, {Kind: TargetDNS, Address: "target.test"}}
	changed := append([]Target(nil), base...)
	changed[0].Expect = "different-secret"
	a, _ := runEgressScenarios(context.Background(), d, base)
	b, _ := runEgressScenarios(context.Background(), d, changed)
	if len(a) != len(b) || a[0].Digest != b[0].Digest {
		t.Fatalf("expectation secret affected evidence: a=%s b=%s", a[0].Digest, b[0].Digest)
	}
	if a[0].Markers[0] != b[0].Markers[0] {
		t.Fatalf("expectation secret affected target marker: %q != %q", a[0].Markers[0], b[0].Markers[0])
	}
}
