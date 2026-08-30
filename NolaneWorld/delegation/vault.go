package delegation

import (
	"bytes"
	"context"
	"sync"
)

// Secret is a callback-scoped credential view. Construction is package-owned;
// trusted host adapters may read the bytes only while Vault.Use is active.
type Secret struct {
	material []byte
}

func (s Secret) Bytes() []byte { return s.material }

type Vault interface {
	Use(context.Context, SecretHandle, func(Secret) error) error
}

// ValidateSecretHandle exposes the exact v6 validity rule to trusted host-side
// Vault implementations without exposing the package-private generic validator.
func ValidateSecretHandle(handle SecretHandle) error {
	if !strict(string(handle), 512) {
		return ErrSecretUnavailable
	}
	return nil
}

// WithSecretLease constructs a callback-scoped Secret while keeping Secret
// construction package-owned. The working copy is wiped before return.
func WithSecretLease(material []byte, fn func(Secret) error) error {
	if len(material) == 0 || len(material) > 1024*1024 || fn == nil {
		return ErrSecretUnavailable
	}
	working := append([]byte(nil), material...)
	defer zero(working)
	return fn(Secret{material: working})
}

// MemoryVault is for tests/development. Production deployments should provide
// a KMS/HSM-backed Vault implementation without changing the plane protocol.
type MemoryVault struct {
	mu      sync.RWMutex
	secrets map[SecretHandle][]byte
}

func NewMemoryVault() *MemoryVault {
	return &MemoryVault{secrets: make(map[SecretHandle][]byte)}
}

func (v *MemoryVault) Put(handle SecretHandle, material []byte) error {
	if v == nil || ValidateSecretHandle(handle) != nil || len(material) == 0 || len(material) > 1024*1024 {
		return ErrSecretUnavailable
	}
	copyMaterial := append([]byte(nil), material...)
	v.mu.Lock()
	defer v.mu.Unlock()
	if prior, ok := v.secrets[handle]; ok {
		if !bytes.Equal(prior, copyMaterial) {
			zero(copyMaterial)
			return ErrSecretHandleCollision
		}
		zero(copyMaterial)
		return nil
	}
	v.secrets[handle] = copyMaterial
	return nil
}

func (v *MemoryVault) Use(ctx context.Context, handle SecretHandle, fn func(Secret) error) error {
	if v == nil || ctx == nil || ValidateSecretHandle(handle) != nil || fn == nil {
		return ErrSecretUnavailable
	}
	select {
	case <-ctx.Done():
		return ErrSecretUnavailable
	default:
	}
	v.mu.RLock()
	stored, ok := v.secrets[handle]
	if ok {
		stored = append([]byte(nil), stored...)
	}
	v.mu.RUnlock()
	if !ok || len(stored) == 0 {
		return ErrSecretUnavailable
	}
	defer zero(stored)
	return fn(Secret{material: stored})
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
