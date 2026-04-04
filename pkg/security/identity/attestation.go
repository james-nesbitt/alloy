package identity

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/security/pki"
)

// Attestor handles the creation of cryptographic attestations for actors.
type Attestor struct {
	keyPair *pki.KeyPair
}

func NewAttestor(pair *pki.KeyPair) *Attestor {
	return &Attestor{keyPair: pair}
}

// CreateAttestation generates a signed attestation for an actor and role.
func (a *Attestor) CreateAttestation(actor, role string) (api.Attestation, error) {
	att := api.Attestation{
		ID:        fmt.Sprintf("att-%d", time.Now().UnixNano()),
		Actor:     actor,
		Role:      role,
		Timestamp: time.Now().Unix(),
	}

	if hws, ok := a.keyPair.Key.(pki.HardwareSigner); ok {
		att.Hardware = hws.ProviderName()
	}

	// Marshaling public key for inclusion
	pubBytes, err := x509.MarshalPKIXPublicKey(a.keyPair.Key.Public())
	if err == nil {
		att.PublicKey = pubBytes
	}

	// Signing the attestation content
	data, _ := json.Marshal(struct {
		Actor     string `json:"actor"`
		Role      string `json:"role"`
		Timestamp int64  `json:"timestamp"`
	}{
		Actor:     att.Actor,
		Role:      att.Role,
		Timestamp: att.Timestamp,
	})

	hash := sha256.Sum256(data)
	sig, err := a.keyPair.Key.Sign(rand.Reader, hash[:], crypto.SHA256)
	if err != nil {
		return api.Attestation{}, err
	}

	att.Signature = sig
	return att, nil
}

// VerifyAttestation checks if an attestation is valid.
func VerifyAttestation(att api.Attestation) error {
	if att.PublicKey == nil {
		return fmt.Errorf("missing public key")
	}

	pub, err := x509.ParsePKIXPublicKey(att.PublicKey)
	if err != nil {
		return err
	}

	ecdsaPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("unsupported key type: %T", pub)
	}

	data, _ := json.Marshal(struct {
		Actor     string `json:"actor"`
		Role      string `json:"role"`
		Timestamp int64  `json:"timestamp"`
	}{
		Actor:     att.Actor,
		Role:      att.Role,
		Timestamp: att.Timestamp,
	})

	hash := sha256.Sum256(data)

	if !ecdsa.VerifyASN1(ecdsaPub, hash[:], att.Signature) {
		return fmt.Errorf("invalid attestation signature")
	}

	return nil
}
