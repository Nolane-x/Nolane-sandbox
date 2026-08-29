package control

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

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

var (
	ErrCatalogCorrupt         = errors.New("control: lifecycle catalog corrupt")
	ErrCatalogLocked          = errors.New("control: lifecycle catalog locked")
	ErrCatalogLockUnsupported = errors.New("control: lifecycle catalog locking unsupported")
	ErrCatalogClosed          = errors.New("control: lifecycle catalog closed")
	ErrCatalogTransition      = errors.New("control: invalid lifecycle transition")
)

const catalogJournalVersion = 1

type Phase string

const (
	PhaseCreating  Phase = "creating"
	PhaseReady     Phase = "ready"
	PhaseTerminal  Phase = "terminal"
	PhaseDestroyed Phase = "destroyed"
)

type CatalogEntry struct {
	WorldID world.ID
	Handle  substrate.Handle
	Phase   Phase
}

type catalogRecord struct {
	Version      int              `json:"version"`
	Sequence     uint64           `json:"sequence"`
	WorldID      world.ID         `json:"world_id"`
	Handle       substrate.Handle `json:"handle"`
	Phase        Phase            `json:"phase"`
	PreviousHash string           `json:"previous_hash"`
	RecordHash   string           `json:"record_hash"`
}

type LifecycleCatalog interface {
	BeginCreate(world.ID) error
	Ready(world.ID, substrate.Handle) error
	Terminal(world.ID) error
	Quarantine(world.ID, substrate.Handle) error
	Destroyed(world.ID) error
	Get(world.ID) (CatalogEntry, bool)
	Entries() map[world.ID]CatalogEntry
	Close() error
}

type JournalCatalog struct {
	path     string
	mu       sync.RWMutex
	file     *os.File
	closed   bool
	sequence uint64
	lastHash string
	entries  map[world.ID]CatalogEntry
}

func OpenJournalCatalog(path string) (*JournalCatalog, error) {
	if path == "" {
		return nil, ErrCatalogCorrupt
	}
	path = filepath.Clean(path)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, errors.Join(ErrCatalogCorrupt, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, errors.Join(ErrCatalogCorrupt, err)
	}
	_, statErr := os.Stat(path)
	newFile := errors.Is(statErr, os.ErrNotExist)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, errors.Join(ErrCatalogCorrupt, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = file.Close()
		return nil, errors.Join(ErrCatalogCorrupt, err)
	}
	if err := lockCatalogFile(file.Fd()); err != nil {
		_ = file.Close()
		return nil, err
	}
	c := &JournalCatalog{path: path, file: file, entries: make(map[world.ID]CatalogEntry)}
	if err := c.recover(); err != nil {
		_ = unlockCatalogFile(file.Fd())
		_ = file.Close()
		return nil, err
	}
	if newFile {
		if err := syncCatalogDirectory(dir); err != nil {
			_ = c.Close()
			return nil, err
		}
	}
	return c, nil
}

func (c *JournalCatalog) Path() string {
	if c == nil {
		return ""
	}
	return c.path
}

func (c *JournalCatalog) BeginCreate(id world.ID) error {
	if id == "" {
		return ErrCatalogTransition
	}
	return c.transition(id, "", PhaseCreating)
}

func (c *JournalCatalog) Ready(id world.ID, handle substrate.Handle) error {
	if id == "" || handle == "" {
		return ErrCatalogTransition
	}
	return c.transition(id, handle, PhaseReady)
}

func (c *JournalCatalog) Terminal(id world.ID) error {
	if id == "" {
		return ErrCatalogTransition
	}
	c.mu.RLock()
	current, ok := c.entries[id]
	c.mu.RUnlock()
	if !ok {
		return ErrCatalogTransition
	}
	return c.transition(id, current.Handle, PhaseTerminal)
}

func (c *JournalCatalog) Quarantine(id world.ID, handle substrate.Handle) error {
	if id == "" || handle == "" {
		return ErrCatalogTransition
	}
	c.mu.RLock()
	current, ok := c.entries[id]
	c.mu.RUnlock()
	if !ok || current.Phase != PhaseCreating {
		return ErrCatalogTransition
	}
	return c.transition(id, handle, PhaseTerminal)
}

func (c *JournalCatalog) Destroyed(id world.ID) error {
	if id == "" {
		return ErrCatalogTransition
	}
	c.mu.RLock()
	current, ok := c.entries[id]
	c.mu.RUnlock()
	if !ok || current.Handle == "" {
		return ErrCatalogTransition
	}
	return c.transition(id, current.Handle, PhaseDestroyed)
}

