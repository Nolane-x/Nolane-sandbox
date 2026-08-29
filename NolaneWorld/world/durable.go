package world

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

var (
	ErrStateExists          = errors.New("world: durable state exists")
	ErrStateNotFound        = errors.New("world: durable state not found")
	ErrStateCorrupt         = errors.New("world: durable state corrupt")
	ErrStateLocked          = errors.New("world: durable state locked")
	ErrStateLockUnsupported = errors.New("world: durable state locking unsupported")
	ErrStateReleased        = errors.New("world: durable state released")
)

const authorityJournalVersion = 1

type authorityOperation string

const (
	authorityInit    authorityOperation = "init"
	authorityAdvance authorityOperation = "advance"
	authorityClose   authorityOperation = "close"
)

type authorityRecord struct {
	Version      int                `json:"version"`
	Sequence     uint64             `json:"sequence"`
	Operation    authorityOperation `json:"operation"`
	WorldID      ID                 `json:"world_id"`
	Epoch        Epoch              `json:"epoch"`
	PreviousHash string             `json:"previous_hash"`
	RecordHash   string             `json:"record_hash"`
}

type DurableFactory struct {
	dir string
}

func NewDurableFactory(dir string) (*DurableFactory, error) {
	if dir == "" {
		return nil, ErrInvalidWorld
	}
	dir = filepath.Clean(dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, errors.Join(ErrStateCorrupt, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, errors.Join(ErrStateCorrupt, err)
	}
	return &DurableFactory{dir: dir}, nil
}

func (f *DurableFactory) Path(id ID) string {
	if f == nil || id == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(id))
	return filepath.Join(f.dir, hex.EncodeToString(sum[:])+".authority.jsonl")
}

func (f *DurableFactory) Create(id ID) (AuthorityControl, error) {
	if f == nil || id == "" {
		return nil, ErrInvalidWorld
	}
	path := f.Path(id)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, ErrStateExists
		}
		return nil, errors.Join(ErrStateCorrupt, err)
	}
	if err := lockStateFile(file.Fd()); err != nil {
		_ = file.Close()
		return nil, err
	}
	state := &DurableState{id: id, epoch: 1, sequence: 1, file: file}
	rec := authorityRecord{Version: authorityJournalVersion, Sequence: 1, Operation: authorityInit, WorldID: id, Epoch: 1}
	rec.RecordHash = hashAuthorityRecord(rec)
	if err := writeAuthorityRecord(file, rec); err != nil {
		_ = unlockStateFile(file.Fd())
		_ = file.Close()
		return nil, err
	}
	state.lastHash = rec.RecordHash
	if err := syncDirectory(f.dir); err != nil {
		_ = state.Release()
		return nil, err
	}
	return state, nil
}

func (f *DurableFactory) Open(id ID) (AuthorityControl, error) {
	if f == nil || id == "" {
		return nil, ErrInvalidWorld
	}
	path := f.Path(id)
	file, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrStateNotFound
		}
		return nil, errors.Join(ErrStateCorrupt, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = file.Close()
		return nil, errors.Join(ErrStateCorrupt, err)
	}
	if err := lockStateFile(file.Fd()); err != nil {
		_ = file.Close()
		return nil, err
	}
	state := &DurableState{id: id, file: file}
	if err := state.recover(); err != nil {
		_ = unlockStateFile(file.Fd())
		_ = file.Close()
		return nil, err
	}
	return state, nil
}

type DurableState struct {
	id       ID
	mu       sync.RWMutex
	epoch    Epoch
	closed   bool
	released bool
	sequence uint64
	lastHash string
	file     *os.File
}

func (s *DurableState) ID() ID {
	if s == nil {
		return ""
	}
	return s.id
}

func (s *DurableState) CurrentEpoch() Epoch {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.epoch
}

