package scenarios

import (
	"context"
	"errors"
	"fmt"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/artifact"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func ArtifactTraversalScenario() gauntlet.Scenario {
	return gauntlet.ScenarioFunc{
		Definition: gauntlet.ScenarioSpec{
			ID: "artifact.path-traversal", Invariant: "guest-controlled logical artifact names cannot escape the artifact namespace", Attack: "submit traversal, absolute, backslash, dot-component, empty-segment, and NUL logical names", ExpectedDefense: "artifact gate rejects every hostile logical name before issuing a receipt", Severity: gauntlet.SeverityHigh,
			RequiredMarkers: []string{"artifact.traversal.attack", "artifact.traversal.boundary", "artifact.traversal.denied", "artifact.traversal.zero-receipts"},
		},
		Execute: func(_ context.Context, p *gauntlet.Probe) error {
			gate := artifact.Gate{MaxBytes: 1024}
			hostile := []string{"../escape", "/absolute", `a\\b`, "a/./b", "a//b", "a/../b", "a\x00b"}
			if err := p.Record(gauntlet.EventAttack, "artifact.traversal.attack", "submitted hostile logical-name corpus"); err != nil {
				return err
			}
			for _, name := range hostile {
				if _, err := gate.Accept(world.ID("gauntlet-artifact"), name, "application/octet-stream", []byte("x")); !errors.Is(err, artifact.ErrInvalidArtifact) {
					return fmt.Errorf("hostile logical name accepted")
				}
			}
			if err := p.Record(gauntlet.EventBoundary, "artifact.traversal.boundary", "artifact logical-name validation executed for full corpus"); err != nil {
				return err
			}
			if err := p.Record(gauntlet.EventDenial, "artifact.traversal.denied", "all hostile logical names rejected"); err != nil {
				return err
			}
			return p.Record(gauntlet.EventObservation, "artifact.traversal.zero-receipts", "no hostile artifact receipt was issued")
		},
	}
}