func (c *JournalCatalog) Get(id world.ID) (CatalogEntry, bool) {
	if c == nil || id == "" {
		return CatalogEntry{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[id]
	return entry, ok
}

func (c *JournalCatalog) Entries() map[world.ID]CatalogEntry {
	out := make(map[world.ID]CatalogEntry)
	if c == nil {
		return out
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for id, entry := range c.entries {
		out[id] = entry
	}
	return out
}

func (c *JournalCatalog) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	unlockErr := unlockCatalogFile(c.file.Fd())
	closeErr := c.file.Close()
	return errors.Join(unlockErr, closeErr)
}

func (c *JournalCatalog) transition(id world.ID, handle substrate.Handle, phase Phase) error {
	if c == nil {
		return ErrCatalogClosed
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrCatalogClosed
	}
	current, exists := c.entries[id]
	if exists && current.Phase == phase && current.Handle == handle {
		return nil
	}
	if err := validateCatalogTransition(current, exists, CatalogEntry{WorldID: id, Handle: handle, Phase: phase}); err != nil {
		return err
	}
	rec := catalogRecord{
		Version: catalogJournalVersion, Sequence: c.sequence + 1, WorldID: id,
		Handle: handle, Phase: phase, PreviousHash: c.lastHash,
	}
	rec.RecordHash = hashCatalogRecord(rec)
	if err := writeCatalogRecord(c.file, rec); err != nil {
		return err
	}
	c.sequence = rec.Sequence
	c.lastHash = rec.RecordHash
	c.entries[id] = CatalogEntry{WorldID: id, Handle: handle, Phase: phase}
	return nil
}

func validateCatalogTransition(current CatalogEntry, exists bool, next CatalogEntry) error {
	switch next.Phase {
	case PhaseCreating:
		if exists || next.Handle != "" {
			return ErrCatalogTransition
		}
	case PhaseReady:
		if !exists || current.Phase != PhaseCreating || current.Handle != "" || next.Handle == "" {
			return ErrCatalogTransition
		}
	case PhaseTerminal:
		if !exists || (current.Phase != PhaseCreating && current.Phase != PhaseReady) {
			return ErrCatalogTransition
		}
		if current.Phase == PhaseReady && next.Handle != current.Handle {
			return ErrCatalogTransition
		}
	case PhaseDestroyed:
		if !exists || current.Phase != PhaseTerminal || current.Handle == "" || next.Handle != current.Handle {
			return ErrCatalogTransition
		}
	default:
		return ErrCatalogTransition
	}
	return nil
}

func (c *JournalCatalog) recover() error {
	if _, err := c.file.Seek(0, 0); err != nil {
		return errors.Join(ErrCatalogCorrupt, err)
	}
	scanner := bufio.NewScanner(c.file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var sequence uint64
	var previous string
	line := 0
	for scanner.Scan() {
		line++
		rec, err := decodeCatalogRecord(scanner.Bytes())
		if err != nil {
			return fmt.Errorf("%w: line %d: %v", ErrCatalogCorrupt, line, err)
		}
		if rec.Version != catalogJournalVersion || rec.Sequence != sequence+1 || rec.PreviousHash != previous || rec.RecordHash != hashCatalogRecord(rec) {
			return fmt.Errorf("%w: line %d: hash/sequence mismatch", ErrCatalogCorrupt, line)
		}
		current, exists := c.entries[rec.WorldID]
		next := CatalogEntry{WorldID: rec.WorldID, Handle: rec.Handle, Phase: rec.Phase}
		if rec.WorldID == "" || validateCatalogTransition(current, exists, next) != nil {
			return fmt.Errorf("%w: line %d: illegal transition", ErrCatalogCorrupt, line)
		}
		c.entries[rec.WorldID] = next
		sequence = rec.Sequence
		previous = rec.RecordHash
	}
	if err := scanner.Err(); err != nil {
		return errors.Join(ErrCatalogCorrupt, err)
	}
	if _, err := c.file.Seek(0, io.SeekEnd); err != nil {
		return errors.Join(ErrCatalogCorrupt, err)
	}
	c.sequence = sequence
	c.lastHash = previous
	return nil
}

func decodeCatalogRecord(raw []byte) (catalogRecord, error) {
	var rec catalogRecord
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

func writeCatalogRecord(file *os.File, rec catalogRecord) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return errors.Join(ErrCatalogCorrupt, err)
	}
	raw = append(raw, '\n')
	if _, err := file.Write(raw); err != nil {
		return errors.Join(ErrCatalogCorrupt, err)
	}
	if err := file.Sync(); err != nil {
		return errors.Join(ErrCatalogCorrupt, err)
	}
	return nil
}

func hashCatalogRecord(rec catalogRecord) string {
	h := sha256.New()
	write := func(b []byte) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(b)))
		_, _ = h.Write(n[:])
		_, _ = h.Write(b)
	}
	write([]byte("nolane.lifecycle-catalog.v1"))
	var version [8]byte
	binary.BigEndian.PutUint64(version[:], uint64(rec.Version))
	write(version[:])
	var seq [8]byte
	binary.BigEndian.PutUint64(seq[:], rec.Sequence)
	write(seq[:])
	write([]byte(rec.WorldID))
	write([]byte(rec.Handle))
	write([]byte(rec.Phase))
	write([]byte(rec.PreviousHash))
	return hex.EncodeToString(h.Sum(nil))
}

func syncCatalogDirectory(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return errors.Join(ErrCatalogCorrupt, err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return errors.Join(ErrCatalogCorrupt, err)
	}
	return nil
}
