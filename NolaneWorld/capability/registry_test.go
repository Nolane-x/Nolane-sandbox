package capability

import (
	"errors"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func candidate(content []byte) Candidate {
	return Candidate{
		CandidateID: "cand-1", OriginWorldID: world.ID("world-1"), Name: "browser", Version: "1.0.0",
		ContentDigest: Digest(content), ManifestDigest: Digest([]byte("manifest")), CreatedAt: time.Unix(1, 0).UTC(),
	}
}

func request(content []byte) PromotionRequest {
	evidence := []byte("verification")
	return PromotionRequest{
		Candidate: candidate(content), Content: content, Manifest: []byte("manifest"),
		VerifierID: "fresh-validator-1", VerificationDigest: Digest(evidence), VerificationEvidence: evidence,
	}
}

func TestCapabilityPromotionIsIndependentAndExactBytesBound(t *testing.T) {
	r := NewRegistry()
	req := request([]byte("tool bytes"))
	receipt, err := r.Promote(req)
	if err != nil {
		t.Fatal(err)
	}
	record, ok := r.Get("browser", "1.0.0")
	if !ok {
		t.Fatal("record missing")
	}
	if record.ContentDigest != req.Candidate.ContentDigest || receipt.ContentDigest != req.Candidate.ContentDigest {
		t.Fatal("digest binding lost")
	}
	if receipt.VerificationDigest != Digest(req.VerificationEvidence) {
		t.Fatal("evidence binding lost")
	}
}

func TestCapabilityRejectsMissingOrMutatedEvidence(t *testing.T) {
	r := NewRegistry()
	req := request([]byte("tool"))
	req.VerificationEvidence = nil
	if _, err := r.Promote(req); !errors.Is(err, ErrInvalidCandidate) {
		t.Fatalf("missing evidence=%v", err)
	}
	req = request([]byte("tool"))
	req.VerificationEvidence = []byte("different")
	if _, err := r.Promote(req); !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("mutated evidence=%v", err)
	}
}

func TestCapabilitySelfPromotionAndMutationsAreRejected(t *testing.T) {
	r := NewRegistry()
	req := request([]byte("tool"))
	req.VerifierID = string(req.Candidate.OriginWorldID)
	if _, err := r.Promote(req); !errors.Is(err, ErrSelfPromotion) {
		t.Fatalf("self=%v", err)
	}
	req = request([]byte("tool"))
	req.Content = []byte("changed")
	if _, err := r.Promote(req); !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("content=%v", err)
	}
	req = request([]byte("tool"))
	req.Manifest = []byte("changed")
	if _, err := r.Promote(req); !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("manifest=%v", err)
	}
}

func TestCapabilityImmutableVersionCandidateAndIdempotency(t *testing.T) {
	r := NewRegistry()
	first := request([]byte("one"))
	one, err := r.Promote(first)
	if err != nil {
		t.Fatal(err)
	}
	two, err := r.Promote(first)
	if err != nil || one != two {
		t.Fatalf("duplicate=(%+v,%v)", two, err)
	}
	second := request([]byte("two"))
	second.Candidate.CandidateID = "cand-2"
	if _, err := r.Promote(second); !errors.Is(err, ErrCapabilityCollision) {
		t.Fatalf("version rebound=%v", err)
	}
	third := request([]byte("one"))
	third.Candidate.Name = "different"
	if _, err := r.Promote(third); !errors.Is(err, ErrCapabilityCollision) {
		t.Fatalf("candidate rebound=%v", err)
	}
}

func TestCapabilityRejectsUnboundedJournalMetadata(t *testing.T) {
	r := NewRegistry()
	req := request([]byte("tool"))
	req.Candidate.Name = string(make([]byte, maxCapabilityNameBytes+1))
	if _, err := r.Promote(req); !errors.Is(err, ErrInvalidCandidate) {
		t.Fatalf("oversized name=%v", err)
	}
}
