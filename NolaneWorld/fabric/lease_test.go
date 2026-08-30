package fabric

import (
	"errors"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func TestLeaseGenerationFencesStaleUse(t *testing.T) {
	book := NewLeaseBook()
	now := time.Now().Unix()
	first, err := book.Issue(realm.ID("realm://test"), world.ID("world-a"), 1, now+60)
	if err != nil { t.Fatal(err) }
	if first.Generation != 1 { t.Fatalf("generation=%d", first.Generation) }
	if err := book.Validate(first, now); err != nil { t.Fatal(err) }

	second, err := book.Issue(first.RealmID, first.WorldID, 2, now+120)
	if err != nil { t.Fatal(err) }
	if second.Generation != 2 { t.Fatalf("generation=%d", second.Generation) }
	if err := book.Validate(first, now); !errors.Is(err, ErrStaleLease) {
		t.Fatalf("stale lease err=%v", err)
	}
	if err := book.Validate(second, now+121); !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("expired lease err=%v", err)
	}
}
