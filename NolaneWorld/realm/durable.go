package realm

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

const durableVersion = 1

type eventKind string

const (
	eventRealmCreate    eventKind = "realm-create"
	eventRealmUpdate    eventKind = "realm-update"
	eventRealmClose     eventKind = "realm-close"
	eventWorldPut       eventKind = "world-put"
	eventCheckpointPut  eventKind = "checkpoint-put"
	eventServicePut     eventKind = "service-put"
	eventOperationPut   eventKind = "operation-put"
)

type durableEvent struct {
	Version      int               `json:"version"`
	Sequence     uint64            `json:"sequence"`
	Kind         eventKind         `json:"kind"`
	Realm        *RealmRecord      `json:"realm,omitempty"`
	World        *WorldRecord      `json:"world,omitempty"`
	Checkpoint   *CheckpointRecord `json:"checkpoint,omitempty"`
	Service      *ServiceRecord    `json:"service,omitempty"`
	Operation    *OperationRecord  `json:"operation,omitempty"`
	PreviousHash string            `json:"previous_hash"`
	RecordHash   string            `json:"record_hash"`
}

type DurableStore struct {
	root        string
	journalPath string
	file        *os.File
	mu          sync.RWMutex
	closed      bool
	sequence    uint64
	lastHash    string
	realms      map[ID]RealmRecord
	worlds      map[string]WorldRecord
	checkpoints map[CheckpointID]CheckpointRecord
	services    map[ServiceID]ServiceRecord
	operations  map[string]OperationRecord
}

func OpenDurableStore(root string) (*DurableStore, error) {
	if root == "" {
		return nil, ErrStoreCorrupt
	}
	root = filepath.Clean(root)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, errors.Join(ErrStoreCorrupt, err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return nil, errors.Join(ErrStoreCorrupt, err)
	}
	journal := filepath.Join(root, "realm-state.jsonl")
	_, statErr := os.Stat(journal)
	newJournal := errors.Is(statErr, os.ErrNotExist)
	f, err := os.OpenFile(journal, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, errors.Join(ErrStoreCorrupt, err)
	}
	if err := os.Chmod(journal, 0o600); err != nil {
		_ = f.Close()
		return nil, errors.Join(ErrStoreCorrupt, err)
	}
	if err := lockStoreFile(f.Fd()); err != nil {
		_ = f.Close()
		return nil, err
	}
	s := &DurableStore{
		root: root, journalPath: journal, file: f,
		realms: make(map[ID]RealmRecord), worlds: make(map[string]WorldRecord),
		checkpoints: make(map[CheckpointID]CheckpointRecord), services: make(map[ServiceID]ServiceRecord),
		operations: make(map[string]OperationRecord),
	}
	if err := s.recover(); err != nil {
		_ = unlockStoreFile(f.Fd())
		_ = f.Close()
		return nil, err
	}
	if newJournal {
		if err := syncStoreDir(root); err != nil {
			_ = s.Close()
			return nil, err
		}
	}
	return s, nil
}

func (s *DurableStore) CreateRealm(spec Spec) (RealmRecord, error) {
	if s == nil || spec.Validate() != nil {
		return RealmRecord{}, ErrInvalidSpec
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return RealmRecord{}, ErrStoreClosed
	}
	if _, ok := s.realms[spec.ID]; ok {
		return RealmRecord{}, ErrRealmExists
	}
	rec := RealmRecord{Spec: spec, Revision: 1}
	e := durableEvent{Kind: eventRealmCreate, Realm: &rec}
	if err := s.appendLocked(e); err != nil {
		return RealmRecord{}, err
	}
	s.realms[spec.ID] = rec
	return rec, nil
}

