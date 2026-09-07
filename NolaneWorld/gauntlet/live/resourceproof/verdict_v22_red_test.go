package resourceproof

import (
	"reflect"
	"testing"
)

// TestV22TrustedReportExposesPublicVerdictProjection is intentionally written
// through reflection so the first TDD checkpoint is a behavioral RED rather
// than a compiler/harness failure. The production API does not exist on the
// Wave 21 base, so this test must execute and fail until Wave 22 introduces the
// trusted-only public verdict projection.
func TestV22TrustedReportExposesPublicVerdictProjection(t *testing.T) {
	typ := reflect.TypeOf(TrustedReport{})
	method, ok := typ.MethodByName("ProjectVerdict")
	if !ok {
		t.Fatal("Wave 22 RED: TrustedReport.ProjectVerdict is absent")
	}
	if method.Type.NumIn() < 2 || method.Type.NumOut() != 2 {
		t.Fatalf("Wave 22 verdict projection has unexpected shape: %v", method.Type)
	}
}
