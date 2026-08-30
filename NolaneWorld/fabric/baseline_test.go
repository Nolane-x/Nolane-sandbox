package fabric

import (
	"errors"
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

func TestBaselineRejectsIdentityAndPolicyBroadening(t *testing.T) {
	cat := NewBaselineCatalog()
	good := Baseline{
		ID: "base-clean", Digest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		TemplateRef: "template-clean", NetworkProfile: realm.R0InternalOnly, Sanitized: true,
	}
	if err := cat.Admit(good); err != nil { t.Fatal(err) }
	if got, ok := cat.Select(realm.R0InternalOnly); !ok || got != good { t.Fatalf("select=%+v ok=%v", got, ok) }
	if _, ok := cat.Select(realm.R1PublicRead); ok { t.Fatal("R0 baseline broadened to R1") }

	badWorld := good
	badWorld.ID = "bad-world"
	badWorld.WorldIdentity = "world://old"
	if err := cat.Admit(badWorld); !errors.Is(err, ErrInvalidBaseline) { t.Fatalf("world identity err=%v", err) }

	badCheckpoint := good
	badCheckpoint.ID = "bad-checkpoint"
	badCheckpoint.CheckpointOwner = "checkpoint://old"
	if err := cat.Admit(badCheckpoint); !errors.Is(err, ErrInvalidBaseline) { t.Fatalf("checkpoint owner err=%v", err) }

	changed := good
	changed.TemplateRef = "other-template"
	if err := cat.Admit(changed); !errors.Is(err, ErrBaselineCollision) { t.Fatalf("changed same ID err=%v", err) }
}
