package ipc

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"github.com/james-nesbitt/alloy/pkg/security/pki"
)

// loadTLSCertificate loads a TLS certificate from PEM data, supporting hardware keys
func loadTLSCertificate(certPEM, keyPEM []byte) (tls.Certificate, error) {
	// Try parsing via pki first to detect hardware-backed keys
	pair, err := pki.ParseKeyPair(certPEM, keyPEM)
	if err == nil {
		if _, ok := pair.Key.(pki.HardwareSigner); ok {
			return tls.Certificate{
				Certificate: [][]byte{pair.Cert.Raw},
				PrivateKey:  pair.Key,
				Leaf:        pair.Cert,
			}, nil
		}
	}

	// Fallback to standard loading for software keys (supports cert chains)
	return tls.X509KeyPair(certPEM, keyPEM)
}

// NewServerTLSConfig creates an mTLS config for the server
func NewServerTLSConfig(caCertPEM, serverCertPEM, serverKeyPEM []byte) (*tls.Config, error) {
	cert, err := loadTLSCertificate(serverCertPEM, serverKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to load server key pair: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCertPEM) {
		return nil, fmt.Errorf("failed to append CA certificate to pool")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caCertPool,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// NewClientTLSConfig creates an mTLS config for the client
func NewClientTLSConfig(caCertPEM, clientCertPEM, clientKeyPEM []byte) (*tls.Config, error) {
	cert, err := loadTLSCertificate(clientCertPEM, clientKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to load client key pair: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCertPEM) {
		return nil, fmt.Errorf("failed to append CA certificate to pool")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS13,
	}, nil
}
