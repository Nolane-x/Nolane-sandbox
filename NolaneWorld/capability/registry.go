package capability

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

type registryKey struct{ name, version string }

type Registry struct {
	mu          sync.RWMutex
	records     map[registryKey]Record
	byCandidate map[string]PromotionReceipt
	now         func() time.Time
}

func NewRegistry() *Registry {
	return &Registry{
		records:     make(map[registryKey]Record),
		byCandidate: make(map[string]PromotionReceipt),
		now:         func() time.Time { return time.Now().UTC() },
	}
}

func (r *Registry) Promote(req PromotionRequest) (PromotionReceipt, error) {
	if r == nil {
		return PromotionReceipt{}, ErrInvalidCandidate
	}
	c := req.Candidate
	if c.CandidateID == "" || c.OriginWorldID == "" || c.Name == "" || c.Version == "" || c.ContentDigest == "" || c.ManifestDigest == "" || c.CreatedAt.IsZero() || req.VerifierID == "" || req.VerificationDigest == "" {
		return PromotionReceipt{}, ErrInvalidCandidate
	}
	if req.VerifierID == string(c.OriginWorldID) {
		return PromotionReceipt{}, ErrSelfPromotion
	}
	if Digest(req.Content) != c.ContentDigest || Digest(req.Manifest) != c.ManifestDigest {
		return PromotionReceipt{}, ErrDigestMismatch
	}

	key := registryKey{name: c.Name, version: c.Version}
	r.mu.Lock()
	defer r.mu.Unlock()

	if prior, ok := r.byCandidate[c.CandidateID]; ok {
		if prior.ContentDigest != c.ContentDigest || prior.OriginWorldID != c.OriginWorldID || prior.VerifierID != req.VerifierID || prior.VerificationDigest != req.VerificationDigest {
			return PromotionReceipt{}, ErrCapabilityCollision
		}
		return prior, nil
	}
	if existing, ok := r.records[key]; ok && existing.ContentDigest != c.ContentDigest {
		return PromotionReceipt{}, ErrCapabilityCollision
	}

	receipt := PromotionReceipt{
		CapabilityID: capabilityID(c.Name, c.Version, c.ContentDigest), CandidateID: c.CandidateID,
		OriginWorldID: c.OriginWorldID, ContentDigest: c.ContentDigest, VerifierID: req.VerifierID,
		VerificationDigest: req.VerificationDigest, PromotedAt: r.now(),
	}
	r.records[key] = Record{Name: c.Name, Version: c.Version, ContentDigest: c.ContentDigest, Receipt: receipt}
	r.byCandidate[c.CandidateID] = receipt
	return receipt, nil
}

func (r *Registry) Get(name, version string) (Record, bool) {
	if r == nil {
		return Record{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	record, ok := r.records[registryKey{name: name, version: version}]
	return record, ok
}

func capabilityID(name, version, digest string) string {
	sum := sha256.Sum256([]byte(name + "\x00" + version + "\x00" + digest))
	return hex.EncodeToString(sum[:])
}
