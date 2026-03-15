package pki

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"
)

func TestNewCA(t *testing.T) {
	org := "Alloy Test Org"
	ca, err := NewCA(org)
	if err != nil {
		t.Fatalf("Failed to create CA: %v", err)
	}

	if ca.Cert.Subject.Organization[0] != org {
		t.Errorf("Expected organization %s, got %s", org, ca.Cert.Subject.Organization[0])
	}

	if !ca.Cert.IsCA {
		t.Error("Expected certificate to be a CA")
	}
}

func TestGenerateCertificate(t *testing.T) {
	ca, _ := NewCA("Alloy Test")
	
	certPem, keyPem, err := ca.GenerateCertificate("test-component", "Alloy Test", false)
	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	if certPem == nil || keyPem == nil {
		t.Fatal("Generated certificate or key is nil")
	}

	block, _ := pem.Decode(certPem)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatal("Failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse generated certificate: %v", err)
	}

	if cert.Subject.CommonName != "test-component" {
		t.Errorf("Expected CN test-component, got %s", cert.Subject.CommonName)
	}
}

func TestSaveToFile(t *testing.T) {
	ca, _ := NewCA("Alloy Test")
	certFile := "test_ca.crt"
	keyFile := "test_ca.key"
	defer os.Remove(certFile)
	defer os.Remove(keyFile)

	err := ca.SaveToFile(certFile, keyFile)
	if err != nil {
		t.Fatalf("Failed to save CA files: %v", err)
	}

	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		t.Error("Certificate file was not created")
	}

	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		t.Error("Key file was not created")
	}
}
