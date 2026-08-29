package authority

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type journalStatus string

const (
	journalPending   journalStatus = "pending"
	journalCompleted journalStatus = "completed"
	journalAborted   journalStatus = "aborted"
)

type journalRecord struct {
	Status        journalStatus `json:"status"`
	WorldID       world.ID      `json:"world_id"`
	ActionID      string        `json:"action_id"`
	RequestDigest string        `json:"request_digest"`
	Receipt       *Receipt      `json:"receipt,omitempty"`
}

type journalEntry struct {
	requestDigest string
	pending       bool
	receipt       Receipt
}

type JournalLedger struct {
	mu      sync.RWMutex
	fileMu  sync.Mutex
	entries map[ledgerKey]journalEntry
	locks   [64]sync.Mutex
	file    *os.File
	closed  bool
}

func OpenJournalLedger(path string) (*JournalLedger, error) {
	if path == "" {
		return nil, ErrInvalidAction
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, errors.Join(ErrLedgerCorrupt, err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, errors.Join(ErrLedgerCorrupt, err)
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return nil, errors.Join(ErrLedgerCorrupt, err)
	}
	if err := lockJournal(f.Fd()); err != nil {
		_ = f.Close()
		return nil, err
	}
	l := &JournalLedger{entries: make(map[ledgerKey]journalEntry), file: f}
	if err := l.recover(); err != nil {
		_ = unlockJournal(f.Fd())
		_ = f.Close()
		return nil, err
	}
	return l, nil
}

func (l *JournalLedger) ExecuteOnce(worldID world.ID, actionID, requestDigest string, fn func() (Receipt, error)) (Receipt, error) {
	if l == nil || worldID == "" || actionID == "" || requestDigest == "" || fn == nil {
		return Receipt{}, ErrInvalidAction
	}
	key := ledgerKey{worldID: worldID, actionID: actionID}
	lock := &l.locks[shardFor(key)]
	lock.Lock()
	defer lock.Unlock()

	if err := l.ensureOpen(); err != nil {
		return Receipt{}, err
	}
	if entry, ok := l.get(key); ok {
		if entry.requestDigest != requestDigest {
			return Receipt{}, ErrActionCollision
		}
		if entry.pending {
			return Receipt{}, ErrActionUncertain
		}
		return entry.receipt, nil
	}

	pending := journalRecord{Status: journalPending, WorldID: worldID, ActionID: actionID, RequestDigest: requestDigest}
	if err := l.appendRecord(pending); err != nil {
		return Receipt{}, err
	}
	l.set(key, journalEntry{requestDigest: requestDigest, pending: true})

	receipt, err := fn()
	if err != nil {
		if definitelyNoEffect(err) {
			aborted := journalRecord{Status: journalAborted, WorldID: worldID, ActionID: actionID, RequestDigest: requestDigest}
			if appendErr := l.appendRecord(aborted); appendErr != nil {
				return Receipt{}, errors.Join(err, appendErr)
			}
			l.delete(key)
		}
		return Receipt{}, err
	}
	if receipt.WorldID != worldID || receipt.ActionID != actionID || receipt.RequestDigest != requestDigest {
		return Receipt{}, ErrExecutionFailure
	}

	completed := journalRecord{Status: journalCompleted, WorldID: worldID, ActionID: actionID, RequestDigest: requestDigest, Receipt: &receipt}
	if err := l.appendRecord(completed); err != nil {
		return Receipt{}, errors.Join(ErrActionUncertain, err)
	}
	l.set(key, journalEntry{requestDigest: requestDigest, receipt: receipt})
	return receipt, nil
}

func (l *JournalLedger) Status(worldID world.ID, actionID, requestDigest string) (ActionStatus, Receipt, error) {
	if l == nil || worldID == "" || actionID == "" || requestDigest == "" {
		return ActionMissing, Receipt{}, ErrInvalidAction
	}
	key := ledgerKey{worldID: worldID, actionID: actionID}
	lock := &l.locks[shardFor(key)]
	lock.Lock()
	defer lock.Unlock()
	if err := l.ensureOpen(); err != nil {
		return ActionMissing, Receipt{}, err
	}
	entry, ok := l.get(key)
	if !ok {
		return ActionMissing, Receipt{}, nil
	}
	if entry.requestDigest != requestDigest {
		return ActionMissing, Receipt{}, ErrActionCollision
	}
	if entry.pending {
		return ActionPending, Receipt{}, nil
	}
	return ActionCompleted, entry.receipt, nil
}

func (l *JournalLedger) Resolve(worldID world.ID, actionID, requestDigest string, receipt Receipt) error {
	if l == nil || worldID == "" || actionID == "" || requestDigest == "" {
		return ErrInvalidAction
	}
	key := ledgerKey{worldID: worldID, actionID: actionID}
	lock := &l.locks[shardFor(key)]
	lock.Lock()
	defer lock.Unlock()

	if err := l.ensureOpen(); err != nil {
		return err
	}
	entry, ok := l.get(key)
	if !ok || !entry.pending {
		return ErrInvalidAction
	}
	if entry.requestDigest != requestDigest {
		return ErrActionCollision
	}
	if receipt.WorldID != worldID || receipt.ActionID != actionID || receipt.RequestDigest != requestDigest {
		return ErrExecutionFailure
	}
	rec := journalRecord{Status: journalCompleted, WorldID: worldID, ActionID: actionID, RequestDigest: requestDigest, Receipt: &receipt}
	if err := l.appendRecord(rec); err != nil {
		return err
	}
	l.set(key, journalEntry{requestDigest: requestDigest, receipt: receipt})
	return nil
}

func (l *JournalLedger) Close() error {
	if l == nil {
		return nil
	}
	l.fileMu.Lock()
	defer l.fileMu.Unlock()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	unlockErr := unlockJournal(l.file.Fd())
	closeErr := l.file.Close()
	return errors.Join(unlockErr, closeErr)
}

func (l *JournalLedger) recover() error {
	if _, err := l.file.Seek(0, 0); err != nil {
		return errors.Join(ErrLedgerCorrupt, err)
	}
	scanner := bufio.NewScanner(l.file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		var rec journalRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			return fmt.Errorf("%w: line %d: %v", ErrLedgerCorrupt, line, err)
		}
		if err := l.applyRecovered(rec); err != nil {
			return fmt.Errorf("%w: line %d: %v", ErrLedgerCorrupt, line, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return errors.Join(ErrLedgerCorrupt, err)
	}
	_, err := l.file.Seek(0, 2)
	return err
}

func (l *JournalLedger) applyRecovered(rec journalRecord) error {
	if rec.WorldID == "" || rec.ActionID == "" || rec.RequestDigest == "" {
		return ErrInvalidAction
	}
	key := ledgerKey{worldID: rec.WorldID, actionID: rec.ActionID}
	entry, exists := l.entries[key]
	if exists && entry.requestDigest != rec.RequestDigest {
		return ErrActionCollision
	}
	switch rec.Status {
	case journalPending:
		if exists {
			return errors.New("duplicate pending transition")
		}
		l.entries[key] = journalEntry{requestDigest: rec.RequestDigest, pending: true}
	case journalAborted:
		if !exists || !entry.pending {
			return errors.New("aborted without pending")
		}
		delete(l.entries, key)
	case journalCompleted:
		if !exists || !entry.pending || rec.Receipt == nil {
			return errors.New("completed without pending receipt")
		}
		if rec.Receipt.WorldID != rec.WorldID || rec.Receipt.ActionID != rec.ActionID || rec.Receipt.RequestDigest != rec.RequestDigest {
			return ErrExecutionFailure
		}
		l.entries[key] = journalEntry{requestDigest: rec.RequestDigest, receipt: *rec.Receipt}
	default:
		return errors.New("unknown journal status")
	}
	return nil
}

func (l *JournalLedger) appendRecord(rec journalRecord) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return errors.Join(ErrLedgerCorrupt, err)
	}
	raw = append(raw, '\n')
	l.fileMu.Lock()
	defer l.fileMu.Unlock()
	l.mu.RLock()
	closed := l.closed
	l.mu.RUnlock()
	if closed {
		return ErrLedgerClosed
	}
	if _, err := l.file.Write(raw); err != nil {
		return errors.Join(ErrLedgerCorrupt, err)
	}
	if err := l.file.Sync(); err != nil {
		return errors.Join(ErrLedgerCorrupt, err)
	}
	return nil
}

func (l *JournalLedger) ensureOpen() error {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.closed {
		return ErrLedgerClosed
	}
	return nil
}

func (l *JournalLedger) get(key ledgerKey) (journalEntry, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	v, ok := l.entries[key]
	return v, ok
}
func (l *JournalLedger) set(key ledgerKey, value journalEntry) {
	l.mu.Lock()
	l.entries[key] = value
	l.mu.Unlock()
}
func (l *JournalLedger) delete(key ledgerKey) {
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}

func definitelyNoEffect(err error) bool {
	return errors.Is(err, ErrDenied) || errors.Is(err, ErrPolicyFailure)
}
