package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"
)

// IssueRequest carries the parameters for creating a new certificate
type IssueRequest struct {
	CommonName   string
	Organization string
	IsCA         bool
	DNSNames     []string
	IPAddresses  []net.IP
	Duration     time.Duration
}

// KeyPair represents a private key and its associated certificate
type KeyPair struct {
	Cert    *x509.Certificate
	Key     *ecdsa.PrivateKey
	CertPEM []byte
	KeyPEM  []byte
}

// GenerateKey generates a new P-256 ECDSA key
func GenerateKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// EncodeToPEM converts certificate and key to PEM format
func EncodeToPEM(certDer []byte, key *ecdsa.PrivateKey) ([]byte, []byte, error) {
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDer})

	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	return certPEM, keyPEM, nil
}

// CreateRootCA generates a new self-signed Root CA
func CreateRootCA(org string) (*KeyPair, error) {
	key, err := GenerateKey()
	if err != nil {
		return nil, err
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{org},
			CommonName:   "Alloy Machine Root",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	cert, _ := x509.ParseCertificate(der)
	certPEM, keyPEM, _ := EncodeToPEM(der, key)

	return &KeyPair{Cert: cert, Key: key, CertPEM: certPEM, KeyPEM: keyPEM}, nil
}

// SignCertificate signs a new certificate using the CA KeyPair
func SignCertificate(ca *KeyPair, req IssueRequest) (*KeyPair, error) {
	if ca == nil || ca.Cert == nil || ca.Key == nil {
		return nil, fmt.Errorf("invalid CA key pair")
	}
	key, err := GenerateKey()
	if err != nil {
		return nil, err
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	if req.Duration == 0 {
		req.Duration = time.Hour * 24 * 365 // 1 year default
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{req.Organization},
			CommonName:   req.CommonName,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(req.Duration),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		DNSNames:    req.DNSNames,
		IPAddresses: req.IPAddresses,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, ca.Cert, &key.PublicKey, ca.Key)
	if err != nil {
		return nil, err
	}

	cert, _ := x509.ParseCertificate(der)
	certPEM, keyPEM, _ := EncodeToPEM(der, key)

	return &KeyPair{Cert: cert, Key: key, CertPEM: certPEM, KeyPEM: keyPEM}, nil
}

// ParseKeyPair loads a certificate and key from PEM data
func ParseKeyPair(certPEM, keyPEM []byte) (*KeyPair, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("failed to decode key PEM")
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, err
	}

	return &KeyPair{
		Cert:    cert,
		Key:     key,
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
	}, nil
}
