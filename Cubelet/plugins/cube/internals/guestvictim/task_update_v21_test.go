// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package guestvictim

import "testing"

func TestV21TaskUpdateAnnotationsAreExactAndCanonical(t *testing.T) {
	var token [32]byte
	for i := range token {
		token[i] = byte(i + 1)
	}

	annotations, err := TaskUpdateAnnotations(token)
	if err != nil {
		t.Fatalf("TaskUpdateAnnotations: %v", err)
	}
	if len(annotations) != 2 {
		t.Fatalf("annotation count = %d, want 2", len(annotations))
	}
	if got := annotations[UpdateActionAnnotation]; got != BindAction {
		t.Fatalf("action = %q, want %q", got, BindAction)
	}
	const wantToken = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	if got := annotations[RealizationTokenAnnotation]; got != wantToken {
		t.Fatalf("token = %q, want %q", got, wantToken)
	}
}

func TestV21TaskUpdateAnnotationsRejectZeroToken(t *testing.T) {
	if _, err := TaskUpdateAnnotations([32]byte{}); err == nil {
		t.Fatal("expected all-zero token to be rejected")
	}
}
