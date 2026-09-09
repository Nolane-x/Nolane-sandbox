package cube

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func v32RuntimeCPUPolicyAuthority(t *testing.T, limit uint64) (realm.RuntimeCPUPolicyAuthority, realm.RuntimeCPUPolicyBinding) {
	t.Helper()
	store := realm.NewMemoryStore()
	spec := realm.Spec{
		ID:                      realm.ID("realm://cube-v32"),
		MaxWorlds:               2,
		DefaultLease:            time.Minute,
		NetworkProfile:          realm.R0InternalOnly,
		ResourceBudget:          realm.ResourceBudget{CPUUnits: 4, MemoryMiB: 4096, DiskMiB: 8192},
		RuntimeCPULimitMilliCPU: limit,
	}
	if _, err := store.CreateRealm(spec); err != nil {
		t.Fatal(err)
	}
	controller, err := realm.NewController(store)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := controller.CurrentRuntimeCPUPolicyAuthority(context.Background(), spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	binding, ok := authority.Binding()
	if !ok {
		t.Fatal("minted Wave31 authority has no valid binding")
	}
	return authority, binding
}

func TestV32CubeCreatePropagatesExactWave31BindingIntoProviderBody(t *testing.T) {
	authority, binding := v32RuntimeCPUPolicyAuthority(t, 750)
	var rawBody []byte
	var requests int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost || r.URL.Path != "/sandboxes" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var err error
		rawBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"sandboxID":"sb-v32"}`)
	}))
	defer ts.Close()

	client, err := New(Config{APIURL: ts.URL, TemplateID: "tpl-v32"})
	if err != nil {
		t.Fatal(err)
	}
	handle, receipt, err := client.CreateWithRuntimeCPUPolicy(context.Background(), world.ID("world-v32"), authority)
	if err != nil {
		t.Fatal(err)
	}
	if handle != substrate.Handle("sb-v32") || requests != 1 {
		t.Fatalf("handle=%q requests=%d", handle, requests)
	}
	var body map[string]any
	if err := json.Unmarshal(rawBody, &body); err != nil {
		t.Fatal(err)
	}
	metadata, ok := body["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata=%T %v", body["metadata"], body["metadata"])
	}
	wantMetadata := map[string]string{
		"nolane.world.id":                         "world-v32",
		"nolane.realm.id":                         string(binding.RealmID),
		"nolane.realm.revision":                   strconv.FormatUint(binding.RealmRevision, 10),
		"nolane.realm.policy_digest":              binding.PolicyDigest,
		"nolane.realm.runtime_cpu_policy_digest":  binding.Digest,
		"nolane.realm.runtime_cpu_limit_millicpu": strconv.FormatUint(binding.LimitMilliCPU, 10),
	}
	for key, want := range wantMetadata {
		if got := metadata[key]; got != want {
			t.Fatalf("metadata[%q]=%v want %q; metadata=%v", key, got, want, metadata)
		}
	}
	if receipt.RealmID != string(binding.RealmID) || receipt.RealmRevision != binding.RealmRevision || receipt.PolicyDigest != binding.PolicyDigest || receipt.LimitMilliCPU != binding.LimitMilliCPU || receipt.AuthorityDigest != binding.Digest || receipt.WorldID != world.ID("world-v32") {
		t.Fatalf("receipt=%+v binding=%+v", receipt, binding)
	}
	h := sha256.Sum256(append([]byte("nolane.runtime-cpu-policy-create-propagation.v32\x00"), rawBody...))
	wantDigest := substrate.RuntimeCPUPolicyCreatePropagationDigestPrefix + hex.EncodeToString(h[:])
	if receipt.RequestDigest != wantDigest {
		t.Fatalf("request digest=%q want %q", receipt.RequestDigest, wantDigest)
	}
	if !receipt.Valid() {
		t.Fatalf("receipt not structurally valid: %+v", receipt)
	}
}

func TestV32CubeRejectsReconstructedAuthorityBeforeHTTP(t *testing.T) {
	var requests int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	client, err := New(Config{APIURL: ts.URL, TemplateID: "tpl-v32"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = client.CreateWithRuntimeCPUPolicy(context.Background(), world.ID("world-v32-invalid"), realm.RuntimeCPUPolicyAuthority{})
	if !errors.Is(err, ErrInvalidRuntimeCPUPolicyCreateAuthority) {
		t.Fatalf("err=%v want ErrInvalidRuntimeCPUPolicyCreateAuthority", err)
	}
	if requests != 0 {
		t.Fatalf("invalid authority reached HTTP: requests=%d", requests)
	}
}
