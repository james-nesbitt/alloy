package identity

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/jnesbitt/alloy-go/pkg/security/pki"
)

type Store struct {
	baseDir string
}

func NewStore(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

func (s *Store) CADir() string {
	return s.baseDir
}

func (s *Store) InstanceDir(name string) string {
	return filepath.Join(s.baseDir, "instances", name)
}

// InitializeMachine creates the root CA if it doesn't exist
func (s *Store) InitializeMachine() (*pki.KeyPair, error) {
	certPath := filepath.Join(s.baseDir, "user-ca.crt")
	keyPath := filepath.Join(s.baseDir, "user-ca.key")

	if _, err := os.Stat(certPath); err == nil {
		certPEM, _ := os.ReadFile(certPath)
		keyPEM, _ := os.ReadFile(keyPath)
		return pki.ParseKeyPair(certPEM, keyPEM)
	}

	if err := os.MkdirAll(s.baseDir, 0700); err != nil {
		return nil, err
	}

	ca, err := pki.CreateRootCA("Alloy User-Level")
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(certPath, ca.CertPEM, 0644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, ca.KeyPEM, 0600); err != nil {
		return nil, err
	}

	return ca, nil
}

// CreateInstanceIdentity generates certificates for a specific core instance
func (s *Store) CreateInstanceIdentity(ca *pki.KeyPair, name string) (*pki.KeyPair, error) {
	dir := s.InstanceDir(name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}

	san := fmt.Sprintf("alloy.instance.%s", name)
	req := pki.IssueRequest{
		CommonName:   san,
		Organization: "Alloy Instances",
		DNSNames:     []string{"localhost", name},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	pair, err := pki.SignCertificate(ca, req)
	if err != nil {
		return nil, err
	}

	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")

	if err := os.WriteFile(certPath, pair.CertPEM, 0644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, pair.KeyPEM, 0600); err != nil {
		return nil, err
	}

	return pair, nil
}

// GetClientTLSConfig returns a config for a frontend to connect to cores
func (s *Store) GetClientTLSConfig(ca *pki.KeyPair, clientName string) (*tls.Config, error) {
	req := pki.IssueRequest{
		CommonName:   clientName,
		Organization: "Alloy Clients",
	}

	pair, err := pki.SignCertificate(ca, req)
	if err != nil {
		return nil, err
	}

	tlsCert, err := tls.X509KeyPair(pair.CertPEM, pair.KeyPEM)
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	certPool.AddCert(ca.Cert)

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		RootCAs:      certPool,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// GetServerTLSConfig returns a config for a core instance
func (s *Store) GetServerTLSConfig(ca *pki.KeyPair, pair *pki.KeyPair) (*tls.Config, error) {
	tlsCert, err := tls.X509KeyPair(pair.CertPEM, pair.KeyPEM)
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	certPool.AddCert(ca.Cert)

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool,
		MinVersion:   tls.VersionTLS13,
	}, nil
}