func (s *DurableStore) UpdateRealm(id ID, expected uint64, spec Spec) (RealmRecord, error) {
	if s == nil || spec.Validate() != nil {
		return RealmRecord{}, ErrInvalidSpec
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return RealmRecord{}, ErrStoreClosed
	}
	old, ok := s.realms[id]
	if !ok {
		return RealmRecord{}, ErrRealmNotFound
	}
	if old.Closed {
		return RealmRecord{}, ErrRealmClosed
	}
	if old.Revision != expected {
		return RealmRecord{}, ErrStaleRevision
	}
	if spec.ID != id {
		return RealmRecord{}, ErrIdentityRebind
	}
	rec := RealmRecord{Spec: spec, Revision: old.Revision + 1}
	if err := s.appendLocked(durableEvent{Kind: eventRealmUpdate, Realm: &rec}); err != nil {
		return RealmRecord{}, err
	}
	s.realms[id] = rec
	return rec, nil
}

func (s *DurableStore) CloseRealm(id ID, expected uint64) error {
	if s == nil {
		return ErrRealmNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrStoreClosed
	}
	old, ok := s.realms[id]
	if !ok {
		return ErrRealmNotFound
	}
	if old.Revision != expected {
		return ErrStaleRevision
	}
	if old.Closed {
		return nil
	}
	rec := old
	rec.Revision++
	rec.Closed = true
	if err := s.appendLocked(durableEvent{Kind: eventRealmClose, Realm: &rec}); err != nil {
		return err
	}
	s.realms[id] = rec
	return nil
}

