package capability

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
	"time"
)

const durableRegistryVersion = 1

type durablePromotionRecord struct {
	Version      int              `json:"version"`
	Sequence     uint64           `json:"sequence"`
	Candidate    Candidate        `json:"candidate"`
	Receipt      PromotionReceipt `json:"receipt"`
	PreviousHash string           `json:"previous_hash"`
	RecordHash   string           `json:"record_hash"`
}

type DurableRegistry struct {
	root        string
	blobRoot    string
	journalPath string
	mu          sync.RWMutex
	file        *os.File
	closed      bool
	sequence    uint64
	lastHash    string
	records     map[registryKey]Record
	candidates  map[string]PromotionReceipt
	now         func() time.Time
}

func OpenDurableRegistry(root string) (*DurableRegistry, error) {
	if root == "" {
		return nil, ErrRegistryCorrupt
	}
	root = filepath.Clean(root)
	blobRoot := filepath.Join(root, "blobs", "sha256")
	if err := os.MkdirAll(blobRoot, 0o700); err != nil {
		return nil, errors.Join(ErrRegistryCorrupt, err)
	}
	for _, dir := range []string{root, filepath.Join(root, "blobs"), blobRoot} {
		if err := os.Chmod(dir, 0o700); err != nil {
			return nil, errors.Join(ErrRegistryCorrupt, err)
		}
	}
	journal := filepath.Join(root, "promotions.jsonl")
	_, statErr := os.Stat(journal)
	newJournal := errors.Is(statErr, os.ErrNotExist)
	file, err := os.OpenFile(journal, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, errors.Join(ErrRegistryCorrupt, err)
	}
	if err := os.Chmod(journal, 0o600); err != nil {
		_ = file.Close()
		return nil, errors.Join(ErrRegistryCorrupt, err)
	}
	if err := lockRegistryFile(file.Fd()); err != nil {
		_ = file.Close()
		return nil, err
	}
	r := &DurableRegistry{
		root: root, blobRoot: blobRoot, journalPath: journal, file: file,
		records: make(map[registryKey]Record), candidates: make(map[string]PromotionReceipt),
		now: func() time.Time { return time.Now().UTC() },
	}
	if err := r.recover(); err != nil {
		_ = unlockRegistryFile(file.Fd())
		_ = file.Close()
		return nil, err
	}
	if newJournal {
		if err := syncRegistryDir(root); err != nil {
			_ = r.Close()
			return nil, err
		}
	}
	return r, nil
}

