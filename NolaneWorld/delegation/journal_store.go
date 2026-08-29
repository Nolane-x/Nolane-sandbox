package delegation

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const journalVersion = 1

type grantJournalRecord struct {
	Version      int    `json:"version"`
	Sequence     uint64 `json:"sequence"`
	PreviousHash string `json:"previous_hash"`
	Kind         string `json:"kind"`
	Grant        *Grant `json:"grant,omitempty"`
	DelegationID ID     `json:"delegation_id,omitempty"`
	Hash         string `json:"hash"`
}

type JournalStore struct {
	mu       sync.Mutex
	file     *os.File
	states   map[ID]GrantState
	sequence uint64
	lastHash string
	closed   bool
}

func OpenJournalStore(path string) (*JournalStore, error) {
	if path == "" {
		return nil, ErrInvalidGrant
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, errors.Join(ErrStoreCorrupt, err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, errors.Join(ErrStoreCorrupt, err)
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return nil, errors.Join(ErrStoreCorrupt, err)
	}
	if err := lockGrantJournal(f.Fd()); err != nil {
		_ = f.Close()
		return nil, err
	}
	s := &JournalStore{file: f, states: make(map[ID]GrantState)}
	if err := s.recover(); err != nil {
		_ = unlockGrantJournal(f.Fd())
		_ = f.Close()
		return nil, err
	}
	return s, nil
}

func (s *JournalStore) Issue(in Grant) error {
	if s == nil {
		return ErrInvalidGrant
	}
	g, err := canonicalGrant(in)
	if err != nil {
		return err
	}
	digest, _ := GrantDigest(g)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrStoreClosed
	}
	if prior, ok := s.states[g.ID]; ok {
		priorDigest, err := GrantDigest(prior.Grant)
		if err != nil {
			return ErrStoreCorrupt
		}
		if priorDigest != digest {
			return ErrGrantCollision
		}
		return nil
	}
	rec := grantJournalRecord{Version: journalVersion, Sequence: s.sequence + 1, PreviousHash: s.lastHash, Kind: "issue", Grant: ptrGrant(g)}
	rec.Hash, err = grantRecordHash(rec)
	if err != nil {
		return err
	}
	if err := s.appendLocked(rec); err != nil {
		return err
	}
	s.states[g.ID] = GrantState{Grant: cloneGrant(g)}
	s.sequence = rec.Sequence
	s.lastHash = rec.Hash
	return nil
}

func (s *JournalStore) Revoke(id ID) error {
	if s == nil || !strict(string(id), 256) {
		return ErrInvalidGrant
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrStoreClosed
	}
	state, ok := s.states[id]
	if !ok {
		return ErrDelegationNotFound
	}
	if state.Revoked {
		return ErrAlreadyRevoked
	}
	rec := grantJournalRecord{Version: journalVersion, Sequence: s.sequence + 1, PreviousHash: s.lastHash, Kind: "revoke", DelegationID: id}
	var err error
	rec.Hash, err = grantRecordHash(rec)
	if err != nil {
		return err
	}
	if err := s.appendLocked(rec); err != nil {
		return err
	}
	state.Revoked = true
	s.states[id] = state
	s.sequence = rec.Sequence
	s.lastHash = rec.Hash
	return nil
}

func (s *JournalStore) Lookup(id ID) (GrantState, error) {
	if s == nil || !strict(string(id), 256) {
		return GrantState{}, ErrDelegationNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return GrantState{}, ErrStoreClosed
	}
	state, ok := s.states[id]
	if !ok {
		return GrantState{}, ErrDelegationNotFound
	}
	state.Grant = cloneGrant(state.Grant)
	return state, nil
}

func (s *JournalStore) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	unlockErr := unlockGrantJournal(s.file.Fd())
	closeErr := s.file.Close()
	return errors.Join(unlockErr, closeErr)
}

func (s *JournalStore) recover() error {
	if _, err := s.file.Seek(0, 0); err != nil {
		return errors.Join(ErrStoreCorrupt, err)
	}
	scanner := bufio.NewScanner(s.file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		var rec grantJournalRecord
		if err := decodeGrantJournalRecord(scanner.Bytes(), &rec); err != nil {
			return fmt.Errorf("%w: line %d", ErrStoreCorrupt, line)
		}
		if err := s.applyRecovered(rec); err != nil {
			return fmt.Errorf("%w: line %d", ErrStoreCorrupt, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return errors.Join(ErrStoreCorrupt, err)
	}
	if _, err := s.file.Seek(0, 2); err != nil {
		return errors.Join(ErrStoreCorrupt, err)
	}
	return nil
}

func decodeGrantJournalRecord(raw []byte, rec *grantJournalRecord) error {
	if len(raw) == 0 || rec == nil {
		return ErrStoreCorrupt
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(rec); err != nil {
		return ErrStoreCorrupt
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ErrStoreCorrupt
	}
	canonical, err := json.Marshal(rec)
	if err != nil || !bytes.Equal(canonical, raw) {
		return ErrStoreCorrupt
	}
	return nil
}

func (s *JournalStore) applyRecovered(rec grantJournalRecord) error {
	if rec.Version != journalVersion || rec.Sequence != s.sequence+1 || rec.PreviousHash != s.lastHash || rec.Hash == "" {
		return ErrStoreCorrupt
	}
	want, err := grantRecordHash(rec)
	if err != nil || want != rec.Hash {
		return ErrStoreCorrupt
	}
	switch rec.Kind {
	case "issue":
		if rec.Grant == nil || rec.DelegationID != "" {
			return ErrStoreCorrupt
		}
		g, err := canonicalGrant(*rec.Grant)
		if err != nil || g.ID != rec.Grant.ID {
			return ErrStoreCorrupt
		}
		if _, exists := s.states[g.ID]; exists {
			return ErrStoreCorrupt
		}
		s.states[g.ID] = GrantState{Grant: cloneGrant(g)}
	case "revoke":
		if rec.Grant != nil || !strict(string(rec.DelegationID), 256) {
			return ErrStoreCorrupt
		}
		state, exists := s.states[rec.DelegationID]
		if !exists || state.Revoked {
			return ErrStoreCorrupt
		}
		state.Revoked = true
		s.states[rec.DelegationID] = state
	default:
		return ErrStoreCorrupt
	}
	s.sequence = rec.Sequence
	s.lastHash = rec.Hash
	return nil
}

func (s *JournalStore) appendLocked(rec grantJournalRecord) error {
	raw, err := json.Marshal(rec)
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
	return nil
}

func grantRecordHash(rec grantJournalRecord) (string, error) {
	fields := [][]byte{
		u64(uint64(rec.Version)),
		u64(rec.Sequence),
		[]byte(rec.PreviousHash),
		[]byte(rec.Kind),
	}
	switch rec.Kind {
	case "issue":
		if rec.Grant == nil || rec.DelegationID != "" {
			return "", ErrStoreCorrupt
		}
		digest, err := GrantDigest(*rec.Grant)
		if err != nil {
			return "", ErrStoreCorrupt
		}
		fields = append(fields, []byte(digest))
	case "revoke":
		if rec.Grant != nil || !strict(string(rec.DelegationID), 256) {
			return "", ErrStoreCorrupt
		}
		fields = append(fields, []byte(rec.DelegationID))
	default:
		return "", ErrStoreCorrupt
	}
	return hashFields("nolane-delegation-journal-v1", fields...), nil
}

func ptrGrant(g Grant) *Grant {
	out := cloneGrant(g)
	return &out
}
