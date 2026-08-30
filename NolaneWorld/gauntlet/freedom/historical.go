package freedomgauntlet

import (
	"context"
	"errors"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet"
	delegationgauntlet "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/delegation"
	providergauntlet "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/provider"
	v4scenarios "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/scenarios"
)

func historicalNondriftCanonical(ctx context.Context, p *gauntlet.Probe) error {
	if err := attack(p, "historical-evidence-regeneration", "v4, v6, and v7 deterministic evidence suites were regenerated using their exact released policies while Freedom Plane v8 code was active"); err != nil {
		return err
	}

	v4Policy := gauntlet.Policy{ProductID: gauntlet.ProductNolaneSandbox, ScenarioTimeout: 2 * time.Second}
	v4, err := v4scenarios.RunStandard(ctx, v4Policy)
	if err != nil || !v4.Approved {
		return errors.New("v4 evidence no longer approved")
	}
	v4raw, err := gauntlet.MarshalReport(v4)
	if err != nil {
		return err
	}
	if digestBytes(v4raw) != v4EvidenceSHA256 {
		return errors.New("v4 canonical evidence drifted")
	}

	v6Policy := gauntlet.Policy{ProductID: gauntlet.ProductNolaneSandbox, ScenarioTimeout: 3 * time.Second}
	v6, err := delegationgauntlet.RunStandard(ctx, v6Policy)
	if err != nil || !v6.Approved {
		return errors.New("v6 evidence no longer approved")
	}
	v6raw, err := gauntlet.MarshalReport(v6)
	if err != nil {
		return err
	}
	if digestBytes(v6raw) != v6EvidenceSHA256 {
		return errors.New("v6 canonical evidence drifted")
	}

	v7Policy := gauntlet.Policy{ProductID: gauntlet.ProductNolaneSandbox, ScenarioTimeout: 5 * time.Second}
	v7, err := providergauntlet.RunStandard(ctx, v7Policy)
	if err != nil || !v7.Approved {
		return errors.New("v7 evidence no longer approved")
	}
	v7raw, err := gauntlet.MarshalReport(v7)
	if err != nil || len(v7raw) == 0 {
		return errors.New("v7 canonical evidence unavailable")
	}

	return defend(p, "historical-gauntlet-release-policy-boundary", "historical-evidence-stable", "v4 and v6 matched pinned canonical release hashes under their original policies and v7 regenerated as fully verified approved evidence")
}
