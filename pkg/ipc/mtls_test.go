package ipc

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/james-nesbitt/alloy/pkg/security/pki"
)

type mockHWR struct {
	keys map[string]*mockHWS
}

func (p *mockHWR) Name() string { return "mock" }
func (p *mockHWR) GenerateKey() (pki.HardwareSigner, error) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	id := fmt.Sprintf("key-%d", len(p.keys))
	s := &mockHWS{PrivateKey: key, id: []byte(id)}
	p.keys[id] = s
	return s, nil
}
func (p *mockHWR) LoadKey(keyID []byte) (pki.HardwareSigner, error) {
	s, ok := p.keys[string(keyID)]
	if !ok {
		return nil, fmt.Errorf("key not found")
	}
	return s, nil
}

type mockHWS struct {
	*ecdsa.PrivateKey
	id []byte
}

func (s *mockHWS) HardwareKeyID() []byte { return s.id }
func (s *mockHWS) ProviderName() string  { return "mock" }

func TestHardwareMTLS(t *testing.T) {
	p := &mockHWR{keys: make(map[string]*mockHWS)}
	pki.RegisterHardwareProvider(p)

	ca, _ := pki.CreateRootCA("Alloy")

	serverKP, err := pki.SignCertificate(ca, pki.IssueRequest{
		CommonName:   "server",
		Organization: "Alloy",
		Hardware:     "mock",
	})
	if err != nil {
		t.Fatalf("failed to sign server cert: %v", err)
	}

	clientKP, err := pki.SignCertificate(ca, pki.IssueRequest{
		CommonName:   "client",
		Organization: "Alloy",
		Hardware:     "mock",
	})
	if err != nil {
		t.Fatalf("failed to sign client cert: %v", err)
	}

	serverConfig, err := NewServerTLSConfig(ca.CertPEM, serverKP.CertPEM, serverKP.KeyPEM)
	if err != nil {
		t.Fatalf("failed to create server tls config: %v", err)
	}

	clientConfig, err := NewClientTLSConfig(ca.CertPEM, clientKP.CertPEM, clientKP.KeyPEM)
	if err != nil {
		t.Fatalf("failed to create client tls config: %v", err)
	}

	if serverConfig == nil || clientConfig == nil {
		t.Fatal("configs are nil")
	}

	// Verify server cert is hardware-backed
	if serverConfig.Certificates[0].PrivateKey == nil {
		t.Fatal("server private key is nil")
	}
	if _, ok := serverConfig.Certificates[0].PrivateKey.(pki.HardwareSigner); !ok {
		t.Errorf("server key is not hardware signer: %T", serverConfig.Certificates[0].PrivateKey)
	}

	// Verify client cert is hardware-backed
	if clientConfig.Certificates[0].PrivateKey == nil {
		t.Fatal("client private key is nil")
	}
	if _, ok := clientConfig.Certificates[0].PrivateKey.(pki.HardwareSigner); !ok {
		t.Errorf("client key is not hardware signer: %T", clientConfig.Certificates[0].PrivateKey)
	}
}
