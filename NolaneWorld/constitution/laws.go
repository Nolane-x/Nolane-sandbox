package constitution

import (
	"errors"
	"fmt"
)

type Law struct {
	ID    string
	Title string
	Rule  string
}

var catalog = []Law{
	{ID: "NS-LAW-001", Title: "No ambient secrets", Rule: "Raw host and external-service credentials must not be stored in guest-visible state."},
	{ID: "NS-LAW-002", Title: "Authority cannot self-increase", Rule: "An agent may request authority-bound actions but cannot mint, widen, or extend its own authority."},
	{ID: "NS-LAW-003", Title: "No self-promotion", Rule: "A world that creates a capability cannot make that capability trusted."},
	{ID: "NS-LAW-004", Title: "Rollback does not roll back reality", Rule: "Guest rollback cannot rewind authority epochs, effect receipts, revocation state, or completed external effects."},
	{ID: "NS-LAW-005", Title: "Execution state is not trusted memory", Rule: "Snapshots, volumes, and paused worlds are execution state, not trusted capability memory."},
	{ID: "NS-LAW-006", Title: "Consequential writes are typed intents", Rule: "Consequential external writes must pass through typed authority mediation."},
	{ID: "NS-LAW-007", Title: "Exports are untrusted", Rule: "Guest-originating artifacts remain untrusted until accepted by the Artifact Gate."},
	{ID: "NS-LAW-008", Title: "Exact-content binding", Rule: "Trust receipts bind exact protected payload content by cryptographic digest and identity context."},
	{ID: "NS-LAW-009", Title: "Monotonic authority epoch", Rule: "Authority epochs are host-owned, positive, monotonic, and never restored from guest snapshots."},
	{ID: "NS-LAW-010", Title: "Network authority is classified", Rule: "Network access uses explicit classes and unknown classes fail closed."},
	{ID: "NS-LAW-011", Title: "Host mounts denied by default", Rule: "Arbitrary host filesystem mounts are denied unless separately authorized."},
	{ID: "NS-LAW-012", Title: "Ambiguity fails closed", Rule: "Malformed, missing, stale, conflicting, or uncertain security state is denied rather than implicitly permitted."},
}

func Laws() []Law {
	out := make([]Law, len(catalog))
	copy(out, catalog)
	return out
}

func Validate() error { return validateLaws(catalog) }

func validateLaws(laws []Law) error {
	if len(laws) == 0 {
		return errors.New("constitution: empty law catalog")
	}
	seen := make(map[string]struct{}, len(laws))
	for _, law := range laws {
		if law.ID == "" || law.Title == "" || law.Rule == "" {
			return errors.New("constitution: incomplete law")
		}
		if _, ok := seen[law.ID]; ok {
			return fmt.Errorf("constitution: duplicate law id %s", law.ID)
		}
		seen[law.ID] = struct{}{}
	}
	return nil
}

func lawID(n int) string { return fmt.Sprintf("NS-LAW-%03d", n) }
