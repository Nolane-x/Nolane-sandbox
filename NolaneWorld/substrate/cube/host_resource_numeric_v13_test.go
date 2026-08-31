package cube

import (
	"math"
	"testing"
)

func TestV13ExactUintRejectsRoundedOverflowBoundary(t *testing.T) {
	if _, err := exactUint(float64(math.MaxUint64)); err == nil {
		t.Fatal("rounded 2^64 float boundary must fail closed")
	}
}
