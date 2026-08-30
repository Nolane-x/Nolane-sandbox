package fabric

import (
	"errors"
	"sync"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

var (
	ErrInvalidLease = errors.New("fabric: invalid lease")
	ErrStaleLease   = errors.New("fabric: stale lease")
	ErrLeaseExpired = errors.New("fabric: lease expired")
)

type Lease struct {
	RealmID             realm.ID `json:"realm_id"`
	WorldID             world.ID `json:"world_id"`
	Generation          uint64   `json:"generation"`
	ExpiresUnix         int64    `json:"expires_unix"`
	RealizationRevision uint64   `json:"realization_revision"`
}

type LeaseBook struct {
	mu     sync.RWMutex
	leases map[string]Lease
}

func NewLeaseBook() *LeaseBook { return &LeaseBook{leases: make(map[string]Lease)} }

func leaseKey(realmID realm.ID, worldID world.ID) string {
	return string(realmID) + "\x00" + string(worldID)
}

func (b *LeaseBook) Issue(realmID realm.ID, worldID world.ID, realization uint64, expiresUnix int64) (Lease, error) {
	if b == nil || realmID == "" || worldID == "" || realization == 0 || expiresUnix <= 0 {
		return Lease{}, ErrInvalidLease
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := leaseKey(realmID, worldID)
	generation := uint64(1)
	if old, ok := b.leases[key]; ok {
		generation = old.Generation + 1
	}
	l := Lease{RealmID: realmID, WorldID: worldID, Generation: generation, ExpiresUnix: expiresUnix, RealizationRevision: realization}
	b.leases[key] = l
	return l, nil
}

func (b *LeaseBook) Validate(l Lease, nowUnix int64) error {
	if b == nil || l.RealmID == "" || l.WorldID == "" || l.Generation == 0 || l.RealizationRevision == 0 {
		return ErrInvalidLease
	}
	b.mu.RLock()
	current, ok := b.leases[leaseKey(l.RealmID, l.WorldID)]
	b.mu.RUnlock()
	if !ok || current.Generation != l.Generation || current.RealizationRevision != l.RealizationRevision || current.ExpiresUnix != l.ExpiresUnix {
		return ErrStaleLease
	}
	if nowUnix > l.ExpiresUnix {
		return ErrLeaseExpired
	}
	return nil
}

func (b *LeaseBook) Current(realmID realm.ID, worldID world.ID) (Lease, bool) {
	if b == nil { return Lease{}, false }
	b.mu.RLock(); defer b.mu.RUnlock()
	l, ok := b.leases[leaseKey(realmID, worldID)]
	return l, ok
}
