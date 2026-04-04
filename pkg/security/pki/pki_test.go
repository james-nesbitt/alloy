package pki

import (
	"crypto/ecdsa"
	"encoding/pem"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestPKI(t *testing.T) {
	org := "Alloy Test Org"
	ca, err := CreateRootCA(org)
	if err != nil {
		t.Fatalf("failed to create root CA: %v", err)
	}

	if ca.Cert.Subject.Organization[0] != org {
		t.Errorf("expected org %s, got %s", org, ca.Cert.Subject.Organization[0])
	}

	if !ca.Cert.IsCA {
		t.Error("expected certificate to be a CA")
	}

	// Test signing
	req := IssueRequest{
		CommonName:   "test-component",
		Organization: org,
		Duration:     time.Hour,
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	kp, err := SignCertificate(ca, req)
	if err != nil {
		t.Fatalf("failed to sign certificate: %v", err)
	}

	if kp.Cert.Subject.CommonName != req.CommonName {
		t.Errorf("expected CN %s, got %s", req.CommonName, kp.Cert.Subject.CommonName)
	}

	// Test ParseKeyPair
	parsed, err := ParseKeyPair(kp.CertPEM, kp.KeyPEM)
	if err != nil {
		t.Fatalf("failed to parse key pair: %v", err)
	}

	if parsed.Cert.Subject.CommonName != kp.Cert.Subject.CommonName {
		t.Error("parsed cert does not match original")
	}

	// Test EncodeToPEM directly
	cp, kp2, err := EncodeToPEM(kp.Cert.Raw, kp.Key)
	if err != nil {
		t.Errorf("EncodeToPEM failed: %v", err)
	}
	if cp == nil || kp2 == nil {
		t.Error("EncodeToPEM returned nil")
	}
}

type mockHardwareProvider struct {
	keys map[string]*mockHardwareSigner
}

func (p *mockHardwareProvider) Name() string { return "mock" }
func (p *mockHardwareProvider) GenerateKey() (HardwareSigner, error) {
	key, _ := GenerateKey()
	id := fmt.Sprintf("key-%d", len(p.keys))
	s := &mockHardwareSigner{PrivateKey: key, id: []byte(id)}
	p.keys[id] = s
	return s, nil
}
func (p *mockHardwareProvider) LoadKey(keyID []byte) (HardwareSigner, error) {
	s, ok := p.keys[string(keyID)]
	if !ok {
		return nil, fmt.Errorf("key not found")
	}
	return s, nil
}

type mockHardwareSigner struct {
	*ecdsa.PrivateKey
	id []byte
}

func (s *mockHardwareSigner) HardwareKeyID() []byte { return s.id }
func (s *mockHardwareSigner) ProviderName() string  { return "mock" }

func TestHardwarePKI(t *testing.T) {
	p := &mockHardwareProvider{keys: make(map[string]*mockHardwareSigner)}
	RegisterHardwareProvider(p)

	ca, _ := CreateRootCA("Alloy")

	req := IssueRequest{
		CommonName:   "hw-instance",
		Organization: "Alloy",
		Hardware:     "mock",
	}

	kp, err := SignCertificate(ca, req)
	if err != nil {
		t.Fatalf("failed to sign hardware cert: %v", err)
	}

	if _, ok := kp.Key.(HardwareSigner); !ok {
		t.Errorf("expected hardware signer, got %T", kp.Key)
	}

	// Verify PEM format
	block, _ := pem.Decode(kp.KeyPEM)
	if block.Type != "ALOY HARDWARE KEY" {
		t.Errorf("unexpected PEM type: %s", block.Type)
	}
	if block.Headers["Provider"] != "mock" {
		t.Errorf("unexpected provider: %s", block.Headers["Provider"])
	}

	// Test ParseKeyPair with hardware key
	parsed, err := ParseKeyPair(kp.CertPEM, kp.KeyPEM)
	if err != nil {
		t.Fatalf("failed to parse hardware key pair: %v", err)
	}

	if hws, ok := parsed.Key.(HardwareSigner); !ok {
		t.Errorf("parsed key is not hardware signer: %T", parsed.Key)
	} else if string(hws.HardwareKeyID()) != "key-0" {
		t.Errorf("wrong key ID: %s", hws.HardwareKeyID())
	}
}

func TestPKIFailures(t *testing.T) {
	_, err := ParseKeyPair([]byte("invalid"), []byte("invalid"))
	if err == nil {
		t.Error("expected error parsing invalid PEM")
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("invalid")})
	_, err = ParseKeyPair(certPEM, []byte("invalid"))
	if err == nil {
		t.Error("expected error parsing invalid certificate DER")
	}

	_, err = SignCertificate(nil, IssueRequest{})
	if err == nil {
		t.Error("expected error signing with nil CA")
	}
}
