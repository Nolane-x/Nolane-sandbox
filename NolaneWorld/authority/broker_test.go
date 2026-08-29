package authority

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type policyFunc func(context.Context, Intent) (Decision, error)

func (f policyFunc) Evaluate(ctx context.Context, in Intent) (Decision, error) { return f(ctx, in) }

type countingExecutor struct {
	mu     sync.Mutex
	calls  int
	result []byte
	err    error
}

func (e *countingExecutor) Execute(context.Context, Intent) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++
	return append([]byte(nil), e.result...), e.err
}
func (e *countingExecutor) Calls() int { e.mu.Lock(); defer e.mu.Unlock(); return e.calls }

func allowPolicy(context.Context, Intent) (Decision, error) { return Allow, nil }

func newBrokerForTest(t *testing.T, id string, p Policy, e Executor, ledger Ledger) (*Broker, *world.State) {
	t.Helper()
	state, err := world.NewState(world.ID(id))
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewBroker(state, p, e, ledger)
	if err != nil {
		t.Fatal(err)
	}
	return b, state
}

func intent(id string, epoch world.Epoch, payload string) Intent {
	return Intent{WorldID: "world-1", AuthorityEpoch: epoch, ActionID: id, Kind: "git.commit", Target: "repo/main", Payload: []byte(payload)}
}

func TestAuthorityExactRetryExecutesOnlyOnce(t *testing.T) {
	exec := &countingExecutor{result: []byte("commit-sha")}
	b, _ := newBrokerForTest(t, "world-1", policyFunc(allowPolicy), exec, NewMemoryLedger())
	first, err := b.Execute(context.Background(), intent("a-1", 1, "patch"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.Execute(context.Background(), intent("a-1", 1, "patch"))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Calls() != 1 {
		t.Fatalf("executor calls=%d", exec.Calls())
	}
	if first != second {
		t.Fatalf("retry receipt changed\nfirst=%#v\nsecond=%#v", first, second)
	}
}

func TestAuthorityActionIDCollisionWithChangedPayloadIsDenied(t *testing.T) {
	exec := &countingExecutor{result: []byte("ok")}
	b, _ := newBrokerForTest(t, "world-1", policyFunc(allowPolicy), exec, NewMemoryLedger())
	if _, err := b.Execute(context.Background(), intent("a-1", 1, "one")); err != nil {
		t.Fatal(err)
	}
	_, err := b.Execute(context.Background(), intent("a-1", 1, "two"))
	if !errors.Is(err, ErrActionCollision) {
		t.Fatalf("expected collision, got %v", err)
	}
	if exec.Calls() != 1 {
		t.Fatalf("collision re-executed action: %d", exec.Calls())
	}
}

func TestAuthorityRejectsStaleEpochAfterHostAdvances(t *testing.T) {
	exec := &countingExecutor{result: []byte("ok")}
	b, state := newBrokerForTest(t, "world-1", policyFunc(allowPolicy), exec, NewMemoryLedger())
	state.AdvanceEpoch()
	_, err := b.Execute(context.Background(), intent("a-1", 1, "patch"))
	if !errors.Is(err, world.ErrStaleEpoch) {
		t.Fatalf("expected stale epoch, got %v", err)
	}
	if exec.Calls() != 0 {
		t.Fatal("stale request reached executor")
	}
}

func TestAuthorityPolicyDenialAndFailureFailClosed(t *testing.T) {
	cases := []struct {
		name   string
		policy Policy
		want   error
	}{
		{"deny", policyFunc(func(context.Context, Intent) (Decision, error) { return Deny, nil }), ErrDenied},
		{"failure", policyFunc(func(context.Context, Intent) (Decision, error) { return Allow, errors.New("policy store unavailable") }), ErrPolicyFailure},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec := &countingExecutor{result: []byte("ok")}
			b, _ := newBrokerForTest(t, "world-1", tc.policy, exec, NewMemoryLedger())
			_, err := b.Execute(context.Background(), intent("a-1", 1, "patch"))
			if !errors.Is(err, tc.want) {
				t.Fatalf("want %v got %v", tc.want, err)
			}
			if exec.Calls() != 0 {
				t.Fatal("denied request reached executor")
			}
		})
	}
}