func (r *DurableRegistry) Promote(req PromotionRequest) (PromotionReceipt, error) {
	if r == nil {
		return PromotionReceipt{}, ErrRegistryClosed
	}
	req = clonePromotionRequest(req)
	if err := validatePromotionRequest(req); err != nil {
		return PromotionReceipt{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return PromotionReceipt{}, ErrRegistryClosed
	}
	if prior, ok, err := promotionCollisionCheck(r.records, r.candidates, req); ok || err != nil {
		if err == nil {
			if verifyErr := r.verifyReceiptBlobs(prior); verifyErr != nil {
				return PromotionReceipt{}, verifyErr
			}
		}
		return prior, err
	}

	for digest, content := range map[string][]byte{
		req.Candidate.ContentDigest:  req.Content,
		req.Candidate.ManifestDigest: req.Manifest,
		req.VerificationDigest:       req.VerificationEvidence,
	} {
		if err := r.putBlob(digest, content); err != nil {
			return PromotionReceipt{}, err
		}
	}

	receipt := newPromotionReceipt(req, r.now())
	rec := durablePromotionRecord{
		Version: durableRegistryVersion, Sequence: r.sequence + 1, Candidate: req.Candidate,
		Receipt: receipt, PreviousHash: r.lastHash,
	}
	rec.RecordHash = hashDurablePromotion(rec)
	if err := writeDurablePromotion(r.file, rec); err != nil {
		return PromotionReceipt{}, err
	}
	r.sequence = rec.Sequence
	r.lastHash = rec.RecordHash
	r.records[registryKey{name: receipt.Name, version: receipt.Version}] = recordFromReceipt(receipt)
	r.candidates[receipt.CandidateID] = receipt
	return receipt, nil
}

func (r *DurableRegistry) Get(name, version string) (Record, bool) {
	if r == nil {
		return Record{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return Record{}, false
	}
	rec, ok := r.records[registryKey{name: name, version: version}]
	return rec, ok
}

func (r *DurableRegistry) Load(name, version string) (Material, error) {
	if r == nil {
		return Material{}, ErrRegistryClosed
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return Material{}, ErrRegistryClosed
	}
	rec, ok := r.records[registryKey{name: name, version: version}]
	if !ok {
		return Material{}, ErrInvalidCandidate
	}
	content, err := r.readBlob(rec.ContentDigest)
	if err != nil {
		return Material{}, err
	}
	manifest, err := r.readBlob(rec.ManifestDigest)
	if err != nil {
		return Material{}, err
	}
	evidence, err := r.readBlob(rec.Receipt.VerificationDigest)
	if err != nil {
		return Material{}, err
	}
	return Material{Record: rec, Content: content, Manifest: manifest, VerificationEvidence: evidence}, nil
}

func (r *DurableRegistry) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	return errors.Join(unlockRegistryFile(r.file.Fd()), r.file.Close())
}

func (r *DurableRegistry) recover() error {
	if _, err := r.file.Seek(0, 0); err != nil {
		return errors.Join(ErrRegistryCorrupt, err)
	}
	scanner := bufio.NewScanner(r.file)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	var sequence uint64
	var previous string
	line := 0
	for scanner.Scan() {
		line++
		rec, err := decodeDurablePromotion(scanner.Bytes())
		if err != nil {
			return fmt.Errorf("%w: line %d: %v", ErrRegistryCorrupt, line, err)
		}
		if rec.Version != durableRegistryVersion || rec.Sequence != sequence+1 || rec.PreviousHash != previous || rec.RecordHash != hashDurablePromotion(rec) {
			return fmt.Errorf("%w: line %d: hash/sequence mismatch", ErrRegistryCorrupt, line)
		}
		if err := validateRecoveredPromotion(rec); err != nil {
			return fmt.Errorf("%w: line %d: %v", ErrRegistryCorrupt, line, err)
		}
		key := registryKey{name: rec.Receipt.Name, version: rec.Receipt.Version}
		if _, exists := r.records[key]; exists {
			return fmt.Errorf("%w: duplicate capability version", ErrRegistryCorrupt)
		}
		if _, exists := r.candidates[rec.Receipt.CandidateID]; exists {
			return fmt.Errorf("%w: duplicate candidate", ErrRegistryCorrupt)
		}
		if err := r.verifyReceiptBlobs(rec.Receipt); err != nil {
			return err
		}
		r.records[key] = recordFromReceipt(rec.Receipt)
		r.candidates[rec.Receipt.CandidateID] = rec.Receipt
		sequence = rec.Sequence
		previous = rec.RecordHash
	}
	if err := scanner.Err(); err != nil {
		return errors.Join(ErrRegistryCorrupt, err)
	}
	if _, err := r.file.Seek(0, io.SeekEnd); err != nil {
		return errors.Join(ErrRegistryCorrupt, err)
	}
	r.sequence = sequence
	r.lastHash = previous
	return nil
}

func validateRecoveredPromotion(rec durablePromotionRecord) error {
	c := rec.Candidate
	p := rec.Receipt
	if c.CandidateID == "" || c.OriginWorldID == "" || c.Name == "" || c.Version == "" || c.ContentDigest == "" || c.ManifestDigest == "" || c.CreatedAt.IsZero() {
		return ErrInvalidCandidate
	}
	if p.CandidateID != c.CandidateID || p.CandidateDigest != candidateDigest(c) || p.OriginWorldID != c.OriginWorldID || p.Name != c.Name || p.Version != c.Version || p.ContentDigest != c.ContentDigest || p.ManifestDigest != c.ManifestDigest || p.VerifierID == "" || p.VerifierID == string(c.OriginWorldID) || p.VerificationDigest == "" || p.PromotedAt.IsZero() {
		return ErrRegistryCorrupt
	}
	if p.CapabilityID != capabilityID(c.Name, c.Version, c.ContentDigest, c.ManifestDigest) {
		return ErrRegistryCorrupt
	}
	for _, digest := range []string{c.ContentDigest, c.ManifestDigest, p.VerificationDigest} {
		if !validDigest(digest) {
			return ErrRegistryCorrupt
		}
	}
	return nil
}

func (r *DurableRegistry) verifyReceiptBlobs(receipt PromotionReceipt) error {
	for _, digest := range []string{receipt.ContentDigest, receipt.ManifestDigest, receipt.VerificationDigest} {
		if err := r.verifyBlob(digest); err != nil {
			return err
		}
	}
	return nil
}

func (r *DurableRegistry) blobPath(digest string) (string, error) {
	if !validDigest(digest) {
		return "", ErrRegistryCorrupt
	}
	return filepath.Join(r.blobRoot, digest[:2], digest), nil
}

func validDigest(digest string) bool {
	if len(digest) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

func (r *DurableRegistry) putBlob(digest string, content []byte) error {
	if Digest(content) != digest {
		return ErrDigestMismatch
	}
	path, err := r.blobPath(digest)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return r.verifyBlob(digest)
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.Join(ErrRegistryCorrupt, err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errors.Join(ErrRegistryCorrupt, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return errors.Join(ErrRegistryCorrupt, err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return r.verifyBlob(digest)
		}
		return errors.Join(ErrRegistryCorrupt, err)
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(content); err != nil {
		return errors.Join(ErrRegistryCorrupt, err)
	}
	if err := file.Sync(); err != nil {
		return errors.Join(ErrRegistryCorrupt, err)
	}
	if err := file.Close(); err != nil {
		return errors.Join(ErrRegistryCorrupt, err)
	}
	if err := syncRegistryDir(dir); err != nil {
		return err
	}
	ok = true
	return nil
}

func (r *DurableRegistry) verifyBlob(digest string) error {
	path, err := r.blobPath(digest)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return errors.Join(ErrRegistryCorrupt, err)
	}
	file, err := os.Open(path)
	if err != nil {
		return errors.Join(ErrRegistryCorrupt, err)
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return errors.Join(ErrRegistryCorrupt, err)
	}
	if hex.EncodeToString(h.Sum(nil)) != digest {
		return ErrRegistryCorrupt
	}
	return nil
}

func (r *DurableRegistry) readBlob(digest string) ([]byte, error) {
	if err := r.verifyBlob(digest); err != nil {
		return nil, err
	}
	path, _ := r.blobPath(digest)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Join(ErrRegistryCorrupt, err)
	}
	return content, nil
}

func decodeDurablePromotion(raw []byte) (durablePromotionRecord, error) {
	var rec durablePromotionRecord
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

func writeDurablePromotion(file *os.File, rec durablePromotionRecord) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return errors.Join(ErrRegistryCorrupt, err)
	}
	if _, err := file.Write(append(raw, '\n')); err != nil {
		return errors.Join(ErrRegistryCorrupt, err)
	}
	if err := file.Sync(); err != nil {
		return errors.Join(ErrRegistryCorrupt, err)
	}
	return nil
}

func hashDurablePromotion(rec durablePromotionRecord) string {
	h := sha256.New()
	write := func(b []byte) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(b)))
		_, _ = h.Write(n[:])
		_, _ = h.Write(b)
	}
	write([]byte("nolane.capability-registry.v1"))
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(rec.Version))
	write(n[:])
	binary.BigEndian.PutUint64(n[:], rec.Sequence)
	write(n[:])
	c := rec.Candidate
	write([]byte(c.CandidateID))
	write([]byte(c.OriginWorldID))
	write([]byte(c.Name))
	write([]byte(c.Version))
	write([]byte(c.ContentDigest))
	write([]byte(c.ManifestDigest))
	write([]byte(c.CreatedAt.UTC().Format(time.RFC3339Nano)))
	p := rec.Receipt
	write([]byte(p.CapabilityID))
	write([]byte(p.CandidateID))
	write([]byte(p.CandidateDigest))
	write([]byte(p.OriginWorldID))
	write([]byte(p.Name))
	write([]byte(p.Version))
	write([]byte(p.ContentDigest))
	write([]byte(p.ManifestDigest))
	write([]byte(p.VerifierID))
	write([]byte(p.VerificationDigest))
	write([]byte(p.PromotedAt.UTC().Format(time.RFC3339Nano)))
	write([]byte(rec.PreviousHash))
	return hex.EncodeToString(h.Sum(nil))
}

func syncRegistryDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return errors.Join(ErrRegistryCorrupt, err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return errors.Join(ErrRegistryCorrupt, err)
	}
	return nil
}
