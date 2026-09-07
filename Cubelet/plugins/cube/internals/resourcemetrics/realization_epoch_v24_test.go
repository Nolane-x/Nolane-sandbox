package resourcemetrics

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

type v24EpochVisitor struct {
	sandboxID  string
	generation uint64
	token      [32]byte
}

func (v v24EpochVisitor) VisitRealizationEpochs(visit func(string, uint64, [32]byte)) {
	visit(v.sandboxID, v.generation, v.token)
}

func gatherV24Epoch(t *testing.T, visitor realizationEpochVisitor) string {
	t.Helper()
	registry := prometheus.NewRegistry()
	registry.MustRegister(&realizationEpochPrometheusCollector{epochs: visitor})
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	encoder := expfmt.NewEncoder(&out, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, family := range families {
		if err := encoder.Encode(family); err != nil {
			t.Fatal(err)
		}
	}
	return out.String()
}

func TestV24RealizationEpochPrometheusUsesCanonicalExactTuple(t *testing.T) {
	var token [32]byte
	for i := range token {
		token[i] = byte(i + 1)
	}
	body := gatherV24Epoch(t, v24EpochVisitor{sandboxID: "sandbox-v24", generation: 7, token: token})
	wantToken := hex.EncodeToString(token[:])
	want := `cubesandbox_realization_epoch_info{generation="7",sandbox_id="sandbox-v24",token="` + wantToken + `"} 1`
	if !strings.Contains(body, want) {
		t.Fatalf("epoch metrics missing exact canonical tuple\nwant substring: %s\nbody:\n%s", want, body)
	}
}

func TestV24RealizationEpochPrometheusDropsInvalidAuthority(t *testing.T) {
	cases := []v24EpochVisitor{
		{sandboxID: "", generation: 1, token: [32]byte{1}},
		{sandboxID: " sandbox ", generation: 1, token: [32]byte{1}},
		{sandboxID: "sandbox", generation: 0, token: [32]byte{1}},
		{sandboxID: "sandbox", generation: 1, token: [32]byte{}},
	}
	for _, tc := range cases {
		if body := gatherV24Epoch(t, tc); strings.Contains(body, "cubesandbox_realization_epoch_info") {
			t.Fatalf("invalid authority exported: %+v\n%s", tc, body)
		}
	}
}
