package fabric

import (
	"errors"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

func TestReservationReplayBeforeFreshAdmission(t *testing.T) {
	c := NewCapacity()
	obs := c.Observe(realm.ResourceBudget{CPUUnits: 2, MemoryMiB: 2048, DiskMiB: 4096})
	if obs.Revision != 1 {
		t.Fatalf("revision=%d", obs.Revision)
	}
	req := ReservationRequest{
		OperationID: "op-1", RealmID: realm.ID("realm://test"),
		Units:       realm.ResourceBudget{CPUUnits: 2, MemoryMiB: 2048, DiskMiB: 4096},
		ExpiresUnix: time.Now().Add(time.Minute).Unix(),
	}
	first, err := c.Reserve(req)
	if err != nil {
		t.Fatal(err)
	}
	if first.ObservationRevision != obs.Revision {
		t.Fatalf("observation=%d", first.ObservationRevision)
	}

	// Capacity is saturated, but an exact semantic replay must be recognized first.
	replay, err := c.Reserve(req)
	if err != nil {
		t.Fatalf("exact replay rejected: %v", err)
	}
	if replay != first {
		t.Fatalf("replay=%+v first=%+v", replay, first)
	}

	changed := req
	changed.Units.MemoryMiB--
	if _, err := c.Reserve(changed); !errors.Is(err, ErrOperationCollision) {
		t.Fatalf("changed-payload replay err=%v", err)
	}

	fresh := req
	fresh.OperationID = "op-2"
	if _, err := c.Reserve(fresh); !errors.Is(err, ErrCapacityExhausted) {
		t.Fatalf("fresh saturated admission err=%v", err)
	}
}

func TestReserveWithinFencesPerRealmBudgetAndReleaseReclaimsIt(t *testing.T) {
	c := NewCapacity()
	c.Observe(realm.ResourceBudget{CPUUnits: 8, MemoryMiB: 8192, DiskMiB: 16384})
	limit := realm.ResourceBudget{CPUUnits: 2, MemoryMiB: 1024, DiskMiB: 2048}
	units := realm.ResourceBudget{CPUUnits: 1, MemoryMiB: 768, DiskMiB: 1024}
	expires := time.Now().Add(time.Minute).Unix()
	firstReq := ReservationRequest{OperationID: "realm-a-1", RealmID: realm.ID("realm://a"), Units: units, ExpiresUnix: expires}
	first, err := c.ReserveWithin(firstReq, limit)
	if err != nil {
		t.Fatal(err)
	}
	if first.EnforcementProven {
		t.Fatal("accounting admission was mislabeled as kernel enforcement")
	}
	if replay, err := c.ReserveWithin(firstReq, limit); err != nil || replay != first {
		t.Fatalf("exact Realm-budget replay failed: replay=%+v err=%v", replay, err)
	}
	secondReq := ReservationRequest{OperationID: "realm-a-2", RealmID: realm.ID("realm://a"), Units: units, ExpiresUnix: expires}
	if _, err := c.ReserveWithin(secondReq, limit); !errors.Is(err, ErrRealmBudgetExceeded) {
		t.Fatalf("same-Realm over-budget admission err=%v", err)
	}
	otherReq := ReservationRequest{OperationID: "realm-b-1", RealmID: realm.ID("realm://b"), Units: units, ExpiresUnix: expires}
	if _, err := c.ReserveWithin(otherReq, limit); err != nil {
		t.Fatalf("one Realm's accounting leaked into another: %v", err)
	}
	if err := c.Release(firstReq.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ReserveWithin(secondReq, limit); err != nil {
		t.Fatalf("released Realm budget was not reclaimed: %v", err)
	}
}

func TestReservationIdentityIsScopedByRealm(t *testing.T) {
	c := NewCapacity()
	c.Observe(realm.ResourceBudget{CPUUnits: 8, MemoryMiB: 8192, DiskMiB: 16384})
	limit := realm.ResourceBudget{CPUUnits: 4, MemoryMiB: 4096, DiskMiB: 8192}
	units := realm.ResourceBudget{CPUUnits: 1, MemoryMiB: 512, DiskMiB: 1024}
	expires := time.Now().Add(time.Minute).Unix()
	a := ReservationRequest{OperationID: "shared-op", RealmID: realm.ID("realm://a"), Units: units, ExpiresUnix: expires}
	b := ReservationRequest{OperationID: "shared-op", RealmID: realm.ID("realm://b"), Units: units, ExpiresUnix: expires}
	first, err := c.ReserveWithin(a, limit)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.ReserveWithin(b, limit)
	if err != nil {
		t.Fatalf("independent Realm operation IDs collided: %v", err)
	}
	if first.ID == second.ID {
		t.Fatal("cross-Realm reservations collapsed to one identity")
	}
	if _, ok := c.Reservation("shared-op"); ok {
		t.Fatal("ambiguous compatibility lookup returned a cross-Realm reservation")
	}
	if err := c.Release("shared-op"); !errors.Is(err, ErrReservationAmbiguous) {
		t.Fatalf("ambiguous compatibility release err=%v", err)
	}
	if err := c.ReleaseForRealm(a.RealmID, a.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.ReservationForRealm(b.RealmID, b.OperationID); !ok {
		t.Fatal("releasing Realm A disturbed Realm B reservation")
	}
}
