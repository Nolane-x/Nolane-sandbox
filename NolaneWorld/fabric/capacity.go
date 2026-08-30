package fabric

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

var (
	ErrInvalidCapacity      = errors.New("fabric: invalid capacity request")
	ErrCapacityExhausted    = errors.New("fabric: capacity exhausted")
	ErrRealmBudgetExceeded  = errors.New("fabric: realm resource budget exceeded")
	ErrOperationCollision   = errors.New("fabric: operation collision")
	ErrReservationAmbiguous = errors.New("fabric: reservation operation ID is ambiguous across Realms")
)

type Observation struct {
	Revision     uint64               `json:"revision"`
	Capacity     realm.ResourceBudget `json:"capacity"`
	ObservedUnix int64                `json:"observed_unix"`
}

type ReservationRequest struct {
	OperationID string
	RealmID     realm.ID
	Units       realm.ResourceBudget
	ExpiresUnix int64
}

type Reservation struct {
	ID                  string               `json:"id"`
	RealmID             realm.ID             `json:"realm_id"`
	OperationID         string               `json:"operation_id"`
	RequestDigest       string               `json:"request_digest"`
	ObservationRevision uint64               `json:"observation_revision"`
	Units               realm.ResourceBudget `json:"units"`
	ExpiresUnix         int64                `json:"expires_unix"`
	EnforcementProven   bool                 `json:"enforcement_proven"`
}

type Capacity struct {
	mu           sync.Mutex
	observation  Observation
	reservations map[string]Reservation
	used         realm.ResourceBudget
	usedByRealm  map[realm.ID]realm.ResourceBudget
}

func NewCapacity() *Capacity {
	return &Capacity{
		reservations: make(map[string]Reservation),
		usedByRealm:  make(map[realm.ID]realm.ResourceBudget),
	}
}

func (c *Capacity) Observe(capacity realm.ResourceBudget) Observation {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.observation.Revision++
	c.observation.Capacity = capacity
	return c.observation
}

// Reservation is retained as a compatibility lookup. It succeeds only when an
// operation ID is unique across all Realms; callers that know Realm identity
// should use ReservationForRealm so cross-Realm names can never alias.
func (c *Capacity) Reservation(operationID string) (Reservation, bool) {
	if c == nil || operationID == "" {
		return Reservation{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var found Reservation
	matches := 0
	for _, r := range c.reservations {
		if r.OperationID != operationID {
			continue
		}
		found = r
		matches++
		if matches > 1 {
			return Reservation{}, false
		}
	}
	return found, matches == 1
}

func (c *Capacity) ReservationForRealm(realmID realm.ID, operationID string) (Reservation, bool) {
	if c == nil || realmID == "" || operationID == "" {
		return Reservation{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.reservations[reservationKey(realmID, operationID)]
	return r, ok
}

func (c *Capacity) Reserve(req ReservationRequest) (Reservation, error) {
	return c.reserve(req, nil)
}

// ReserveWithin applies both host-capacity accounting and the owning Realm's
// admission budget. The Realm budget is accounting policy, not proof of kernel
// enforcement; EnforcementProven therefore remains false on the reservation.
func (c *Capacity) ReserveWithin(req ReservationRequest, realmLimit realm.ResourceBudget) (Reservation, error) {
	if !realmLimit.Valid() {
		return Reservation{}, ErrInvalidCapacity
	}
	return c.reserve(req, &realmLimit)
}

func (c *Capacity) reserve(req ReservationRequest, realmLimit *realm.ResourceBudget) (Reservation, error) {
	if c == nil || req.OperationID == "" || req.RealmID == "" || !req.Units.Valid() || req.ExpiresUnix <= 0 {
		return Reservation{}, ErrInvalidCapacity
	}
	digest := digestReservation(req)
	key := reservationKey(req.RealmID, req.OperationID)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.reservations == nil {
		c.reservations = make(map[string]Reservation)
	}
	if c.usedByRealm == nil {
		c.usedByRealm = make(map[realm.ID]realm.ResourceBudget)
	}

	// Replay-before-admission is deliberate. An exact retry returns the
	// original reservation even if later observations or budgets are tighter.
	if prior, ok := c.reservations[key]; ok {
		if prior.RequestDigest != digest {
			return Reservation{}, ErrOperationCollision
		}
		return prior, nil
	}
	if c.observation.Revision == 0 || !c.observation.Capacity.Valid() {
		return Reservation{}, ErrInvalidCapacity
	}
	if !fits(c.used, req.Units, c.observation.Capacity) {
		return Reservation{}, ErrCapacityExhausted
	}
	if realmLimit != nil && !fits(c.usedByRealm[req.RealmID], req.Units, *realmLimit) {
		return Reservation{}, ErrRealmBudgetExceeded
	}

	idHash := sha256.Sum256([]byte("nolane.fabric.reservation.v1\x00" + string(req.RealmID) + "\x00" + req.OperationID + "\x00" + digest))
	r := Reservation{
		ID: hex.EncodeToString(idHash[:]), RealmID: req.RealmID, OperationID: req.OperationID,
		RequestDigest: digest, ObservationRevision: c.observation.Revision, Units: req.Units,
		ExpiresUnix: req.ExpiresUnix, EnforcementProven: false,
	}
	c.used = addBudget(c.used, req.Units)
	c.usedByRealm[req.RealmID] = addBudget(c.usedByRealm[req.RealmID], req.Units)
	c.reservations[key] = r
	return r, nil
}

func reservationKey(realmID realm.ID, operationID string) string {
	return string(realmID) + "\x00" + operationID
}

func digestReservation(req ReservationRequest) string {
	type canonical struct {
		OperationID string               `json:"operation_id"`
		RealmID     realm.ID             `json:"realm_id"`
		Units       realm.ResourceBudget `json:"units"`
		ExpiresUnix int64                `json:"expires_unix"`
	}
	raw, _ := json.Marshal(canonical(req))
	h := sha256.Sum256(append([]byte("nolane.fabric.reserve.v1\x00"), raw...))
	return hex.EncodeToString(h[:])
}

func fits(used, add, cap realm.ResourceBudget) bool {
	if used.CPUUnits > cap.CPUUnits || used.MemoryMiB > cap.MemoryMiB || used.DiskMiB > cap.DiskMiB {
		return false
	}
	return add.CPUUnits <= cap.CPUUnits-used.CPUUnits && add.MemoryMiB <= cap.MemoryMiB-used.MemoryMiB && add.DiskMiB <= cap.DiskMiB-used.DiskMiB
}

func addBudget(a, b realm.ResourceBudget) realm.ResourceBudget {
	return realm.ResourceBudget{
		CPUUnits:  a.CPUUnits + b.CPUUnits,
		MemoryMiB: a.MemoryMiB + b.MemoryMiB,
		DiskMiB:   a.DiskMiB + b.DiskMiB,
	}
}
