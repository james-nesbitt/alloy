package pki

import (
	"encoding/pem"
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