func (s *DurableState) Closed() bool {
	if s == nil {
		return true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

func (s *DurableState) ValidateEpoch(epoch Epoch) error {
	if s == nil || s.id == "" {
		return ErrInvalidWorld
	}
	if epoch == 0 {
		return ErrInvalidEpoch
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.released {
		return ErrStateReleased
	}
	if s.closed {
		return ErrClosedWorld
	}
	return validateEpoch(epoch, s.epoch)
}

func (s *DurableState) WithEpoch(epoch Epoch, fn func() error) error {
	if s == nil || s.id == "" {
		return ErrInvalidWorld
	}
	if epoch == 0 || fn == nil {
		return ErrInvalidEpoch
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.released {
		return ErrStateReleased
	}
	if s.closed {
		return ErrClosedWorld
	}
	if err := validateEpoch(epoch, s.epoch); err != nil {
		return err
	}
	return fn()
}

func (s *DurableState) AdvanceAuthority() (Epoch, error) {
	if s == nil || s.id == "" {
		return 0, ErrInvalidWorld
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.released {
		return s.epoch, ErrStateReleased
	}
	if s.closed {
		return s.epoch, ErrClosedWorld
	}
	next := s.epoch + 1
	if err := s.appendLocked(authorityAdvance, next); err != nil {
		return s.epoch, err
	}
	s.epoch = next
	return s.epoch, nil
}

func (s *DurableState) CloseAuthority() (Epoch, error) {
	if s == nil || s.id == "" {
		return 0, ErrInvalidWorld
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.released {
		return s.epoch, ErrStateReleased
	}
	if s.closed {
		return s.epoch, nil
	}
	next := s.epoch + 1
	if err := s.appendLocked(authorityClose, next); err != nil {
		return s.epoch, err
	}
	s.epoch = next
	s.closed = true
	return s.epoch, nil
}

func (s *DurableState) Release() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.released {
		return nil
	}
	s.released = true
	unlockErr := unlockStateFile(s.file.Fd())
	closeErr := s.file.Close()
	return errors.Join(unlockErr, closeErr)
}

func (s *DurableState) appendLocked(op authorityOperation, epoch Epoch) error {
	rec := authorityRecord{
		Version: authorityJournalVersion, Sequence: s.sequence + 1, Operation: op,
		WorldID: s.id, Epoch: epoch, PreviousHash: s.lastHash,
	}
	rec.RecordHash = hashAuthorityRecord(rec)
	if err := writeAuthorityRecord(s.file, rec); err != nil {
		return err
	}
	s.sequence = rec.Sequence
	s.lastHash = rec.RecordHash
	return nil
}

func (s *DurableState) recover() error {
	if _, err := s.file.Seek(0, 0); err != nil {
		return errors.Join(ErrStateCorrupt, err)
	}
	scanner := bufio.NewScanner(s.file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var previous string
	var sequence uint64
	var epoch Epoch
	closed := false
	lines := 0
	for scanner.Scan() {
		lines++
		rec, err := decodeAuthorityRecord(scanner.Bytes())
		if err != nil {
			return fmt.Errorf("%w: line %d: %v", ErrStateCorrupt, lines, err)
		}
		if rec.Version != authorityJournalVersion || rec.WorldID != s.id {
			return fmt.Errorf("%w: line %d: version/world mismatch", ErrStateCorrupt, lines)
		}
		if rec.Sequence != sequence+1 || rec.PreviousHash != previous || rec.RecordHash != hashAuthorityRecord(rec) {
			return fmt.Errorf("%w: line %d: hash/sequence mismatch", ErrStateCorrupt, lines)
		}
		switch rec.Operation {
		case authorityInit:
			if sequence != 0 || rec.Sequence != 1 || rec.Epoch != 1 || rec.PreviousHash != "" {
				return fmt.Errorf("%w: invalid init", ErrStateCorrupt)
			}
			epoch = 1
		case authorityAdvance:
			if sequence == 0 || closed || rec.Epoch != epoch+1 {
				return fmt.Errorf("%w: invalid advance", ErrStateCorrupt)
			}
			epoch = rec.Epoch
		case authorityClose:
			if sequence == 0 || closed || rec.Epoch != epoch+1 {
				return fmt.Errorf("%w: invalid close", ErrStateCorrupt)
			}
			epoch = rec.Epoch
			closed = true
		default:
			return fmt.Errorf("%w: unknown operation", ErrStateCorrupt)
		}
		sequence = rec.Sequence
		previous = rec.RecordHash
	}
	if err := scanner.Err(); err != nil {
		return errors.Join(ErrStateCorrupt, err)
	}
	if lines == 0 || sequence == 0 {
		return ErrStateCorrupt
	}
	if _, err := s.file.Seek(0, io.SeekEnd); err != nil {
		return errors.Join(ErrStateCorrupt, err)
	}
	s.sequence = sequence
	s.lastHash = previous
	s.epoch = epoch
	s.closed = closed
	return nil
}

func decodeAuthorityRecord(raw []byte) (authorityRecord, error) {
	var rec authorityRecord
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rec); err != nil {
		return rec, err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return rec, errors.New("multiple JSON values")
		}
		return rec, err
	}
	return rec, nil
}

func writeAuthorityRecord(file *os.File, rec authorityRecord) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return errors.Join(ErrStateCorrupt, err)
	}
	raw = append(raw, '\n')
	if _, err := file.Write(raw); err != nil {
		return errors.Join(ErrStateCorrupt, err)
	}
	if err := file.Sync(); err != nil {
		return errors.Join(ErrStateCorrupt, err)
	}
	return nil
}

func hashAuthorityRecord(rec authorityRecord) string {
	h := sha256.New()
	write := func(b []byte) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(b)))
		_, _ = h.Write(n[:])
		_, _ = h.Write(b)
	}
	write([]byte("nolane.authority-state.v1"))
	var version [8]byte
	binary.BigEndian.PutUint64(version[:], uint64(rec.Version))
	write(version[:])
	var seq [8]byte
	binary.BigEndian.PutUint64(seq[:], rec.Sequence)
	write(seq[:])
	write([]byte(rec.Operation))
	write([]byte(rec.WorldID))
	var epoch [8]byte
	binary.BigEndian.PutUint64(epoch[:], uint64(rec.Epoch))
	write(epoch[:])
	write([]byte(rec.PreviousHash))
	return hex.EncodeToString(h.Sum(nil))
}

func syncDirectory(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return errors.Join(ErrStateCorrupt, err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return errors.Join(ErrStateCorrupt, err)
	}
	return nil
}