func (s *DurableStore) Realm(id ID) (RealmRecord, bool) {
	if s == nil {
		return RealmRecord{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return RealmRecord{}, false
	}
	r, ok := s.realms[id]
	return r, ok
}

func (s *DurableStore) PutWorld(rec WorldRecord) error {
	if s == nil || rec.Validate() != nil {
		return ErrInvalidWorld
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrStoreClosed
	}
	if err := s.validateWorldLocked(rec); err != nil {
		return err
	}
	if err := s.appendLocked(durableEvent{Kind: eventWorldPut, World: &rec}); err != nil {
		return err
	}
	s.worlds[worldKey(rec.RealmID, rec.WorldID)] = rec
	return nil
}

func (s *DurableStore) World(realmID ID, id world.ID) (WorldRecord, bool) {
	if s == nil || id == "" {
		return WorldRecord{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return WorldRecord{}, false
	}
	r, ok := s.worlds[worldKey(realmID, id)]
	return r, ok
}

func (s *DurableStore) PutCheckpoint(rec CheckpointRecord) error {
	if s == nil || rec.Validate() != nil {
		return ErrInvalidCheckpoint
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrStoreClosed
	}
	if _, ok := s.realms[rec.RealmID]; !ok {
		return ErrRealmNotFound
	}
	if old, ok := s.checkpoints[rec.ID]; ok {
		if old == rec {
			return nil
		}
		return ErrInvalidCheckpoint
	}
	if err := s.appendLocked(durableEvent{Kind: eventCheckpointPut, Checkpoint: &rec}); err != nil {
		return err
	}
	s.checkpoints[rec.ID] = rec
	return nil
}

func (s *DurableStore) Checkpoint(id CheckpointID) (CheckpointRecord, bool) {
	if s == nil {
		return CheckpointRecord{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return CheckpointRecord{}, false
	}
	r, ok := s.checkpoints[id]
	return r, ok
}

func (s *DurableStore) PutService(rec ServiceRecord) error {
	if s == nil || rec.ID == "" || !validRealmID(rec.RealmID) || rec.WorldID == "" || rec.RealizationRevision == 0 || rec.Generation == 0 || rec.Port == 0 {
		return ErrInvalidService
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrStoreClosed
	}
	if _, ok := s.realms[rec.RealmID]; !ok {
		return ErrRealmNotFound
	}
	if old, ok := s.services[rec.ID]; ok {
		if old.RealmID != rec.RealmID || old.WorldID != rec.WorldID || rec.Generation < old.Generation {
			return ErrInvalidService
		}
	}
	if err := s.appendLocked(durableEvent{Kind: eventServicePut, Service: &rec}); err != nil {
		return err
	}
	s.services[rec.ID] = rec
	return nil
}

func (s *DurableStore) Service(id ServiceID) (ServiceRecord, bool) {
	if s == nil {
		return ServiceRecord{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ServiceRecord{}, false
	}
	r, ok := s.services[id]
	return r, ok
}

func (s *DurableStore) RecordOperation(rec OperationRecord) error {
	if s == nil || !validRealmID(rec.RealmID) || rec.OperationID == "" || rec.RequestDigest == "" {
		return ErrInvalidOperation
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrStoreClosed
	}
	key := operationKey(rec.RealmID, rec.OperationID)
	if old, ok := s.operations[key]; ok {
		if old.RequestDigest != rec.RequestDigest {
			return ErrInvalidOperation
		}
		if old == rec {
			return nil
		}
	}
	if err := s.appendLocked(durableEvent{Kind: eventOperationPut, Operation: &rec}); err != nil {
		return err
	}
	s.operations[key] = rec
	return nil
}

func (s *DurableStore) Operation(id ID, operation string) (OperationRecord, bool) {
	if s == nil {
		return OperationRecord{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return OperationRecord{}, false
	}
	r, ok := s.operations[operationKey(id, operation)]
	return r, ok
}

func (s *DurableStore) validateWorldLocked(rec WorldRecord) error {
	r, ok := s.realms[rec.RealmID]
	if !ok {
		return ErrRealmNotFound
	}
	if r.Closed {
		return ErrRealmClosed
	}
	if old, ok := s.worlds[worldKey(rec.RealmID, rec.WorldID)]; ok {
		if rec.RealizationRevision < old.RealizationRevision || rec.LeaseGeneration < old.LeaseGeneration {
			return ErrInvalidWorld
		}
		if old.Phase == WorldTerminal && rec.Phase != WorldTerminal {
			return ErrInvalidWorld
		}
	}
	return nil
}

func (s *DurableStore) appendLocked(e durableEvent) error {
	e.Version = durableVersion
	e.Sequence = s.sequence + 1
	e.PreviousHash = s.lastHash
	e.RecordHash = hashEvent(e)
	raw, err := json.Marshal(e)
	if err != nil {
		return errors.Join(ErrStoreCorrupt, err)
	}
	raw = append(raw, '\n')
	if _, err := s.file.Write(raw); err != nil {
		return errors.Join(ErrStoreCorrupt, err)
	}
	if err := s.file.Sync(); err != nil {
		return errors.Join(ErrStoreCorrupt, err)
	}
	s.sequence = e.Sequence
	s.lastHash = e.RecordHash
	return nil
}

func (s *DurableStore) recover() error {
	if _, err := s.file.Seek(0, 0); err != nil {
		return errors.Join(ErrStoreCorrupt, err)
	}
	scanner := bufio.NewScanner(s.file)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	var seq uint64
	var prev string
	line := 0
	for scanner.Scan() {
		line++
		e, err := decodeEvent(scanner.Bytes())
		if err != nil {
			return fmt.Errorf("%w: line %d: %v", ErrStoreCorrupt, line, err)
		}
		if e.Version != durableVersion || e.Sequence != seq+1 || e.PreviousHash != prev || e.RecordHash != hashEvent(e) {
			return fmt.Errorf("%w: line %d: sequence/hash mismatch", ErrStoreCorrupt, line)
		}
		if err := s.applyRecovered(e); err != nil {
			return fmt.Errorf("%w: line %d: %v", ErrStoreCorrupt, line, err)
		}
		seq = e.Sequence
		prev = e.RecordHash
	}
	if err := scanner.Err(); err != nil {
		return errors.Join(ErrStoreCorrupt, err)
	}
	if _, err := s.file.Seek(0, io.SeekEnd); err != nil {
		return errors.Join(ErrStoreCorrupt, err)
	}
	s.sequence = seq
	s.lastHash = prev
	return nil
}

func (s *DurableStore) applyRecovered(e durableEvent) error {
	count := 0
	if e.Realm != nil { count++ }
	if e.World != nil { count++ }
	if e.Checkpoint != nil { count++ }
	if e.Service != nil { count++ }
	if e.Operation != nil { count++ }
	if count != 1 {
		return ErrStoreCorrupt
	}
	switch e.Kind {
	case eventRealmCreate:
		r := *e.Realm
		if r.Spec.Validate() != nil || r.Revision != 1 || r.Closed { return ErrStoreCorrupt }
		if _, ok := s.realms[r.Spec.ID]; ok { return ErrStoreCorrupt }
		s.realms[r.Spec.ID] = r
	case eventRealmUpdate:
		r := *e.Realm
		old, ok := s.realms[r.Spec.ID]
		if !ok || old.Closed || r.Spec.Validate() != nil || r.Closed || r.Revision != old.Revision+1 { return ErrStoreCorrupt }
		s.realms[r.Spec.ID] = r
	case eventRealmClose:
		r := *e.Realm
		old, ok := s.realms[r.Spec.ID]
		if !ok || old.Closed || !r.Closed || r.Revision != old.Revision+1 || r.Spec != old.Spec { return ErrStoreCorrupt }
		s.realms[r.Spec.ID] = r
	case eventWorldPut:
		r := *e.World
		if r.Validate() != nil || s.validateWorldLocked(r) != nil { return ErrStoreCorrupt }
		s.worlds[worldKey(r.RealmID, r.WorldID)] = r
	case eventCheckpointPut:
		r := *e.Checkpoint
		if r.Validate() != nil { return ErrStoreCorrupt }
		if _, ok := s.realms[r.RealmID]; !ok { return ErrStoreCorrupt }
		if old, ok := s.checkpoints[r.ID]; ok && old != r { return ErrStoreCorrupt }
		s.checkpoints[r.ID] = r
	case eventServicePut:
		r := *e.Service
		if r.ID == "" || !validRealmID(r.RealmID) || r.WorldID == "" || r.RealizationRevision == 0 || r.Generation == 0 || r.Port == 0 { return ErrStoreCorrupt }
		if _, ok := s.realms[r.RealmID]; !ok { return ErrStoreCorrupt }
		if old, ok := s.services[r.ID]; ok && (old.RealmID != r.RealmID || old.WorldID != r.WorldID || r.Generation < old.Generation) { return ErrStoreCorrupt }
		s.services[r.ID] = r
	case eventOperationPut:
		r := *e.Operation
		if !validRealmID(r.RealmID) || r.OperationID == "" || r.RequestDigest == "" { return ErrStoreCorrupt }
		key := operationKey(r.RealmID, r.OperationID)
		if old, ok := s.operations[key]; ok && old.RequestDigest != r.RequestDigest { return ErrStoreCorrupt }
		s.operations[key] = r
	default:
		return ErrStoreCorrupt
	}
	return nil
}

func decodeEvent(raw []byte) (durableEvent, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var e durableEvent
	if err := dec.Decode(&e); err != nil {
		return durableEvent{}, err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil { return durableEvent{}, ErrStoreCorrupt }
		return durableEvent{}, err
	}
	return e, nil
}

func hashEvent(e durableEvent) string {
	e.RecordHash = ""
	raw, _ := json.Marshal(e)
	h := sha256.New()
	_, _ = h.Write([]byte("nolane.realm-state.v1\x00"))
	_, _ = h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}

func syncStoreDir(path string) error {
	d, err := os.Open(path)
	if err != nil { return errors.Join(ErrStoreCorrupt, err) }
	defer d.Close()
	if err := d.Sync(); err != nil { return errors.Join(ErrStoreCorrupt, err) }
	return nil
}

func (s *DurableStore) Close() error {
	if s == nil { return nil }
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed { return nil }
	s.closed = true
	return errors.Join(unlockStoreFile(s.file.Fd()), s.file.Close())
}
