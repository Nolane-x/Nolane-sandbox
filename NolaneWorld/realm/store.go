package realm

import (
	"sort"
	"sync"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type Store interface {
	CreateRealm(Spec) (RealmRecord, error)
	UpdateRealm(ID, uint64, Spec) (RealmRecord, error)
	CloseRealm(ID, uint64) error
	Realm(ID) (RealmRecord, bool)
	PutWorld(WorldRecord) error
	World(ID, world.ID) (WorldRecord, bool)
	Worlds(ID) []WorldRecord
	PutCheckpoint(CheckpointRecord) error
	Checkpoint(CheckpointID) (CheckpointRecord, bool)
	PutService(ServiceRecord) error
	Service(ServiceID) (ServiceRecord, bool)
	RecordOperation(OperationRecord) error
	Operation(ID, string) (OperationRecord, bool)
	Close() error
}

type MemoryStore struct {
	mu          sync.RWMutex
	closed      bool
	realms      map[ID]RealmRecord
	worlds      map[string]WorldRecord
	checkpoints map[CheckpointID]CheckpointRecord
	services    map[ServiceID]ServiceRecord
	operations  map[string]OperationRecord
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		realms: make(map[ID]RealmRecord), worlds: make(map[string]WorldRecord),
		checkpoints: make(map[CheckpointID]CheckpointRecord), services: make(map[ServiceID]ServiceRecord),
		operations: make(map[string]OperationRecord),
	}
}

func (s *MemoryStore) CreateRealm(spec Spec) (RealmRecord, error) {
	if s == nil || spec.Validate() != nil { return RealmRecord{}, ErrInvalidSpec }
	s.mu.Lock(); defer s.mu.Unlock()
	if s.closed { return RealmRecord{}, ErrStoreClosed }
	if _, ok := s.realms[spec.ID]; ok { return RealmRecord{}, ErrRealmExists }
	rec := RealmRecord{Spec: spec, Revision: 1}
	s.realms[spec.ID] = rec
	return rec, nil
}

func (s *MemoryStore) UpdateRealm(id ID, expected uint64, spec Spec) (RealmRecord, error) {
	if s == nil || spec.Validate() != nil { return RealmRecord{}, ErrInvalidSpec }
	s.mu.Lock(); defer s.mu.Unlock()
	if s.closed { return RealmRecord{}, ErrStoreClosed }
	rec, ok := s.realms[id]; if !ok { return RealmRecord{}, ErrRealmNotFound }
	if rec.Closed { return RealmRecord{}, ErrRealmClosed }
	if rec.Revision != expected { return RealmRecord{}, ErrStaleRevision }
	if spec.ID != id { return RealmRecord{}, ErrIdentityRebind }
	rec.Spec = spec; rec.Revision++
	s.realms[id] = rec
	return rec, nil
}

func (s *MemoryStore) CloseRealm(id ID, expected uint64) error {
	if s == nil { return ErrRealmNotFound }
	s.mu.Lock(); defer s.mu.Unlock()
	if s.closed { return ErrStoreClosed }
	rec, ok := s.realms[id]; if !ok { return ErrRealmNotFound }
	if rec.Revision != expected { return ErrStaleRevision }
	if rec.Closed { return nil }
	rec.Closed = true; rec.Revision++
	s.realms[id] = rec
	return nil
}

func (s *MemoryStore) Realm(id ID) (RealmRecord, bool) {
	if s == nil { return RealmRecord{}, false }
	s.mu.RLock(); defer s.mu.RUnlock()
	if s.closed { return RealmRecord{}, false }
	r, ok := s.realms[id]
	return r, ok
}

func worldKey(realmID ID, worldID world.ID) string { return string(realmID) + "\x00" + string(worldID) }
func operationKey(realmID ID, operation string) string { return string(realmID) + "\x00" + operation }

