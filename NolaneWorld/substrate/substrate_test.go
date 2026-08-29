package substrate

import (
	"context"
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type fake struct{}

func (fake) Create(context.Context, world.ID) (Handle, error)   { return Handle("h"), nil }
func (fake) Destroy(context.Context, Handle) error              { return nil }
func (fake) Pause(context.Context, Handle) error                { return nil }
func (fake) Resume(context.Context, Handle) error               { return nil }
func (fake) Snapshot(context.Context, Handle) (Snapshot, error) { return Snapshot("s"), nil }
func (fake) Rollback(context.Context, Handle, Snapshot) error   { return nil }
func (fake) Clone(context.Context, Handle, Snapshot, world.ID) (Handle, error) {
	return Handle("clone"), nil
}

var _ SandboxSubstrate = fake{}

func TestSubstrateHandlesAreOpaqueAndIndependentOfTrustState(t *testing.T) {
	var s SandboxSubstrate = fake{}
	h, err := s.Create(context.Background(), world.ID("world-1"))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := s.Snapshot(context.Background(), h)
	if err != nil {
		t.Fatal(err)
	}
	clone, err := s.Clone(context.Background(), h, snap, world.ID("world-2"))
	if err != nil {
		t.Fatal(err)
	}
	if h == "" || snap == "" || clone == "" {
		t.Fatal("opaque handles must be non-empty in fake")
	}
}
