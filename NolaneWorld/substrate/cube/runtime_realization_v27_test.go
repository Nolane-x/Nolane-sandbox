package cube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const v27TokenA = "1111111111111111111111111111111111111111111111111111111111111111"
const v27TokenB = "2222222222222222222222222222222222222222222222222222222222222222"

func v27RuntimeMetrics(sandboxID string, generation uint64, token string, pid uint32, start uint64, bootID string) string {
	return fmt.Sprintf(
		"cubesandbox_realization_epoch_info{sandbox_id=%q,generation=%q,token=%q} 1\n"+
			"cubesandbox_host_process_identity_info{sandbox_id=%q,generation=%q,host_pid=%q,starttime_ticks=%q,boot_id=%q,cgroup_path=%q,runtime_role=%q,source=%q,placed_at=%q,bound_at=%q} 1\n",
		sandboxID, fmt.Sprintf("%d", generation), token,
		sandboxID, fmt.Sprintf("%d", generation), fmt.Sprintf("%d", pid), fmt.Sprintf("%d", start), bootID,
		"/cubes/"+sandboxID, HostSandboxProcessRuntimeRoleCubeShimVMM, HostSandboxProcessIdentitySourceCubeBoxAddProc,
		"2026-09-08T10:00:00Z", "2026-09-08T10:00:01Z",
	)
}

type v27MutableMetrics struct{ body string }

func (m *v27MutableMetrics) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(m.body))
}

func newV27Observer(t *testing.T, body string) (*RuntimeRealizationObserver, *v27MutableMetrics) {
	t.Helper()
	m := &v27MutableMetrics{body: body}
	ts := httptest.NewServer(m)
	t.Cleanup(ts.Close)
	o, err := NewRuntimeRealizationObserver(RuntimeRealizationConfig{BaseURL: ts.URL, HTTPClient: ts.Client()})
	if err != nil {
		t.Fatalf("NewRuntimeRealizationObserver: %v", err)
	}
	return o, m
}

func v27Binding(id string) ResourceBinding { return ResourceBinding{sandboxID: id} }

func TestRuntimeRealizationV27ObserveExactSameScrape(t *testing.T) {
	const sandboxID = "sb-v27"
	o, _ := newV27Observer(t, v27RuntimeMetrics(sandboxID, 7, v27TokenA, 321, 9001, "11111111-1111-4111-8111-111111111111"))
	proof, err := o.Observe(context.Background(), v27Binding(sandboxID))
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !proof.Valid() {
		t.Fatal("same-scrape proof is invalid")
	}
	epoch, ok := proof.Epoch()
	if !ok {
		t.Fatal("missing epoch")
	}
	epochSandboxID, sandboxOK := epoch.SandboxID()
	epochGeneration, generationOK := epoch.Generation()
	epochToken, tokenOK := epoch.TokenHex()
	if !sandboxOK || !generationOK || !tokenOK || epochSandboxID != sandboxID || epochGeneration != 7 || epochToken != v27TokenA {
		t.Fatalf("unexpected epoch: %#v", epoch)
	}
	process, ok := proof.ProcessIdentity()
	if !ok || process.SandboxID != sandboxID || process.Generation != 7 || process.HostPID != 321 || process.StartTimeTicks != 9001 {
		t.Fatalf("unexpected process identity: %#v, ok=%v", process, ok)
	}
}

func TestRuntimeRealizationV27FailsClosedOnMissingDuplicateAndMismatch(t *testing.T) {
	const sandboxID = "sb-v27"
	valid := v27RuntimeMetrics(sandboxID, 7, v27TokenA, 321, 9001, "11111111-1111-4111-8111-111111111111")
	lines := strings.Split(strings.TrimSpace(valid), "\n")
	cases := map[string]string{
		"missing epoch": lines[1] + "\n",
		"duplicate epoch": lines[0] + "\n" + lines[0] + "\n" + lines[1] + "\n",
		"missing process": lines[0] + "\n",
		"duplicate process": lines[0] + "\n" + lines[1] + "\n" + lines[1] + "\n",
		"generation mismatch": strings.Replace(valid, `generation="7",host_pid`, `generation="8",host_pid`, 1),
		"zero token": strings.Replace(valid, v27TokenA, strings.Repeat("0", 64), 1),
		"malformed process": strings.Replace(valid, `host_pid="321"`, `host_pid="0"`, 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			o, _ := newV27Observer(t, body)
			if _, err := o.Observe(context.Background(), v27Binding(sandboxID)); err == nil {
				t.Fatal("expected fail-closed observation")
			}
		})
	}
}

func TestRuntimeRealizationV27ProofOpacityContextAndFreshness(t *testing.T) {
	const sandboxID = "sb-v27"
	bodyA := v27RuntimeMetrics(sandboxID, 7, v27TokenA, 321, 9001, "11111111-1111-4111-8111-111111111111")
	o, mutable := newV27Observer(t, bodyA)
	proof, err := o.Observe(context.Background(), v27Binding(sandboxID))
	if err != nil {
		t.Fatal(err)
	}

	var zero RuntimeRealizationProof
	if zero.Valid() {
		t.Fatal("zero proof became valid")
	}
	raw, err := json.Marshal(proof)
	if err != nil {
		t.Fatal(err)
	}
	var restored RuntimeRealizationProof
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Valid() {
		t.Fatal("serialized proof restored authority")
	}

	o2, _ := newV27Observer(t, bodyA)
	if err := o2.ValidateCurrent(context.Background(), v27Binding(sandboxID), proof); err == nil {
		t.Fatal("cross-observer proof validated")
	}

	mutable.body = v27RuntimeMetrics(sandboxID, 7, v27TokenB, 321, 9001, "11111111-1111-4111-8111-111111111111")
	if err := o.ValidateCurrent(context.Background(), v27Binding(sandboxID), proof); err == nil {
		t.Fatal("old proof survived epoch rotation")
	}

	mutable.body = v27RuntimeMetrics(sandboxID, 7, v27TokenA, 322, 9002, "11111111-1111-4111-8111-111111111111")
	if err := o.ValidateCurrent(context.Background(), v27Binding(sandboxID), proof); err == nil {
		t.Fatal("old proof survived process replacement")
	}

	mutable.body = v27RuntimeMetrics(sandboxID, 7, v27TokenA, 321, 9001, "22222222-2222-4222-8222-222222222222")
	if err := o.ValidateCurrent(context.Background(), v27Binding(sandboxID), proof); err == nil {
		t.Fatal("old proof survived host boot change")
	}
}
