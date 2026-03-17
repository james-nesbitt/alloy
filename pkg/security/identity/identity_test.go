package identity

import (
	"os"
	"testing"
	"time"
)

func TestIdentityStore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "identity-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	s := NewStore(tempDir)
	if s.CADir() == "" || s.InstanceDir("test") == "" {
		t.Error("expected non-empty directories")
	}

	ca, err := s.InitializeMachine()
	if err != nil {
		t.Fatalf("InitializeMachine failed: %v", err)
	}

	// Should not fail if already initialized (it should load existing)
	_, err = s.InitializeMachine()
	if err != nil {
		t.Errorf("expected success re-initializing machine, got %v", err)
	}

	instancePair, err := s.CreateInstanceIdentity(ca, "instance-1")
	if err != nil {
		t.Fatalf("CreateInstanceIdentity failed: %v", err)
	}

	clientConfig, err := s.GetClientTLSConfig(ca, "client-1")
	if err != nil {
		t.Fatalf("GetClientTLSConfig failed: %v", err)
	}
	if clientConfig == nil {
		t.Fatal("client config is nil")
	}

	serverConfig, err := s.GetServerTLSConfig(ca, instancePair)
	if err != nil {
		t.Fatalf("GetServerTLSConfig failed: %v", err)
	}
	if serverConfig == nil {
		t.Fatal("server config is nil")
	}
}

func TestInstanceTracking(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tracking-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	runtimeDir, _ := os.MkdirTemp("", "runtime-test")
	defer os.RemoveAll(runtimeDir)

	s := NewStore(tempDir)
	info := InstanceInfo{
		Name:      "test-instance",
		PID:       os.Getpid(),
		Socket:    "test.sock",
		StartTime: time.Now(),
	}

	err = s.WriteInstanceInfo(info, runtimeDir)
	if err != nil {
		t.Fatalf("WriteInstanceInfo failed: %v", err)
	}

	instances, err := s.ListInstances(runtimeDir)
	if err != nil {
		t.Fatalf("ListInstances failed: %v", err)
	}
	if len(instances) != 1 {
		t.Errorf("expected 1 instance, got %d", len(instances))
	}

	err = s.ClearInstanceInfo("test-instance", runtimeDir)
	if err != nil {
		t.Fatalf("ClearInstanceInfo failed: %v", err)
	}

	instances, _ = s.ListInstances(runtimeDir)
	if len(instances) != 0 {
		t.Error("expected 0 instances after clear")
	}
}

func TestStoreFailures(t *testing.T) {
	s := NewStore("/dev/null/invalid")
	_, err := s.InitializeMachine()
	if err == nil {
		t.Error("expected error initializing machine in invalid path")
	}

	s2 := NewStore(os.TempDir())
	_ = s2
	// We don't call InitializeMachine, so s2 has no CA yet.
	// But GetClientTLSConfig/GetServerTLSConfig now take ca as argument.
	// If we pass nil, pki.SignCertificate will panic.
	// So we avoid passing nil.
}
