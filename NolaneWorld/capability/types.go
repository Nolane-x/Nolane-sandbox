package capability

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type Candidate struct {
	CandidateID    string
	OriginWorldID  world.ID
	Name           string
	Version        string
	ContentDigest  string
	ManifestDigest string
	CreatedAt      time.Time
}

type PromotionRequest struct {
	Candidate          Candidate
	Content            []byte
	Manifest           []byte
	VerifierID         string
	VerificationDigest string
}

type PromotionReceipt struct {
	CapabilityID       string
	CandidateID        string
	CandidateDigest    string
	OriginWorldID      world.ID
	Name               string
	Version            string
	ContentDigest      string
	ManifestDigest     string
	VerifierID         string
	VerificationDigest string
	PromotedAt         time.Time
}

type Record struct {
	Name           string
	Version        string
	ContentDigest  string
	ManifestDigest string
	Receipt        PromotionReceipt
}

var (
	ErrInvalidCandidate    = errors.New("capability: invalid candidate")
	ErrSelfPromotion       = errors.New("capability: self promotion")
	ErrDigestMismatch      = errors.New("capability: digest mismatch")
	ErrCapabilityCollision = errors.New("capability: version collision")
)

func Digest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
