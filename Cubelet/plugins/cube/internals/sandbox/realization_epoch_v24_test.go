package sandbox

import "testing"

func TestV24RealizationEpochDoesNotAliasAcrossClear(t *testing.T) {
	store := newTaskOutcomeProofStore()

	first, err := store.BeginRealizationEpoch("sandbox-reused")
	if err != nil {
		t.Fatalf("first BeginRealizationEpoch: %v", err)
	}
	if first.Generation != 1 {
		t.Fatalf("first generation=%d, want 1", first.Generation)
	}
	if first.Token == ([32]byte{}) {
		t.Fatal("first realization epoch token is zero")
	}
	if !store.IsCurrentRealizationEpoch("sandbox-reused", first.Generation, first.Token) {
		t.Fatal("first exact realization epoch is not current")
	}

	store.Clear("sandbox-reused")
	if store.IsCurrentRealizationEpoch("sandbox-reused", first.Generation, first.Token) {
		t.Fatal("cleared realization epoch remained current")
	}

	second, err := store.BeginRealizationEpoch("sandbox-reused")
	if err != nil {
		t.Fatalf("second BeginRealizationEpoch: %v", err)
	}
	if second.Generation != 1 {
		t.Fatalf("post-Clear generation=%d, want reset generation 1", second.Generation)
	}
	if second.Token == ([32]byte{}) {
		t.Fatal("second realization epoch token is zero")
	}
	if second.Token == first.Token {
		t.Fatal("post-Clear realization reused the previous epoch token")
	}
	if store.IsCurrentRealizationEpoch("sandbox-reused", first.Generation, first.Token) {
		t.Fatal("old epoch resurrected after same sandbox/generation reuse")
	}
	if !store.IsCurrentRealizationEpoch("sandbox-reused", second.Generation, second.Token) {
		t.Fatal("second exact realization epoch is not current")
	}
	if store.IsCurrentRealizationEpoch("sandbox-reused", second.Generation, [32]byte{}) {
		t.Fatal("zero token was accepted as current realization epoch")
	}
}

func TestV24NextGenerationInvalidatesPriorEpoch(t *testing.T) {
	store := newTaskOutcomeProofStore()
	first, err := store.BeginRealizationEpoch("sandbox-next")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.BeginRealizationEpoch("sandbox-next")
	if err != nil {
		t.Fatal(err)
	}
	if second.Generation != first.Generation+1 {
		t.Fatalf("second generation=%d, first=%d", second.Generation, first.Generation)
	}
	if second.Token == first.Token {
		t.Fatal("new generation reused prior realization epoch token")
	}
	if store.IsCurrentRealizationEpoch("sandbox-next", first.Generation, first.Token) {
		t.Fatal("prior generation epoch remained current")
	}
	if !store.IsCurrentRealizationEpoch("sandbox-next", second.Generation, second.Token) {
		t.Fatal("new generation epoch is not current")
	}
}
