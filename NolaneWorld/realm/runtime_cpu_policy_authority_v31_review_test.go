package realm

import (
	"context"
	"errors"
	"testing"
)

type v31PackageInternalStoreWrapper struct {
	*MemoryStore
}

func (*v31PackageInternalStoreWrapper) packageOwnedRuntimeCPUPolicyStore() {}

func TestV31BindingRejectsNonExactPackageStore(t *testing.T) {
	store, ctl, rec := seedV31MemoryRealm(t, 500)
	authority, err := ctl.CurrentRuntimeCPUPolicyAuthority(context.Background(), rec.Spec.ID)
	if err != nil {
		t.Fatal(err)
	}

	authority.store = &v31PackageInternalStoreWrapper{MemoryStore: store}
	if binding, ok := authority.Binding(); ok {
		t.Fatalf("authority with non-exact bound Store remained structurally valid: %+v", binding)
	}
	if _, err := ctl.ValidateRuntimeCPUPolicyAuthority(context.Background(), authority); !errors.Is(err, ErrInvalidRuntimeCPUPolicyAuthority) {
		t.Fatalf("authority with non-exact bound Store err=%v, want invalid", err)
	}
}
