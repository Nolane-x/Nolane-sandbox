package live

import (
	"context"
	"errors"
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/authority"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type fakeDriver struct {
	healthErr error
	createErr error
	boxes     []*fakeBox
}

func (d *fakeDriver) Fingerprint() Fingerprint {
	return Fingerprint{EndpointDigest: digestString("endpoint"), TemplateDigest: digestString("template")}
}
func (d *fakeDriver) Health(context.Context) error { return d.healthErr }
func (d *fakeDriver) Create(context.Context, world.ID) (Sandbox, error) {
	if d.createErr != nil {
		return nil, d.createErr
	}
	b := &fakeBox{id: "box"}
	d.boxes = append(d.boxes, b)
	return b, nil
}

type fakeBox struct {
	id                string
	state             string
	canaryErr         error
	cleanupErr        error
	rollbackNoRestore bool
}

func (b *fakeBox) Digest() string                                       { return digestString(b.id) }
func (b *fakeBox) Canary(context.Context) error                         { return b.canaryErr }
func (b *fakeBox) PutSentinel(_ context.Context, v string) error        { b.state = v; return nil }
func (b *fakeBox) ReadSentinel(context.Context) (string, error)         { return b.state, nil }
func (b *fakeBox) Snapshot(context.Context) (substrate.Snapshot, error) { return "snap", nil }
func (b *fakeBox) Rollback(context.Context, substrate.Snapshot) error {
	if !b.rollbackNoRestore {
		b.state = "A"
	}
	return nil
}
func (b *fakeBox) DestroyObserved(context.Context) error { return b.cleanupErr }
func (b *fakeBox) ProbeEgress(context.Context, Target) (EgressObservation, error) {
	return EgressObservation{}, ErrProbeUnsupported
}

func TestProbeModeMissingDriverIsUnavailableNotPass(t *testing.T) {
	r, err := Runner{Mode: ModeProbe, Profile: ProfileCore}.Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusUnavailable || r.Approved {
		t.Fatalf("report=%+v", r)
	}
	if err := VerifyReport(r); err != nil {
		t.Fatal(err)
	}
}

func TestRequireLiveMissingDriverFailsGate(t *testing.T) {
	r, err := Runner{Mode: ModeRequireLive, Profile: ProfileCore}.Run(context.Background(), nil)
	if !errors.Is(err, ErrLiveUnavailable) {
		t.Fatalf("err=%v report=%+v", err, r)
	}
	if r.Status != StatusUnavailable || r.Approved {
		t.Fatalf("report=%+v", r)
	}
}

func TestCoreLivePassRequiresGuestSnapshotAuthorityAndCleanup(t *testing.T) {
	d := &fakeDriver{}
	r, err := Runner{Mode: ModeRequireLive, Profile: ProfileCore}.Run(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusLivePass || !r.Approved {
		t.Fatalf("report=%+v", r)
	}
	if !r.Capabilities.GuestExecution || !r.Capabilities.SnapshotRollback || !r.Capabilities.CleanupObserved {
		t.Fatalf("capabilities=%+v", r.Capabilities)
	}
	if len(r.Scenarios) != 2 {
		t.Fatalf("scenarios=%v", r.Scenarios)
	}
	if err := VerifyReport(r); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupFailureIsLiveFail(t *testing.T) {
	d2 := &customDriver{create: func(context.Context, world.ID) (Sandbox, error) {
		return &fakeBox{id: "box", cleanupErr: errors.New("still alive")}, nil
	}}
	r, err := Runner{Mode: ModeRequireLive, Profile: ProfileCore}.Run(context.Background(), d2)
	if !errors.Is(err, ErrLiveFailed) {
		t.Fatalf("err=%v report=%+v", err, r)
	}
	if r.Status != StatusLiveFail || r.Approved {
		t.Fatalf("report=%+v", r)
	}
}

type customDriver struct {
	create func(context.Context, world.ID) (Sandbox, error)
}

func (d *customDriver) Fingerprint() Fingerprint {
	return Fingerprint{EndpointDigest: digestString("e"), TemplateDigest: digestString("t")}
}
func (d *customDriver) Health(context.Context) error { return nil }
func (d *customDriver) Create(ctx context.Context, id world.ID) (Sandbox, error) {
	return d.create(ctx, id)
}

var _ = authority.ErrActionCollision

func TestCreateCleanupUncertaintyIsClassifiedAsCleanupFailure(t *testing.T) {
	d := &customDriver{create: func(context.Context, world.ID) (Sandbox, error) { return nil, ErrCleanupFailed }}
	r, err := Runner{Mode: ModeRequireLive, Profile: ProfileCore}.Run(context.Background(), d)
	if !errors.Is(err, ErrLiveFailed) {
		t.Fatalf("err=%v report=%+v", err, r)
	}
	if r.Reason != ReasonCleanupFailed || r.Status != StatusLiveFail {
		t.Fatalf("report=%+v", r)
	}
}
