package fabric

import "errors"

var (
	ErrReservationNotFound      = errors.New("fabric: reservation not found")
	ErrRecoveryFenceUnsupported = errors.New("fabric: store does not support recovery fencing")
)

type recoveryFencer interface {
	FenceRealizationsForRecovery()
}

func (c *Capacity) Release(operationID string) error {
	if c == nil || operationID == "" {
		return ErrReservationNotFound
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.reservations[operationID]
	if !ok {
		return ErrReservationNotFound
	}
	if c.used.CPUUnits < r.Units.CPUUnits || c.used.MemoryMiB < r.Units.MemoryMiB || c.used.DiskMiB < r.Units.DiskMiB {
		return ErrInvalidCapacity
	}
	c.used.CPUUnits -= r.Units.CPUUnits
	c.used.MemoryMiB -= r.Units.MemoryMiB
	c.used.DiskMiB -= r.Units.DiskMiB
	delete(c.reservations, operationID)
	return nil
}

func (b *LeaseBook) Restore(l Lease) error {
	if b == nil || l.RealmID == "" || l.WorldID == "" || l.Generation == 0 || l.ExpiresUnix <= 0 || l.RealizationRevision == 0 {
		return ErrInvalidLease
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := leaseKey(l.RealmID, l.WorldID)
	if old, ok := b.leases[key]; ok && old.Generation > l.Generation {
		return ErrStaleLease
	}
	b.leases[key] = l
	return nil
}

// FenceRecoveredRealizations establishes the explicit post-replay recovery
// boundary. The durable store first replays historical truth unchanged; only
// then does the host-owned fabric invalidate stale realization handles and
// service readiness in memory so nothing is falsely treated as live.
func (f *Local) FenceRecoveredRealizations() error {
	if f == nil || f.store == nil {
		return ErrInvalidFabric
	}
	fencer, ok := f.store.(recoveryFencer)
	if !ok {
		return ErrRecoveryFenceUnsupported
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	fencer.FenceRealizationsForRecovery()
	return nil
}
