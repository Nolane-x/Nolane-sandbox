package fabric

import "errors"

var ErrReservationNotFound = errors.New("fabric: reservation not found")

func (c *Capacity) Release(operationID string) error {
	if c == nil || operationID == "" { return ErrReservationNotFound }
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.reservations[operationID]
	if !ok { return ErrReservationNotFound }
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
