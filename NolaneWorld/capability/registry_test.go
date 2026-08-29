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
	return PromotionRequest{Candidate: candidate(content), Content: content, Manifest: []byte("manifest"), VerifierID: "fresh-validator-1", VerificationDigest: Digest([]byte("verification"))}
}

func TestCapabilityPromotionIsIndependentAndContentBound(t *testing.T) {
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
	if receipt.OriginWorldID != "world-1" || receipt.VerifierID != "fresh-validator-1" {
		t.Fatal("identity binding lost")
	}
}

func TestCapabilitySelfPromotionIsRejected(t *testing.T) {
	r := NewRegistry()
	req := request([]byte("tool"))
	req.VerifierID = string(req.Candidate.OriginWorldID)
	if _, err := r.Promote(req); !errors.Is(err, ErrSelfPromotion) {
		t.Fatalf("got %v", err)
	}
}

func TestCapabilityContentMutationAndMissingEvidenceAreRejected(t *testing.T) {
	r := NewRegistry()
	req := request([]byte("tool"))
	req.Content = []byte("changed")
	if _, err := r.Promote(req); !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("mutation: %v", err)
	}
	req = request([]byte("tool"))
	req.VerificationDigest = ""
	if _, err := r.Promote(req); !errors.Is(err, ErrInvalidCandidate) {
		t.Fatalf("missing evidence: %v", err)
	}
}

func TestCapabilitySameVersionCannotBeReboundToDifferentContent(t *testing.T) {
	r := NewRegistry()
	first := request([]byte("one"))
	if _, err := r.Promote(first); err != nil {
		t.Fatal(err)
	}
	second := request([]byte("two"))
	second.Candidate.CandidateID = "cand-2"
	if _, err := r.Promote(second); !errors.Is(err, ErrCapabilityCollision) {
		t.Fatalf("got %v", err)
	}
}

func TestCapabilityExactDuplicatePromotionIsIdempotent(t *testing.T) {
	r := NewRegistry()
	req := request([]byte("same"))
	one, err := r.Promote(req)
	if err != nil {
		t.Fatal(err)
	}
	two, err := r.Promote(req)
	if err != nil {
		t.Fatal(err)
	}
	if one != two {
		t.Fatalf("promotion receipt changed\none=%#v\ntwo=%#v", one, two)
	}
}

func TestCapabilityManifestMutationIsRejected(t *testing.T) {
	r := NewRegistry()
	req := request([]byte("tool"))
	req.Manifest = []byte("changed manifest")
	if _, err := r.Promote(req); !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("manifest mutation: %v", err)
	}
}
