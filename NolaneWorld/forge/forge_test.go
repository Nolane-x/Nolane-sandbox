package forge

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/artifact"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/capability"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type fakeWorlds struct {
	mu         sync.Mutex
	created    []world.ID
	destroyed  []substrate.Handle
	clones     int
	destroyErr error
}

func (f *fakeWorlds) Create(_ context.Context, id world.ID) (substrate.Handle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, id)
	return substrate.Handle("h-" + string(id)), nil
}
func (f *fakeWorlds) Destroy(_ context.Context, h substrate.Handle) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.destroyed = append(f.destroyed, h)
	return f.destroyErr
}
func (f *fakeWorlds) Pause(context.Context, substrate.Handle) error  { return nil }
func (f *fakeWorlds) Resume(context.Context, substrate.Handle) error { return nil }
func (f *fakeWorlds) Snapshot(context.Context, substrate.Handle) (substrate.Snapshot, error) {
	return "", nil
}
func (f *fakeWorlds) Rollback(context.Context, substrate.Handle, substrate.Snapshot) error {
	return nil
}
func (f *fakeWorlds) Clone(context.Context, substrate.Handle, substrate.Snapshot, world.ID) (substrate.Handle, error) {
	f.clones++
	return "", nil
}

type validatorFunc func(context.Context, substrate.Handle, capability.Candidate, []byte, []byte) (Evidence, error)

func (v validatorFunc) Validate(ctx context.Context, h substrate.Handle, c capability.Candidate, content, manifest []byte) (Evidence, error) {
	return v(ctx, h, c, content, manifest)
}

func TestPromoteUsesFreshWorldThenDestroysBeforeRegistryMutation(t *testing.T) {
	worlds := &fakeWorlds{}
	reg := capability.NewRegistry()
	f, err := New(worlds, validatorFunc(func(_ context.Context, h substrate.Handle, c capability.Candidate, content, manifest []byte) (Evidence, error) {
		if h == "" || c.OriginWorldID != "origin" {
			t.Fatalf("bad validation input")
		}
		if capability.Digest(content) != c.ContentDigest || capability.Digest(manifest) != c.ManifestDigest {
			t.Fatal("candidate digest mismatch")
		}
		return Evidence{Report: []byte("evidence-1")}, nil
	}), reg, artifact.Gate{MaxBytes: 1 << 20}, "host-verifier")
	if err != nil {
		t.Fatal(err)
	}
	f.newID = func(prefix string) (string, error) { return prefix + "fixed", nil }

	receipt, err := f.Promote(context.Background(), "origin", "browser", "1.0", []byte("code"), []byte("manifest"))
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Name != "browser" {
		t.Fatalf("receipt=%+v", receipt)
	}
	if worlds.clones != 0 {
		t.Fatal("validator must never clone origin world")
	}
	if len(worlds.created) != 1 || worlds.created[0] == "origin" {
		t.Fatalf("fresh validator world not created: %v", worlds.created)
	}
	if len(worlds.destroyed) != 1 {
		t.Fatalf("validator teardown=%v", worlds.destroyed)
	}
	if _, ok := reg.Get("browser", "1.0"); !ok {
		t.Fatal("capability not promoted")
	}
}

func TestValidationFailureDestroysWorldAndDoesNotPromote(t *testing.T) {
	worlds := &fakeWorlds{}
	reg := capability.NewRegistry()
	f, _ := New(worlds, validatorFunc(func(context.Context, substrate.Handle, capability.Candidate, []byte, []byte) (Evidence, error) {
		return Evidence{}, errors.New("bad")
	}), reg, artifact.Gate{MaxBytes: 1024}, "host-verifier")
	f.newID = func(prefix string) (string, error) { return prefix + "fixed", nil }
	if _, err := f.Promote(context.Background(), "origin", "x", "1", []byte("code"), []byte("manifest")); !errors.Is(err, ErrValidationFailed) {
		t.Fatalf("error=%v", err)
	}
	if len(worlds.destroyed) != 1 {
		t.Fatal("validator world leaked")
	}
	if _, ok := reg.Get("x", "1"); ok {
		t.Fatal("failed candidate promoted")
	}
}

