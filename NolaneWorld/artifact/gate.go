package artifact

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"path"
	"strings"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type Gate struct{ MaxBytes int64 }

type Envelope struct {
	WorldID       world.ID
	LogicalName   string
	MediaType     string
	Size          int64
	ContentDigest string
	ReceiptDigest string
}

type Receipt = Envelope

var (
	ErrInvalidArtifact  = errors.New("artifact: invalid artifact")
	ErrArtifactTooLarge = errors.New("artifact: too large")
)

func (g Gate) Accept(worldID world.ID, logicalName, mediaType string, content []byte) (Receipt, error) {
	if worldID == "" || mediaType == "" || len(content) == 0 || !safeLogicalName(logicalName) {
		return Receipt{}, ErrInvalidArtifact
	}
	if g.MaxBytes <= 0 {
		return Receipt{}, ErrInvalidArtifact
	}
	if int64(len(content)) > g.MaxBytes {
		return Receipt{}, ErrArtifactTooLarge
	}
	contentSum := sha256.Sum256(content)
	contentDigest := hex.EncodeToString(contentSum[:])
	receipt := Receipt{
		WorldID:       worldID,
		LogicalName:   logicalName,
		MediaType:     mediaType,
		Size:          int64(len(content)),
		ContentDigest: contentDigest,
	}
	receipt.ReceiptDigest = receiptDigest(receipt)
	return receipt, nil
}

func receiptDigest(r Receipt) string {
	h := sha256.New()
	writeHashField(h, []byte(r.WorldID))
	writeHashField(h, []byte(r.LogicalName))
	writeHashField(h, []byte(r.MediaType))
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(r.Size))
	writeHashField(h, size[:])
	writeHashField(h, []byte(r.ContentDigest))
	return hex.EncodeToString(h.Sum(nil))
}

func writeHashField(h hash.Hash, b []byte) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(b)))
	_, _ = h.Write(n[:])
	_, _ = h.Write(b)
}

func safeLogicalName(name string) bool {
	if name == "" || strings.ContainsRune(name, '\x00') || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") {
		return false
	}
	parts := strings.Split(name, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	clean := path.Clean(name)
	return clean == name && clean != "." && !strings.HasPrefix(clean, "../")
}
