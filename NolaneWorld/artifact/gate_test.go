package artifact

import (
	"errors"
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func TestArtifactGateAcceptsRelativeContentAndBindsExactBytes(t *testing.T) {
	g := Gate{MaxBytes: 1024}
	a, err := g.Accept(world.ID("world-1"), "reports/result.json", "application/json", []byte("one"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := g.Accept(world.ID("world-1"), "reports/result.json", "application/json", []byte("two"))
	if err != nil {
		t.Fatal(err)
	}
	if a.ContentDigest == b.ContentDigest {
		t.Fatal("different content produced same digest")
	}
	if a.WorldID != "world-1" || a.LogicalName != "reports/result.json" || a.Size != 3 {
		t.Fatalf("bad receipt: %#v", a)
	}
}

func TestArtifactGateRejectsUnsafeNamesAndEmptyContent(t *testing.T) {
	g := Gate{MaxBytes: 1024}
	for _, name := range []string{"/etc/passwd", "../host", "a/../host", "C:\\host\\x", "bad\x00name", ""} {
		if _, err := g.Accept(world.ID("world-1"), name, "text/plain", []byte("x")); !errors.Is(err, ErrInvalidArtifact) {
			t.Fatalf("name %q: %v", name, err)
		}
	}
	if _, err := g.Accept(world.ID("world-1"), "x", "text/plain", nil); !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("empty: %v", err)
	}
}

func TestArtifactGateRejectsInvalidWorldAndOversize(t *testing.T) {
	g := Gate{MaxBytes: 3}
	if _, err := g.Accept("", "x", "text/plain", []byte("x")); !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("world: %v", err)
	}
	if _, err := g.Accept(world.ID("world-1"), "x", "text/plain", []byte("1234")); !errors.Is(err, ErrArtifactTooLarge) {
		t.Fatalf("size: %v", err)
	}
}

func TestArtifactReceiptDigestBindsWorldNameMediaTypeSizeAndContent(t *testing.T) {
	g := Gate{MaxBytes: 1024}
	base, err := g.Accept(world.ID("world-1"), "reports/result.json", "application/json", []byte("one"))
	if err != nil {
		t.Fatal(err)
	}
	otherWorld, _ := g.Accept(world.ID("world-2"), "reports/result.json", "application/json", []byte("one"))
	otherName, _ := g.Accept(world.ID("world-1"), "reports/other.json", "application/json", []byte("one"))
	otherType, _ := g.Accept(world.ID("world-1"), "reports/result.json", "text/plain", []byte("one"))
	if base.ReceiptDigest == "" {
		t.Fatal("receipt digest missing")
	}
	for label, got := range map[string]string{"world": otherWorld.ReceiptDigest, "name": otherName.ReceiptDigest, "media": otherType.ReceiptDigest} {
		if got == base.ReceiptDigest {
			t.Fatalf("%s was not bound into receipt digest", label)
		}
	}
}
