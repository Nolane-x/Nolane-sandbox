package github

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/delegation"
)

type evidenceWire struct {
	Version        int                  `json:"version"`
	Provider       string               `json:"provider"`
	Operation      delegation.Operation `json:"operation"`
	ResourceDigest string               `json:"resource_digest"`
	ObjectID       string               `json:"object_id"`
	MarkerDigest   string               `json:"marker_digest"`
	StatusClass    string               `json:"status_class"`
}

func sanitizedEvidence(operation delegation.Operation, resource, objectID, marker string, status int) ([]byte, error) {
	if objectID == "" || marker == "" || status < 100 || status > 599 {
		return nil, ErrProviderResponse
	}
	resourceSum := sha256.Sum256([]byte(resource))
	markerSum := sha256.Sum256([]byte(marker))
	evidence := evidenceWire{
		Version:        1,
		Provider:       string(Kind),
		Operation:      operation,
		ResourceDigest: hex.EncodeToString(resourceSum[:]),
		ObjectID:       objectID,
		MarkerDigest:   hex.EncodeToString(markerSum[:]),
		StatusClass:    statusClass(status),
	}
	raw, err := json.Marshal(evidence)
	if err != nil {
		return nil, ErrProviderResponse
	}
	return raw, nil
}

func statusClass(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "2xx"
	case status >= 300 && status < 400:
		return "3xx"
	case status >= 400 && status < 500:
		return "4xx"
	case status >= 500 && status < 600:
		return "5xx"
	default:
		return "invalid"
	}
}