func TestAuthorityReceiptCannotBeReusedAcrossWorlds(t *testing.T) {
	ledger := NewMemoryLedger()
	exec1 := &countingExecutor{result: []byte("w1")}
	b1, _ := newBrokerForTest(t, "world-1", policyFunc(allowPolicy), exec1, ledger)
	if _, err := b1.Execute(context.Background(), intent("a-1", 1, "patch")); err != nil {
		t.Fatal(err)
	}

	exec2 := &countingExecutor{result: []byte("w2")}
	b2, _ := newBrokerForTest(t, "world-2", policyFunc(allowPolicy), exec2, ledger)
	in2 := intent("a-1", 1, "patch")
	in2.WorldID = "world-2"
	r2, err := b2.Execute(context.Background(), in2)
	if err != nil {
		t.Fatal(err)
	}
	if exec2.Calls() != 1 {
		t.Fatalf("world 2 should execute independently, calls=%d", exec2.Calls())
	}
	if r2.WorldID != "world-2" {
		t.Fatalf("wrong receipt world %q", r2.WorldID)
	}
}

func TestAuthoritySharedLedgerSerializesSameActionAcrossBrokers(t *testing.T) {
	ledger := NewMemoryLedger()
	started := make(chan struct{})
	release := make(chan struct{})
	secondCall := make(chan struct{}, 1)
	var once sync.Once
	exec := &countingExecutorWithHook{hook: func(call int) {
		if call == 1 {
			once.Do(func() { close(started) })
			<-release
		}
		if call == 2 {
			secondCall <- struct{}{}
		}
	}, result: []byte("ok")}
	b1, _ := newBrokerForTest(t, "world-1", policyFunc(allowPolicy), exec, ledger)
	b2, _ := newBrokerForTest(t, "world-1", policyFunc(allowPolicy), exec, ledger)

	type result struct {
		r   Receipt
		err error
	}
	ch1 := make(chan result, 1)
	ch2 := make(chan result, 1)
	go func() {
		r, err := b1.Execute(context.Background(), intent("shared", 1, "patch"))
		ch1 <- result{r, err}
	}()
	<-started
	go func() {
		r, err := b2.Execute(context.Background(), intent("shared", 1, "patch"))
		ch2 <- result{r, err}
	}()

	select {
	case <-secondCall:
		close(release)
		<-ch1
		<-ch2
		t.Fatal("same action executed twice across brokers")
	case <-time.After(40 * time.Millisecond):
	}
	close(release)
	one, two := <-ch1, <-ch2
	if one.err != nil || two.err != nil {
		t.Fatalf("errors: %v / %v", one.err, two.err)
	}
	if one.r != two.r {
		t.Fatalf("receipts differ: %#v / %#v", one.r, two.r)
	}
	if exec.Calls() != 1 {
		t.Fatalf("executor calls=%d", exec.Calls())
	}
}

type countingExecutorWithHook struct {
	mu     sync.Mutex
	calls  int
	hook   func(int)
	result []byte
}

func (e *countingExecutorWithHook) Execute(context.Context, Intent) ([]byte, error) {
	e.mu.Lock()
	e.calls++
	call := e.calls
	e.mu.Unlock()
	if e.hook != nil {
		e.hook(call)
	}
	return append([]byte(nil), e.result...), nil
}
func (e *countingExecutorWithHook) Calls() int { e.mu.Lock(); defer e.mu.Unlock(); return e.calls }

func TestAuthorityEpochAdvanceIsLinearizableWithInFlightExecution(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	exec := &countingExecutorWithHook{hook: func(call int) {
		if call == 1 {
			close(started)
			<-release
		}
	}, result: []byte("ok")}
	b, state := newBrokerForTest(t, "world-1", policyFunc(allowPolicy), exec, NewMemoryLedger())

	actionDone := make(chan error, 1)
	go func() {
		_, err := b.Execute(context.Background(), intent("linearizable", 1, "patch"))
		actionDone <- err
	}()
	<-started

	advanced := make(chan world.Epoch, 1)
	go func() { advanced <- state.AdvanceEpoch() }()

	select {
	case got := <-advanced:
		t.Fatalf("epoch advanced to %d while old-epoch effect was still in flight", got)
	case <-time.After(40 * time.Millisecond):
	}

	close(release)
	if err := <-actionDone; err != nil {
		t.Fatalf("in-flight action failed: %v", err)
	}
	if got := <-advanced; got != 2 {
		t.Fatalf("advanced epoch=%d", got)
	}

	if _, err := b.Execute(context.Background(), intent("after-revoke", 1, "patch")); !errors.Is(err, world.ErrStaleEpoch) {
		t.Fatalf("old epoch accepted after advance: %v", err)
	}
}
