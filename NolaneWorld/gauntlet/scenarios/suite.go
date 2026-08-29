package scenarios

import (
	"context"
	"sort"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet"
)

func StandardSuite() []gauntlet.Scenario {
	suite := []gauntlet.Scenario{
		StaleEpochScenario(), TerminalAuthorityScenario(), ActionCollisionScenario(),
		ArtifactTraversalScenario(), CapabilityBlobTamperScenario(), CapabilityJournalTamperScenario(),
	}
	sort.Slice(suite, func(i, j int) bool { return suite[i].Spec().ID < suite[j].Spec().ID })
	return suite
}

func RunStandard(ctx context.Context, policy gauntlet.Policy) (gauntlet.Report, error) {
	return gauntlet.NewRunner(policy).Run(ctx, StandardSuite())
}
