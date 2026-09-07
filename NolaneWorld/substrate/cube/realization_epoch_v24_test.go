package cube

import (
	"strings"
	"testing"
)

func v24EpochMetric(sandboxID, generation, token, value string) string {
	return `cubesandbox_realization_epoch_info{sandbox_id="` + sandboxID + `",generation="` + generation + `",token="` + token + `"} ` + value + "\n"
}

func TestV24ExactRealizationEpochMetricMintsSealedProof(t *testing.T) {
	token := strings.Repeat("71", 32)
	proof, ok, err := parseRealizationEpochMetrics(strings.NewReader(v24EpochMetric("sandbox-v24", "7", token, "1")), "sandbox-v24")
	if err != nil {
		t.Fatalf("parse exact epoch: %v", err)
	}
	if !ok {
		t.Fatal("exact epoch was not observed")
	}
	if !proof.Valid() {
		t.Fatal("trusted exact epoch did not mint sealed proof")
	}
	if sandboxID, ok := proof.SandboxID(); !ok || sandboxID != "sandbox-v24" {
		t.Fatalf("SandboxID=%q ok=%v", sandboxID, ok)
	}
	if generation, ok := proof.Generation(); !ok || generation != 7 {
		t.Fatalf("Generation=%d ok=%v", generation, ok)
	}
	if tokenHex, ok := proof.TokenHex(); !ok || tokenHex != token {
		t.Fatalf("TokenHex=%q ok=%v", tokenHex, ok)
	}
}

func TestV24RealizationEpochMetricRejectsNonCanonicalAuthority(t *testing.T) {
	goodToken := strings.Repeat("71", 32)
	zeroToken := strings.Repeat("00", 32)
	upperToken := strings.Repeat("AB", 32)

	cases := []struct {
		name       string
		sandboxID  string
		generation string
		token      string
		value      string
	}{
		{name: "empty sandbox", sandboxID: "", generation: "7", token: goodToken, value: "1"},
		{name: "whitespace sandbox", sandboxID: " sandbox-v24 ", generation: "7", token: goodToken, value: "1"},
		{name: "zero generation", sandboxID: "sandbox-v24", generation: "0", token: goodToken, value: "1"},
		{name: "leading zero generation", sandboxID: "sandbox-v24", generation: "07", token: goodToken, value: "1"},
		{name: "short token", sandboxID: "sandbox-v24", generation: "7", token: "71", value: "1"},
		{name: "uppercase token", sandboxID: "sandbox-v24", generation: "7", token: upperToken, value: "1"},
		{name: "zero token", sandboxID: "sandbox-v24", generation: "7", token: zeroToken, value: "1"},
		{name: "noncanonical one", sandboxID: "sandbox-v24", generation: "7", token: goodToken, value: "1.0"},
		{name: "wrong value", sandboxID: "sandbox-v24", generation: "7", token: goodToken, value: "0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := v24EpochMetric(tc.sandboxID, tc.generation, tc.token, tc.value)
			if proof, ok, err := parseRealizationEpochMetrics(strings.NewReader(body), "sandbox-v24"); err == nil || ok || proof.Valid() {
				t.Fatalf("non-canonical epoch accepted: proof=%+v ok=%v err=%v", proof, ok, err)
			}
		})
	}
}

func TestV24RealizationEpochMetricRejectsDuplicateExactSandbox(t *testing.T) {
	tokenA := strings.Repeat("71", 32)
	tokenB := strings.Repeat("72", 32)
	body := v24EpochMetric("sandbox-v24", "7", tokenA, "1") + v24EpochMetric("sandbox-v24", "7", tokenB, "1")
	if proof, ok, err := parseRealizationEpochMetrics(strings.NewReader(body), "sandbox-v24"); err == nil || ok || proof.Valid() {
		t.Fatalf("duplicate epoch accepted: proof=%+v ok=%v err=%v", proof, ok, err)
	}
}

func TestV24RealizationEpochMetricForOtherSandboxDoesNotMintAuthority(t *testing.T) {
	token := strings.Repeat("71", 32)
	proof, ok, err := parseRealizationEpochMetrics(strings.NewReader(v24EpochMetric("sandbox-other", "7", token, "1")), "sandbox-v24")
	if err != nil {
		t.Fatalf("other sandbox should be ignorable, got %v", err)
	}
	if ok || proof.Valid() {
		t.Fatalf("other sandbox minted authority: proof=%+v ok=%v", proof, ok)
	}
}

func TestV24DescriptiveEpochFieldsCannotForgeSealedProof(t *testing.T) {
	var token [32]byte
	for i := range token {
		token[i] = byte(i + 1)
	}
	forged := RealizationEpochProof{
		sandboxID:  "sandbox-v24",
		generation: 7,
		token:      token,
	}
	if forged.Valid() {
		t.Fatal("descriptive epoch fields forged sealed Wave24 authority")
	}
}
