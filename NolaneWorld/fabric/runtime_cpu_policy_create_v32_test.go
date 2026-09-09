package fabric

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type v32PolicyManager struct {
	*fakeManager
	policyCreates int
	beforeReturn  func()
	mutateReceipt func(*substrate.RuntimeCPUPolicyCreatePropagation)
}

func newV32PolicyManager() *v32PolicyManager {
	return &v32PolicyManager{fakeManager: newFakeManager()}
}

func (m *v32PolicyManager) CreateWithRuntimeCPUPolicy(_ context.Context, id world.ID, authority realm.RuntimeCPUPolicyAuthority) (substrate.Handle, substrate.RuntimeCPUPolicyCreatePropagation, error) {
	m.policyCreates++
	binding, ok := authority.Binding()
	if !ok {
		return "", substrate.RuntimeCPUPolicyCreatePropagation{}, errors.New("invalid Wave31 authority")
	}
	state, err := world.NewState(id)
	if err != nil {
		return "", substrate.RuntimeCPUPolicyCreatePropagation{}, err
	}
	m.states[id] = state
	receipt := substrate.RuntimeCPUPolicyCreatePropagation{
		RealmID:         string(binding.RealmID),
		RealmRevision:   binding.RealmRevision,
		PolicyDigest:    binding.PolicyDigest,
		LimitMilliCPU:   binding.LimitMilliCPU,
		AuthorityDigest: binding.Digest,
		WorldID:         id,
		RequestDigest:   substrate.RuntimeCPUPolicyCreatePropagationDigestPrefix + strings.Repeat("a", 64),
	}
	if m.mutateReceipt != nil {
		m.mutateReceipt(&receipt)
	}
	if m.beforeReturn != nil {
		m.beforeReturn()
	}
	return substrate.Handle("handle-" + string(id)), receipt, nil
}

func newV32Local(t *testing.T, manager WorldManager, limit uint64) (*Local, *realm.MemoryStore, realm.Spec) {
	t.Helper()
	store := realm.NewMemoryStore()
	spec := realm.Spec{
		ID:                      realm.ID("realm://wave32"),
		MaxWorlds:               4,
		DefaultLease:            time.Minute,
		NetworkProfile:          realm.R0InternalOnly,
		ResourceBudget:          realm.ResourceBudget{CPUUnits: 4, MemoryMiB: 4096, DiskMiB: 8192},
		RuntimeCPULimitMilliCPU: limit,
	}
	if _, err := store.CreateRealm(spec); err != nil {
		t.Fatal(err)
	}
	cap := NewCapacity()
	cap.Observe(spec.ResourceBudget)
	local, err := NewLocal(store, manager, cap, NewLeaseBook(), NewBaselineCatalog())
	if err != nil {
		t.Fatal(err)
	}
	return local, store, spec
}

func v32AcquireRequest(spec realm.Spec, id world.ID, op string) AcquireRequest {
	return AcquireRequest{
		RealmID:       spec.ID,
		RealmRevision: 1,
		WorldID:       id,
		OperationID:   op,
		Units:         realm.ResourceBudget{CPUUnits: 1, MemoryMiB: 512, DiskMiB: 1024},
		ExpiresUnix:   time.Now().Add(time.Minute).Unix(),
	}
}

func TestV32PositivePolicyUsesOnlySealedPropagationPath(t *testing.T) {
	manager := newV32PolicyManager()
	local, _, spec := newV32Local(t, manager, 750)
	lease, err := local.Acquire(context.Background(), v32AcquireRequest(spec, world.ID("world-v32"), "op-v32"))
	if err != nil {
		t.Fatal(err)
	}
	if lease.WorldID != world.ID("world-v32") {
		t.Fatalf("lease=%+v", lease)
	}
	if manager.policyCreates != 1 {
		t.Fatalf("policy creates=%d want 1", manager.policyCreates)
	}
	if len(manager.creates) != 0 {
		t.Fatalf("positive policy used legacy Create: %v", manager.creates)
	}
}

func TestV32PositivePolicyFailsClosedWithoutTypedManagerCapability(t *testing.T) {
	manager := newFakeManager()
	local, _, spec := newV32Local(t, manager, 750)
	_, err := local.Acquire(context.Background(), v32AcquireRequest(spec, world.ID("world-v32-no-cap"), "op-v32-no-cap"))
	if !errors.Is(err, ErrRuntimeCPUPolicyPropagationUnavailable) {
		t.Fatalf("err=%v want ErrRuntimeCPUPolicyPropagationUnavailable", err)
	}
	if len(manager.creates) != 0 {
		t.Fatalf("fail-closed path called legacy Create: %v", manager.creates)
	}
}

func TestV32ReceiptMismatchCannotCompleteAcquire(t *testing.T) {
	manager := newV32PolicyManager()
	manager.mutateReceipt = func(receipt *substrate.RuntimeCPUPolicyCreatePropagation) {
		receipt.WorldID = world.ID("wrong-world")
	}
	local, _, spec := newV32Local(t, manager, 750)
	_, err := local.Acquire(context.Background(), v32AcquireRequest(spec, world.ID("world-v32-mismatch"), "op-v32-mismatch"))
	if !errors.Is(err, ErrOutcomeUncertain) {
		t.Fatalf("err=%v want ErrOutcomeUncertain", err)
	}
}

func TestV32RealmDriftAcrossProviderCreateCannotYieldSuccessfulLease(t *testing.T) {
	manager := newV32PolicyManager()
	local, store, spec := newV32Local(t, manager, 750)
	manager.beforeReturn = func() {
		updated := spec
		updated.RuntimeCPULimitMilliCPU = 800
		if _, err := store.UpdateRealm(spec.ID, 1, updated); err != nil {
			t.Fatalf("UpdateRealm: %v", err)
		}
	}
	_, err := local.Acquire(context.Background(), v32AcquireRequest(spec, world.ID("world-v32-drift"), "op-v32-drift"))
	if !errors.Is(err, ErrOutcomeUncertain) {
		t.Fatalf("err=%v want ErrOutcomeUncertain", err)
	}
}

func TestV32ZeroPolicyPreservesLegacyCreatePath(t *testing.T) {
	manager := newFakeManager()
	local, _, spec := newV32Local(t, manager, 0)
	if _, err := local.Acquire(context.Background(), v32AcquireRequest(spec, world.ID("world-v32-legacy"), "op-v32-legacy")); err != nil {
		t.Fatal(err)
	}
	if len(manager.creates) != 1 || manager.creates[0] != world.ID("world-v32-legacy") {
		t.Fatalf("legacy creates=%v", manager.creates)
	}
}
