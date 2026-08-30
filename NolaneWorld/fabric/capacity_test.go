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
	if obs.Revision != 1 { t.Fatalf("revision=%d", obs.Revision) }
	req := ReservationRequest{
		OperationID: "op-1", RealmID: realm.ID("realm://test"),
		Units: realm.ResourceBudget{CPUUnits: 2, MemoryMiB: 2048, DiskMiB: 4096},
		ExpiresUnix: time.Now().Add(time.Minute).Unix(),
	}
	first, err := c.Reserve(req)
	if err != nil { t.Fatal(err) }
	if first.ObservationRevision != obs.Revision { t.Fatalf("observation=%d", first.ObservationRevision) }

	// Capacity is saturated, but an exact semantic replay must be recognized first.
	replay, err := c.Reserve(req)
	if err != nil { t.Fatalf("exact replay rejected: %v", err) }
	if replay != first { t.Fatalf("replay=%+v first=%+v", replay, first) }

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
