package freedomgauntlet

import (
	"context"
	"sort"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet"
)

const SyntheticSecret = "SYNTHETIC-V8-CREDENTIAL"

func Standard() []gauntlet.Scenario {
	suite := []gauntlet.Scenario{
		freedomScenario("freedom.authority-noninheritance", "Internal Realm capability never inherits Reality authority.", "Exercise an N3 Reality crossing from a Realm capability context.", "The Reality Membrane requires delegated typed-provider authority and grants no ambient credential or ingress authority.", "reality-authority-not-inherited", authorityNoninheritance),
		freedomScenario("freedom.agent-projection-secret-free", "Agent-facing semantic projections expose no provider handles, credentials, or substrate authority.", "Inspect the complete agent-facing runtime projection surface for authority-bearing fields.", "Only semantic IDs, receipts, claims, and bounded observations remain agent-visible.", "agent-projection-secret-free", agentProjectionSecretFree),
		freedomScenario("freedom.realm-policy-host-only", "Realm policy identity and aggregate admission remain host-owned.", "Inspect the Runtime for policy administration, attempt a forged policy digest, and exceed the Realm budget while host capacity remains available.", "The Runtime exposes no Realm administration, Enter binds the host-derived policy digest, and Fabric denies aggregate Realm-budget broadening before realization.", "realm-policy-broadening-denied", realmPolicyHostOnly),
		freedomScenario("freedom.acquire-idempotency", "Acquire is idempotent for one operation identity.", "Replay an identical Acquire request after a successful realization.", "The Fabric returns the same lease and does not create a second realization.", "acquire-idempotent", acquireIdempotency),
		freedomScenario("freedom.acquire-collision", "An Acquire operation ID cannot be rebound to a different request.", "Reuse one completed operation ID with changed resource units.", "The Fabric rejects the request digest collision before another realization is created.", "acquire-collision-denied", acquireCollision),
		freedomScenario("freedom.stale-lease-denial", "A newer lease generation fences older authority.", "Issue a second lease generation and validate the first.", "The LeaseBook rejects the old generation as stale.", "stale-lease-denied", staleLeaseDenial),
		freedomScenario("freedom.terminal-world-denial", "A terminal World cannot regain a host realization through an old lease.", "Release a leased World and then request its host handle with the former generation.", "The Fabric reports the World terminal and exposes no handle.", "terminal-world-denied", terminalWorldDenial),
		freedomScenario("freedom.checkpoint-authority-nonrewind", "Checkpoint rollback restores guest state without rewinding host authority.", "Checkpoint a World, then resume the snapshot.", "Rollback advances host authority, realization revision, and lease generation while invalidating the old lease.", "checkpoint-authority-advanced", checkpointAuthorityNonrewind),
		freedomScenario("freedom.baseline-identity-isolation", "Reusable baselines are sanitized and identity-free.", "Attempt to admit baselines carrying World identity or checkpoint ownership.", "The catalog rejects identity-bearing baselines and accepts only sanitized identity-free material.", "baseline-identity-denied", baselineIdentityIsolation),
		freedomScenario("freedom.baseline-fresh-create", "A reusable baseline never turns one realized World into another World.", "Acquire two distinct World IDs while the same sanitized baseline is selected.", "The Fabric performs two fresh exact-World creates and records distinct host handles.", "baseline-fresh-create", baselineFreshCreate),
		freedomScenario("freedom.profile-no-public-ingress", "No Realm network profile enables public inbound traffic or ambient credentials.", "Plan every R0-R2 Realm network profile.", "Every plan keeps public ingress and ambient credentials disabled; R1/R2 require the governed Reality gateway.", "profile-ingress-denied", profileNoPublicIngress),
		freedomScenario("freedom.profile-no-n3-n5", "Realm profiles cannot directly grant authenticated or consequential Reality authority.", "Classify N3-N5 crossings as though internal capability requested them.", "Every N3-N5 crossing is routed to delegated typed-provider authority rather than Realm networking.", "n3-n5-delegated", profileNoN3N5),
		freedomScenario("freedom.service-generation-stale", "Internal service readiness is realization-revision fenced.", "Advance a service-owning World realization after publishing readiness.", "The previous service generation becomes non-current until re-registered against the new realization.", "stale-service-not-ready", serviceGenerationStale),
		freedomScenario("freedom.capability-fail-honest", "Capability availability is never upgraded into verification without evidence.", "Advertise available/verified provider booleans while withholding evidence.", "Capability projection reports only available-unproven or unavailable claims.", "capability-not-falsely-verified", capabilityFailHonest),
		freedomScenario("freedom.persistence-tamper", "Durable Realm history is strict hash-chained evidence.", "Modify persisted Realm journal bytes after clean shutdown.", "Recovery rejects the journal as corrupt rather than trusting altered history.", "persistence-tamper-denied", persistenceTamper),
		freedomScenario("freedom.restart-no-false-ready", "Host restart never treats historical realization handles or services as currently ready.", "Replay a leased World and ready service, then apply the explicit Fabric recovery fence.", "The in-memory recovery projection removes the stale handle and readiness without rewriting durable history.", "restart-readiness-fenced", restartNoFalseReady),
		freedomScenario("freedom.exec-bounded-output", "Agent execution is bounded before guest authority is entered and returns bounded observations.", "Execute with a small output budget and then request an output budget above the runtime maximum.", "The valid execution remains bounded and the oversized request is rejected before guest execution.", "exec-output-bounded", execBoundedOutput),
		freedomScenario("freedom.exec-uncertain-not-success", "An uncertain guest execution outcome is never converted into success or replayed automatically.", "Lose the guest execution result and submit the same action again.", "The operation remains uncertain and guest execution is entered only once.", "exec-uncertain-quarantined", execUncertainNotSuccess),
		freedomScenario("freedom.v4-v6-v7-nondrift", "Freedom Plane work cannot silently weaken established v4, v6, or v7 release evidence.", "Regenerate historical deterministic authority suites under their release policy.", "v4/v6 canonical hashes remain pinned and v7 remains fully approved under the unchanged runner contract.", "historical-evidence-stable", historicalNondriftCanonical),
		freedomScenario("freedom.v5-unavailable-not-pass", "Missing live substrate evidence is UNAVAILABLE, never PASS.", "Run the v5 live substrate probe without a configured live driver.", "The report is explicitly UNAVAILABLE and unapproved.", "unavailable-not-pass", v5UnavailableNotPass),
	}
	sort.Slice(suite, func(i, j int) bool { return suite[i].Spec().ID < suite[j].Spec().ID })
	return suite
}

func RunStandard(ctx context.Context, policy gauntlet.Policy) (gauntlet.Report, error) {
	return gauntlet.NewRunner(policy).Run(ctx, Standard())
}

func freedomScenario(id, invariant, attack, defense, marker string, fn func(context.Context, *gauntlet.Probe) error) gauntlet.Scenario {
	return gauntlet.ScenarioFunc{Definition: gauntlet.ScenarioSpec{ID: id, Invariant: invariant, Attack: attack, ExpectedDefense: defense, Severity: gauntlet.SeverityCritical, RequiredMarkers: []string{marker}}, Execute: fn}
}
