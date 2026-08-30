package fabric

import (
	"encoding/hex"
	"errors"
	"sync"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

var (
	ErrInvalidBaseline   = errors.New("fabric: invalid baseline")
	ErrBaselineCollision = errors.New("fabric: baseline collision")
)

type Baseline struct {
	ID              string               `json:"id"`
	Digest          string               `json:"digest"`
	TemplateRef     string               `json:"template_ref"`
	NetworkProfile  realm.NetworkProfile `json:"network_profile"`
	Sanitized       bool                 `json:"sanitized"`
	WorldIdentity   string               `json:"world_identity,omitempty"`
	CheckpointOwner string               `json:"checkpoint_owner,omitempty"`
}

type BaselineCatalog struct {
	mu    sync.RWMutex
	items map[string]Baseline
}

func NewBaselineCatalog() *BaselineCatalog { return &BaselineCatalog{items: make(map[string]Baseline)} }

func (c *BaselineCatalog) Admit(b Baseline) error {
	if c == nil || !validBaseline(b) {
		return ErrInvalidBaseline
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.items[b.ID]; ok {
		if old != b {
			return ErrBaselineCollision
		}
		return nil
	}
	c.items[b.ID] = b
	return nil
}

func (c *BaselineCatalog) Select(profile realm.NetworkProfile) (Baseline, bool) {
	if c == nil || !profile.Valid() {
		return Baseline{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	var best Baseline
	found := false
	for _, b := range c.items {
		if b.NetworkProfile != profile {
			continue
		}
		if !found || b.ID < best.ID {
			best = b
			found = true
		}
	}
	return best, found
}

func validBaseline(b Baseline) bool {
	if b.ID == "" || b.TemplateRef == "" || !b.NetworkProfile.Valid() || !b.Sanitized || b.WorldIdentity != "" || b.CheckpointOwner != "" || len(b.Digest) != 64 {
		return false
	}
	_, err := hex.DecodeString(b.Digest)
	return err == nil
}
