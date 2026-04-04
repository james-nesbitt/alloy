package ipc

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/james-nesbitt/alloy/pkg/security/pki"
)

// NewServerTLSConfig creates an mTLS config for the server
func NewServerTLSConfig(caCertPEM, serverCertPEM, serverKeyPEM []byte) (*tls.Config, error) {
	cert, err := pki.LoadTLSCertificate(serverCertPEM, serverKeyPEM)
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
	cert, err := pki.LoadTLSCertificate(clientCertPEM, clientKeyPEM)
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
