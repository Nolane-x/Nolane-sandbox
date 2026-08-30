package delegation

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestValidateSecretHandleMatchesV6Rule(t *testing.T) {
	if err := ValidateSecretHandle(SecretHandle("kms/github/repo-a")); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []SecretHandle{
		"",
		" leading",
		"trailing ",
		SecretHandle(strings.Repeat("x", 513)),
	} {
		if err := ValidateSecretHandle(bad); !errors.Is(err, ErrSecretUnavailable) {
			t.Fatalf("handle=%q err=%v", bad, err)
		}
	}
}

func TestWithSecretLeaseCopiesAndWipesWorkingMaterial(t *testing.T) {
	original := []byte("SYNTHETIC-V7-SECRET")
	var lease []byte
	if err := WithSecretLease(original, func(secret Secret) error {
		lease = secret.Bytes()
		if string(lease) != string(original) {
			t.Fatalf("lease=%q", lease)
		}
		if len(lease) == 0 || &lease[0] == &original[0] {
			t.Fatal("lease aliases caller material")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(lease, original) {
		t.Fatal("lease working buffer was not wiped")
	}
	if string(original) != "SYNTHETIC-V7-SECRET" {
		t.Fatal("caller material mutated")
	}
}

func TestWithSecretLeaseRejectsInvalidInputs(t *testing.T) {
	if err := WithSecretLease(nil, func(Secret) error { return nil }); !errors.Is(err, ErrSecretUnavailable) {
		t.Fatalf("empty material err=%v", err)
	}
	if err := WithSecretLease([]byte("secret"), nil); !errors.Is(err, ErrSecretUnavailable) {
		t.Fatalf("nil callback err=%v", err)
	}
}
