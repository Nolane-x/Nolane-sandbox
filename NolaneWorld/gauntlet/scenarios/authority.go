package scenarios

import (
	"context"
	"errors"
	"fmt"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/authority"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type allowPolicy struct{}

func (allowPolicy) Evaluate(context.Context, authority.Intent) (authority.Decision, error) {
	return authority.Allow, nil
}

type countingExecutor struct{ calls int }

func (e *countingExecutor) Execute(context.Context, authority.Intent) ([]byte, error) {
	e.calls++
	return []byte("effect"), nil
}

func brokerFor(state world.AuthorityState, exec *countingExecutor) (*authority.Broker, error) {
	return authority.NewBroker(state, allowPolicy{}, exec, authority.NewMemoryLedger())
}

func baseIntent(id world.ID, epoch world.Epoch, actionID string, payload []byte) authority.Intent {
	return authority.Intent{WorldID: id, AuthorityEpoch: epoch, ActionID: actionID, Kind: "release-test", Target: "deterministic-target", Payload: append([]byte(nil), payload...)}
}

func StaleEpochScenario() gauntlet.Scenario {
	return gauntlet.ScenarioFunc{
		Definition: gauntlet.ScenarioSpec{
			ID: "authority.stale-epoch", Invariant: "rollback or authority advance invalidates every prior epoch", Attack: "submit an otherwise valid intent using the previous authority epoch", ExpectedDefense: "broker rejects the stale epoch before policy or executor side effects", Severity: gauntlet.SeverityCritical,
			RequiredMarkers: []string{"authority.stale.attack", "authority.stale.boundary", "authority.stale.denied", "authority.stale.executor-zero"},
		},
		Execute: func(ctx context.Context, p *gauntlet.Probe) error {
			state, err := world.NewState("gauntlet-stale")
			if err != nil {
				return err
			}
			stale := state.CurrentEpoch()
			state.AdvanceEpoch()
			exec := &countingExecutor{}
			broker, err := brokerFor(state, exec)
			if err != nil {
				return err
			}
			if err := p.Record(gauntlet.EventAttack, "authority.stale.attack", "submitted intent with prior epoch"); err != nil {
				return err
			}
			_, attackErr := broker.Execute(ctx, baseIntent(state.ID(), stale, "stale-action", []byte("payload")))
			if err := p.Record(gauntlet.EventBoundary, "authority.stale.boundary", "broker epoch boundary executed"); err != nil {
				return err
			}
			if !errors.Is(attackErr, world.ErrStaleEpoch) {
				return fmt.Errorf("stale epoch defense failed")
			}
			if exec.calls != 0 {
				return fmt.Errorf("stale epoch reached executor")
			}
			if err := p.Record(gauntlet.EventDenial, "authority.stale.denied", "stale epoch rejected"); err != nil {
				return err
			}
			return p.Record(gauntlet.EventObservation, "authority.stale.executor-zero", "executor invocation count remained zero")
		},
	}
}

func TerminalAuthorityScenario() gauntlet.Scenario {
	return gauntlet.ScenarioFunc{
		Definition: gauntlet.ScenarioSpec{
			ID: "authority.terminal-world", Invariant: "terminal authority can never be used again", Attack: "submit an intent after terminal authority close", ExpectedDefense: "broker rejects the closed world before any external effect", Severity: gauntlet.SeverityCritical,
			RequiredMarkers: []string{"authority.terminal.attack", "authority.terminal.boundary", "authority.terminal.denied", "authority.terminal.executor-zero"},
		},
		Execute: func(ctx context.Context, p *gauntlet.Probe) error {
			state, err := world.NewState("gauntlet-terminal")
			if err != nil {
				return err
			}
			epoch := state.CurrentEpoch()
			state.Close()
			exec := &countingExecutor{}
			broker, err := brokerFor(state, exec)
			if err != nil {
				return err
			}
			if err := p.Record(gauntlet.EventAttack, "authority.terminal.attack", "submitted intent after terminal close"); err != nil {
				return err
			}
			_, attackErr := broker.Execute(ctx, baseIntent(state.ID(), epoch, "terminal-action", []byte("payload")))
			if err := p.Record(gauntlet.EventBoundary, "authority.terminal.boundary", "broker terminal-authority boundary executed"); err != nil {
				return err
			}
			if !errors.Is(attackErr, world.ErrClosedWorld) {
				return fmt.Errorf("terminal authority defense failed")
			}
			if exec.calls != 0 {
				return fmt.Errorf("terminal authority reached executor")
			}
			if err := p.Record(gauntlet.EventDenial, "authority.terminal.denied", "closed world rejected"); err != nil {
				return err
			}
			return p.Record(gauntlet.EventObservation, "authority.terminal.executor-zero", "executor invocation count remained zero")
		},
	}
}

func ActionCollisionScenario() gauntlet.Scenario {
	return gauntlet.ScenarioFunc{
		Definition: gauntlet.ScenarioSpec{
			ID: "authority.action-id-rebinding", Invariant: "one action ID cannot be rebound to a materially different request", Attack: "reuse a completed action ID with different payload bytes", ExpectedDefense: "effect ledger rejects the collision and never executes the second request", Severity: gauntlet.SeverityCritical,
			RequiredMarkers: []string{"authority.collision.attack", "authority.collision.boundary", "authority.collision.denied", "authority.collision.executor-once"},
		},
		Execute: func(ctx context.Context, p *gauntlet.Probe) error {
			state, err := world.NewState("gauntlet-collision")
			if err != nil {
				return err
			}
			exec := &countingExecutor{}
			broker, err := brokerFor(state, exec)
			if err != nil {
				return err
			}
			epoch := state.CurrentEpoch()
			if _, err := broker.Execute(ctx, baseIntent(state.ID(), epoch, "same-action", []byte("A"))); err != nil {
				return err
			}
			if err := p.Record(gauntlet.EventAttack, "authority.collision.attack", "reused action ID with different payload"); err != nil {
				return err
			}
			_, attackErr := broker.Execute(ctx, baseIntent(state.ID(), epoch, "same-action", []byte("B")))
			if err := p.Record(gauntlet.EventBoundary, "authority.collision.boundary", "effect ledger collision boundary executed"); err != nil {
				return err
			}
			if !errors.Is(attackErr, authority.ErrActionCollision) {
				return fmt.Errorf("action collision defense failed")
			}
			if exec.calls != 1 {
				return fmt.Errorf("colliding request changed executor count")
			}
			if err := p.Record(gauntlet.EventDenial, "authority.collision.denied", "rebound action rejected"); err != nil {
				return err
			}
			return p.Record(gauntlet.EventObservation, "authority.collision.executor-once", "executor invocation count remained exactly one")
		},
	}
}