func (s *MemoryStore) PutWorld(rec WorldRecord) error {
	if s == nil || rec.Validate() != nil { return ErrInvalidWorld }
	s.mu.Lock(); defer s.mu.Unlock()
	if s.closed { return ErrStoreClosed }
	r, ok := s.realms[rec.RealmID]; if !ok { return ErrRealmNotFound }
	if r.Closed { return ErrRealmClosed }
	key := worldKey(rec.RealmID, rec.WorldID)
	if old, ok := s.worlds[key]; ok {
		if old.WorldID != rec.WorldID || old.RealmID != rec.RealmID || rec.RealizationRevision < old.RealizationRevision || rec.LeaseGeneration < old.LeaseGeneration { return ErrInvalidWorld }
		if old.Phase == WorldTerminal && rec.Phase != WorldTerminal { return ErrInvalidWorld }
	}
	s.worlds[key] = rec
	return nil
}

func (s *MemoryStore) World(realmID ID, id world.ID) (WorldRecord, bool) {
	if s == nil || id == "" { return WorldRecord{}, false }
	s.mu.RLock(); defer s.mu.RUnlock()
	if s.closed { return WorldRecord{}, false }
	r, ok := s.worlds[worldKey(realmID, id)]
	return r, ok
}

func (s *MemoryStore) Worlds(realmID ID) []WorldRecord {
	if s == nil { return nil }
	s.mu.RLock(); defer s.mu.RUnlock()
	if s.closed { return nil }
	out := make([]WorldRecord, 0)
	for _, rec := range s.worlds { if rec.RealmID == realmID { out = append(out, rec) } }
	sort.Slice(out, func(i, j int) bool { return string(out[i].WorldID) < string(out[j].WorldID) })
	return out
}

func (s *MemoryStore) PutCheckpoint(rec CheckpointRecord) error {
	if s == nil || rec.Validate() != nil { return ErrInvalidCheckpoint }
	s.mu.Lock(); defer s.mu.Unlock()
	if s.closed { return ErrStoreClosed }
	if _, ok := s.realms[rec.RealmID]; !ok { return ErrRealmNotFound }
	if old, ok := s.checkpoints[rec.ID]; ok && old != rec { return ErrInvalidCheckpoint }
	s.checkpoints[rec.ID] = rec
	return nil
}
func (s *MemoryStore) Checkpoint(id CheckpointID) (CheckpointRecord, bool) { s.mu.RLock(); defer s.mu.RUnlock(); if s.closed { return CheckpointRecord{}, false }; r, ok := s.checkpoints[id]; return r, ok }
func (s *MemoryStore) PutService(rec ServiceRecord) error { if s == nil { return ErrInvalidService }; s.mu.Lock(); defer s.mu.Unlock(); if s.closed { return ErrStoreClosed }; s.services[rec.ID] = rec; return nil }
func (s *MemoryStore) Service(id ServiceID) (ServiceRecord, bool) { if s == nil { return ServiceRecord{}, false }; s.mu.RLock(); defer s.mu.RUnlock(); if s.closed { return ServiceRecord{}, false }; r, ok := s.services[id]; return r, ok }
func (s *MemoryStore) RecordOperation(rec OperationRecord) error { if s == nil || rec.RealmID == "" || rec.OperationID == "" || rec.RequestDigest == "" { return ErrInvalidOperation }; s.mu.Lock(); defer s.mu.Unlock(); if s.closed { return ErrStoreClosed }; key := operationKey(rec.RealmID, rec.OperationID); if old, ok := s.operations[key]; ok && old.RequestDigest != rec.RequestDigest { return ErrInvalidOperation }; s.operations[key] = rec; return nil }
func (s *MemoryStore) Operation(id ID, operation string) (OperationRecord, bool) { if s == nil { return OperationRecord{}, false }; s.mu.RLock(); defer s.mu.RUnlock(); if s.closed { return OperationRecord{}, false }; r, ok := s.operations[operationKey(id, operation)]; return r, ok }
func (s *MemoryStore) Close() error { if s == nil { return nil }; s.mu.Lock(); defer s.mu.Unlock(); s.closed = true; return nil }
