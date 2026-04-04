package pki

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"io"
	"sync"
)

// TPMProvider is a simulated hardware provider for TPM-backed keys
type TPMProvider struct {
	mu   sync.RWMutex
	keys map[string]*TPMSigner
}

// NewTPMProvider creates a new TPM identity provider
func NewTPMProvider() *TPMProvider {
	return &TPMProvider{
		keys: make(map[string]*TPMSigner),
	}
}

func (p *TPMProvider) Name() string {
	return "tpm"
}

// GenerateKey generates a new simulated TPM-backed key
func (p *TPMProvider) GenerateKey() (HardwareSigner, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	id := fmt.Sprintf("tpm-key-%d", len(p.keys))
	signer := &TPMSigner{
		PrivateKey: key,
		id:         []byte(id),
	}

	p.mu.Lock()
	p.keys[id] = signer
	p.mu.Unlock()

	return signer, nil
}

// LoadKey loads a TPM-backed key by its ID
func (p *TPMProvider) LoadKey(keyID []byte) (HardwareSigner, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	signer, ok := p.keys[string(keyID)]
	if !ok {
		// In a real implementation, this would talk to the TPM chip
		// For simulation, we might want to persist the "TPM state" if needed
		return nil, fmt.Errorf("TPM key %s not found in secure storage", string(keyID))
	}
	return signer, nil
}

// TPMSigner implements HardwareSigner for TPM keys
type TPMSigner struct {
	*ecdsa.PrivateKey
	id []byte
}

func (s *TPMSigner) Public() crypto.PublicKey {
	return s.PrivateKey.Public()
}

func (s *TPMSigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	// Simulate hardware signing
	return s.PrivateKey.Sign(rand, digest, opts)
}

func (s *TPMSigner) HardwareKeyID() []byte {
	return s.id
}

func (s *TPMSigner) ProviderName() string {
	return "tpm"
}

func init() {
	RegisterHardwareProvider(NewTPMProvider())
}