func TestEmptyEvidenceDeniesPromotion(t *testing.T) {
	worlds := &fakeWorlds{}
	reg := capability.NewRegistry()
	f, _ := New(worlds, validatorFunc(func(context.Context, substrate.Handle, capability.Candidate, []byte, []byte) (Evidence, error) {
		return Evidence{}, nil
	}), reg, artifact.Gate{MaxBytes: 1024}, "host-verifier")
	f.newID = func(prefix string) (string, error) { return prefix + "fixed", nil }
	if _, err := f.Promote(context.Background(), "origin", "x", "1", []byte("code"), []byte("manifest")); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("error=%v", err)
	}
	if _, ok := reg.Get("x", "1"); ok {
		t.Fatal("candidate promoted without evidence")
	}
}

func TestTeardownFailureBlocksPromotion(t *testing.T) {
	worlds := &fakeWorlds{destroyErr: errors.New("teardown failed")}
	reg := capability.NewRegistry()
	f, _ := New(worlds, validatorFunc(func(context.Context, substrate.Handle, capability.Candidate, []byte, []byte) (Evidence, error) {
		return Evidence{Report: []byte("ok")}, nil
	}), reg, artifact.Gate{MaxBytes: 1024}, "host-verifier")
	f.newID = func(prefix string) (string, error) { return prefix + "fixed", nil }
	if _, err := f.Promote(context.Background(), "origin", "x", "1", []byte("code"), []byte("manifest")); !errors.Is(err, ErrTeardownFailed) {
		t.Fatalf("error=%v", err)
	}
	if _, ok := reg.Get("x", "1"); ok {
		t.Fatal("candidate promoted before clean validator teardown")
	}
}

func TestInvalidArtifactNeverStartsValidatorWorld(t *testing.T) {
	worlds := &fakeWorlds{}
	reg := capability.NewRegistry()
	f, _ := New(worlds, validatorFunc(func(context.Context, substrate.Handle, capability.Candidate, []byte, []byte) (Evidence, error) {
		return Evidence{Report: []byte("ok")}, nil
	}), reg, artifact.Gate{MaxBytes: 1024}, "host-verifier")
	if _, err := f.Promote(context.Background(), "origin", "x", "1", nil, []byte("manifest")); err == nil {
		t.Fatal("expected invalid artifact")
	}
	if len(worlds.created) != 0 {
		t.Fatal("validator started for invalid candidate bytes")
	}
}

func TestValidatorPanicStillTearsDownAndNeverPromotes(t *testing.T) {
	worlds := &fakeWorlds{}
	reg := capability.NewRegistry()
	f, _ := New(worlds, validatorFunc(func(context.Context, substrate.Handle, capability.Candidate, []byte, []byte) (Evidence, error) {
		panic("validator crash")
	}), reg, artifact.Gate{MaxBytes: 1024}, "host-verifier")
	f.newID = func(prefix string) (string, error) { return prefix + "fixed", nil }
	if _, err := f.Promote(context.Background(), "origin", "x", "panic", []byte("code"), []byte("manifest")); !errors.Is(err, ErrValidationFailed) {
		t.Fatalf("panic error=%v want ErrValidationFailed", err)
	}
	if len(worlds.destroyed) != 1 {
		t.Fatalf("validator world leaked after panic: %v", worlds.destroyed)
	}
	if _, ok := reg.Get("x", "panic"); ok {
		t.Fatal("panic candidate promoted")
	}
}

func TestForgePersistsExactValidationEvidenceInDurableRegistry(t *testing.T) {
	worlds := &fakeWorlds{}
	root := t.TempDir()
	reg, err := capability.OpenDurableRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	f, err := New(worlds, validatorFunc(func(context.Context, substrate.Handle, capability.Candidate, []byte, []byte) (Evidence, error) {
		return Evidence{Report: []byte("signed-test-report")}, nil
	}), reg, artifact.Gate{MaxBytes: 1024}, "host-verifier")
	if err != nil {
		t.Fatal(err)
	}
	f.newID = func(prefix string) (string, error) { return prefix + "durable", nil }
	if _, err := f.Promote(context.Background(), "origin", "durable-tool", "1", []byte("code"), []byte("manifest")); err != nil {
		t.Fatal(err)
	}
	if len(worlds.destroyed) != 1 {
		t.Fatal("validator must be destroyed before durable promotion returns")
	}
	if err := reg.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := capability.OpenDurableRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	material, err := reopened.Load("durable-tool", "1")
	if err != nil {
		t.Fatal(err)
	}
	if string(material.VerificationEvidence) != "signed-test-report" {
		t.Fatalf("evidence=%q", material.VerificationEvidence)
	}
}
