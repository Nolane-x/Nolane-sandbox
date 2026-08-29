package scenarios

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/capability"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func promotionRequest() capability.PromotionRequest {
	content := []byte("gauntlet capability bytes")
	manifest := []byte("gauntlet manifest")
	evidence := []byte("gauntlet independent verification evidence")
	return capability.PromotionRequest{
		Candidate: capability.Candidate{CandidateID: "gauntlet-candidate", OriginWorldID: world.ID("gauntlet-origin"), Name: "browser", Version: "v4", ContentDigest: capability.Digest(content), ManifestDigest: capability.Digest(manifest), CreatedAt: time.Unix(1, 0).UTC()},
		Content:   content, Manifest: manifest, VerifierID: "gauntlet-independent-verifier", VerificationDigest: capability.Digest(evidence), VerificationEvidence: evidence,
	}
}

func prepareRegistry() (string, capability.PromotionRequest, error) {
	root, err := os.MkdirTemp("", "nolane-gauntlet-capability-")
	if err != nil {
		return "", capability.PromotionRequest{}, err
	}
	req := promotionRequest()
	registry, err := capability.OpenDurableRegistry(root)
	if err != nil {
		_ = os.RemoveAll(root)
		return "", req, err
	}
	if _, err := registry.Promote(req); err != nil {
		_ = registry.Close()
		_ = os.RemoveAll(root)
		return "", req, err
	}
	if err := registry.Close(); err != nil {
		_ = os.RemoveAll(root)
		return "", req, err
	}
	return root, req, nil
}

func CapabilityBlobTamperScenario() gauntlet.Scenario {
	return gauntlet.ScenarioFunc{
		Definition: gauntlet.ScenarioSpec{
			ID: "capability.cas-tamper", Invariant: "trusted capability material remains bound to its promoted SHA-256 digest across restart", Attack: "modify the exact content-addressed blob after a valid promotion", ExpectedDefense: "registry recovery detects the digest mismatch and refuses to trust the capability", Severity: gauntlet.SeverityCritical,
			RequiredMarkers: []string{"capability.cas.attack", "capability.cas.boundary", "capability.cas.denied", "capability.cas.untrusted"},
		},
		Execute: func(_ context.Context, p *gauntlet.Probe) error {
			root, req, err := prepareRegistry()
			if err != nil {
				return err
			}
			defer os.RemoveAll(root)
			blob := filepath.Join(root, "blobs", "sha256", req.Candidate.ContentDigest[:2], req.Candidate.ContentDigest)
			if err := p.Record(gauntlet.EventAttack, "capability.cas.attack", "modified trusted content-addressed blob"); err != nil {
				return err
			}
			if err := os.WriteFile(blob, []byte("tampered capability bytes"), 0o600); err != nil {
				return err
			}
			registry, openErr := capability.OpenDurableRegistry(root)
			if registry != nil {
				_ = registry.Close()
			}
			if err := p.Record(gauntlet.EventBoundary, "capability.cas.boundary", "durable registry replay verified referenced blobs"); err != nil {
				return err
			}
			if !errors.Is(openErr, capability.ErrRegistryCorrupt) {
				return fmt.Errorf("tampered blob trusted")
			}
			if err := p.Record(gauntlet.EventDenial, "capability.cas.denied", "registry reopen rejected tampered blob"); err != nil {
				return err
			}
			return p.Record(gauntlet.EventObservation, "capability.cas.untrusted", "tampered capability never reopened as trusted material")
		},
	}
}

func CapabilityJournalTamperScenario() gauntlet.Scenario {
	return gauntlet.ScenarioFunc{
		Definition: gauntlet.ScenarioSpec{
			ID: "capability.journal-tamper", Invariant: "promotion history cannot be rewritten without detection", Attack: "mutate a trust-bearing candidate name byte in the fsynced promotion journal", ExpectedDefense: "hash-chain replay rejects the modified promotion record", Severity: gauntlet.SeverityCritical,
			RequiredMarkers: []string{"capability.journal.attack", "capability.journal.boundary", "capability.journal.denied", "capability.journal.untrusted"},
		},
		Execute: func(_ context.Context, p *gauntlet.Probe) error {
			root, _, err := prepareRegistry()
			if err != nil {
				return err
			}
			defer os.RemoveAll(root)
			journal := filepath.Join(root, "promotions.jsonl")
			raw, err := os.ReadFile(journal)
			if err != nil {
				return err
			}
			changed := bytes.Replace(raw, []byte(`"Name":"browser"`), []byte(`"Name":"browzer"`), 1)
			if bytes.Equal(raw, changed) {
				return fmt.Errorf("journal mutation target absent")
			}
			if err := p.Record(gauntlet.EventAttack, "capability.journal.attack", "mutated trust-bearing promotion journal byte sequence"); err != nil {
				return err
			}
			if err := os.WriteFile(journal, changed, 0o600); err != nil {
				return err
			}
			registry, openErr := capability.OpenDurableRegistry(root)
			if registry != nil {
				_ = registry.Close()
			}
			if err := p.Record(gauntlet.EventBoundary, "capability.journal.boundary", "promotion hash-chain replay executed"); err != nil {
				return err
			}
			if !errors.Is(openErr, capability.ErrRegistryCorrupt) {
				return fmt.Errorf("tampered journal trusted")
			}
			if err := p.Record(gauntlet.EventDenial, "capability.journal.denied", "registry reopen rejected tampered promotion journal"); err != nil {
				return err
			}
			return p.Record(gauntlet.EventObservation, "capability.journal.untrusted", "tampered promotion never re-entered trusted registry state")
		},
	}
}
