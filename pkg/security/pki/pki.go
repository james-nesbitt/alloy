package pki

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"
)

// HardwareProvider interface defines methods for managing hardware-backed keys
type HardwareProvider interface {
	Name() string
	GenerateKey() (HardwareSigner, error)
	LoadKey(keyID []byte) (HardwareSigner, error)
}

// HardwareSigner interface extends crypto.Signer to support hardware-backed keys
type HardwareSigner interface {
	crypto.Signer
	HardwareKeyID() []byte
	ProviderName() string
}

var (
	providersMu       sync.RWMutex
	hardwareProviders = make(map[string]HardwareProvider)
)

// RegisterHardwareProvider registers a new hardware key provider
func RegisterHardwareProvider(p HardwareProvider) {
	providersMu.Lock()
	defer providersMu.Unlock()
	hardwareProviders[p.Name()] = p
}

// GetHardwareProvider returns a hardware provider by name
func GetHardwareProvider(name string) (HardwareProvider, error) {
	providersMu.RLock()
	defer providersMu.RUnlock()
	p, ok := hardwareProviders[name]
	if !ok {
		return nil, fmt.Errorf("hardware provider %q not found", name)
	}
	return p, nil
}

// IssueRequest carries the parameters for creating a new certificate
type IssueRequest struct {
	CommonName   string
	Organization string
	IsCA         bool
	DNSNames     []string
	IPAddresses  []net.IP
	Duration     time.Duration
	Hardware     string // Hardware provider name if hardware-backed key is requested
}

// KeyPair represents a private key or hardware signer and its associated certificate
type KeyPair struct {
	Cert    *x509.Certificate
	Key     crypto.Signer
	CertPEM []byte
	KeyPEM  []byte
}

// GenerateKey generates a new P-256 ECDSA key (software-based)
func GenerateKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// EncodeToPEM converts certificate and key to PEM format
func EncodeToPEM(certDer []byte, key crypto.Signer) ([]byte, []byte, error) {
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDer})

	var keyPEM []byte
	if ecdsaKey, ok := key.(*ecdsa.PrivateKey); ok {
		keyBytes, err := x509.MarshalECPrivateKey(ecdsaKey)
		if err != nil {
			return nil, nil, err
		}
		keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	} else if hws, ok := key.(HardwareSigner); ok {
		keyPEM = pem.EncodeToMemory(&pem.Block{
			Type: "ALOY HARDWARE KEY",
			Headers: map[string]string{
				"Provider": hws.ProviderName(),
			},
			Bytes: hws.HardwareKeyID(),
		})
	} else {
		return nil, nil, fmt.Errorf("unsupported key type: %T", key)
	}

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

	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
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

	var key crypto.Signer
	var err error

	if req.Hardware != "" {
		provider, err := GetHardwareProvider(req.Hardware)
		if err != nil {
			return nil, err
		}
		key, err = provider.GenerateKey()
		if err != nil {
			return nil, err
		}
	} else {
		key, err = GenerateKey()
		if err != nil {
			return nil, err
		}
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

	der, err := x509.CreateCertificate(rand.Reader, template, ca.Cert, key.Public(), ca.Key)
	if err != nil {
		return nil, err
	}

	cert, _ := x509.ParseCertificate(der)
	certPEM, keyPEM, _ := EncodeToPEM(der, key)

	return &KeyPair{Cert: cert, Key: key, CertPEM: certPEM, KeyPEM: keyPEM}, nil
}

// LoadTLSCertificate loads a TLS certificate from PEM data, supporting hardware keys
func LoadTLSCertificate(certPEM, keyPEM []byte) (tls.Certificate, error) {
	// Try parsing via pki first to detect hardware-backed keys
	pair, err := ParseKeyPair(certPEM, keyPEM)
	if err == nil {
		if _, ok := pair.Key.(HardwareSigner); ok {
			return tls.Certificate{
				Certificate: [][]byte{pair.Cert.Raw},
				PrivateKey:  pair.Key,
				Leaf:        pair.Cert,
			}, nil
		}
	}

	// Fallback to standard loading for software keys
	return tls.X509KeyPair(certPEM, keyPEM)
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

	var key crypto.Signer
	switch keyBlock.Type {
	case "EC PRIVATE KEY":
		key, err = x509.ParseECPrivateKey(keyBlock.Bytes)
	case "ALOY HARDWARE KEY":
		providerName := keyBlock.Headers["Provider"]
		provider, perr := GetHardwareProvider(providerName)
		if perr != nil {
			return nil, perr
		}
		key, err = provider.LoadKey(keyBlock.Bytes)
	default:
		return nil, fmt.Errorf("unsupported key type: %s", keyBlock.Type)
	}

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
