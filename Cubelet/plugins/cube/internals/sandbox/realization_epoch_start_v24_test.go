package sandbox

import (
	"context"
	"errors"
	"testing"
)

func TestV24StartMintsEpochAtAuthoritativeBeginSeam(t *testing.T) {
	var token [32]byte
	for i := range token {
		token[i] = byte(i + 1)
	}
	controller := &controllerLocal{
		realizationEpochTokenGenerator: func() ([32]byte, error) {
			return token, nil
		},
	}

	if _, err := controller.Start(context.Background(), "sandbox-v24-start"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	store := controller.ensureTaskOutcomeProofStore()
	epoch, ok := store.CurrentRealizationEpoch("sandbox-v24-start")
	if !ok {
		t.Fatal("Start did not create current realization epoch authority")
	}
	if epoch.Generation != 1 || epoch.Token != token {
		t.Fatalf("Start epoch=%+v, want generation=1 exact injected token", epoch)
	}
}

func TestV24StartFailsClosedWhenEpochMintFails(t *testing.T) {
	controller := &controllerLocal{
		realizationEpochTokenGenerator: func() ([32]byte, error) {
			return [32]byte{}, errors.New("rng unavailable")
		},
	}

	if _, err := controller.Start(context.Background(), "sandbox-v24-fail"); err == nil {
		t.Fatal("Start succeeded after realization epoch mint failure")
	}
	store := controller.ensureTaskOutcomeProofStore()
	store.mu.RLock()
	generation := store.generations["sandbox-v24-fail"]
	store.mu.RUnlock()
	if generation != 0 {
		t.Fatalf("mint failure advanced Wave17 generation to %d", generation)
	}
	if _, ok := store.CurrentRealizationEpoch("sandbox-v24-fail"); ok {
		t.Fatal("mint failure left partial Wave24 epoch authority")
	}
}

func TestV24StartRejectsZeroEpochTokenWithoutLifecycleMutation(t *testing.T) {
	controller := &controllerLocal{
		realizationEpochTokenGenerator: func() ([32]byte, error) {
			return [32]byte{}, nil
		},
	}

	if _, err := controller.Start(context.Background(), "sandbox-v24-zero"); err == nil {
		t.Fatal("Start accepted a zero realization epoch token")
	}
	store := controller.ensureTaskOutcomeProofStore()
	store.mu.RLock()
	generation := store.generations["sandbox-v24-zero"]
	store.mu.RUnlock()
	if generation != 0 {
		t.Fatalf("zero token advanced Wave17 generation to %d", generation)
	}
}
