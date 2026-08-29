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
	req = clonePromotionRequest(req)
	if err := validatePromotionRequest(req); err != nil {
		return PromotionReceipt{}, err
	}

	key := registryKey{name: req.Candidate.Name, version: req.Candidate.Version}
	r.mu.Lock()
	defer r.mu.Unlock()
	if prior, ok, err := promotionCollisionCheck(r.records, r.byCandidate, req); ok || err != nil {
		return prior, err
	}

	receipt := newPromotionReceipt(req, r.now())
	r.records[key] = recordFromReceipt(receipt)
	r.byCandidate[req.Candidate.CandidateID] = receipt
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

const (
	maxCandidateIDBytes       = 256
	maxWorldIDBytes           = 512
	maxCapabilityNameBytes    = 512
	maxCapabilityVersionBytes = 128
	maxVerifierIDBytes        = 256
)

func clonePromotionRequest(req PromotionRequest) PromotionRequest {
	out := req
	out.Content = append([]byte(nil), req.Content...)
	out.Manifest = append([]byte(nil), req.Manifest...)
	out.VerificationEvidence = append([]byte(nil), req.VerificationEvidence...)
	return out
}

func validatePromotionRequest(req PromotionRequest) error {
	c := req.Candidate
	if c.CandidateID == "" || c.OriginWorldID == "" || c.Name == "" || c.Version == "" || c.ContentDigest == "" || c.ManifestDigest == "" || c.CreatedAt.IsZero() || req.VerifierID == "" || req.VerificationDigest == "" || len(req.VerificationEvidence) == 0 {
		return ErrInvalidCandidate
	}
	if len(c.CandidateID) > maxCandidateIDBytes || len(c.OriginWorldID) > maxWorldIDBytes || len(c.Name) > maxCapabilityNameBytes || len(c.Version) > maxCapabilityVersionBytes || len(req.VerifierID) > maxVerifierIDBytes {
		return ErrInvalidCandidate
	}
	if req.VerifierID == string(c.OriginWorldID) {
		return ErrSelfPromotion
	}
	if Digest(req.Content) != c.ContentDigest || Digest(req.Manifest) != c.ManifestDigest || Digest(req.VerificationEvidence) != req.VerificationDigest {
		return ErrDigestMismatch
	}
	return nil
}

func promotionCollisionCheck(records map[registryKey]Record, byCandidate map[string]PromotionReceipt, req PromotionRequest) (PromotionReceipt, bool, error) {
	c := req.Candidate
	binding := candidateDigest(c)
	if prior, ok := byCandidate[c.CandidateID]; ok {
		if prior.CandidateDigest != binding || prior.VerifierID != req.VerifierID || prior.VerificationDigest != req.VerificationDigest {
			return PromotionReceipt{}, true, ErrCapabilityCollision
		}
		return prior, true, nil
	}
	key := registryKey{name: c.Name, version: c.Version}
	if existing, ok := records[key]; ok {
		if existing.Receipt.CandidateID != c.CandidateID || existing.ContentDigest != c.ContentDigest || existing.ManifestDigest != c.ManifestDigest || existing.Receipt.VerificationDigest != req.VerificationDigest {
			return PromotionReceipt{}, true, ErrCapabilityCollision
		}
		return existing.Receipt, true, nil
	}
	return PromotionReceipt{}, false, nil
}

func newPromotionReceipt(req PromotionRequest, promotedAt time.Time) PromotionReceipt {
	c := req.Candidate
	return PromotionReceipt{
		CapabilityID: capabilityID(c.Name, c.Version, c.ContentDigest, c.ManifestDigest), CandidateID: c.CandidateID,
		CandidateDigest: candidateDigest(c), OriginWorldID: c.OriginWorldID, Name: c.Name, Version: c.Version,
		ContentDigest: c.ContentDigest, ManifestDigest: c.ManifestDigest, VerifierID: req.VerifierID,
		VerificationDigest: req.VerificationDigest, PromotedAt: promotedAt.UTC(),
	}
}

func recordFromReceipt(receipt PromotionReceipt) Record {
	return Record{Name: receipt.Name, Version: receipt.Version, ContentDigest: receipt.ContentDigest, ManifestDigest: receipt.ManifestDigest, Receipt: receipt}
}

func capabilityID(name, version, contentDigest, manifestDigest string) string {
	sum := sha256.Sum256([]byte("nolane.capability.v1\x00" + name + "\x00" + version + "\x00" + contentDigest + "\x00" + manifestDigest))
	return hex.EncodeToString(sum[:])
}

func candidateDigest(c Candidate) string {
	material := "nolane.candidate.v1\x00" + c.CandidateID + "\x00" + string(c.OriginWorldID) + "\x00" + c.Name + "\x00" + c.Version + "\x00" + c.ContentDigest + "\x00" + c.ManifestDigest + "\x00" + c.CreatedAt.UTC().Format(time.RFC3339Nano)
	sum := sha256.Sum256([]byte(material))
	return hex.EncodeToString(sum[:])
}
