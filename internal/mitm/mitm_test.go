package mitm

import (
	"testing"
)

func TestNew(t *testing.T) {
	m := New(8080, 8081, nil, "test-secret")
	if m == nil {
		t.Fatal("New returned nil")
	}
	if m.listenPort != 8080 {
		t.Errorf("expected listenPort 8080, got %d", m.listenPort)
	}
	if m.targetPort != 8081 {
		t.Errorf("expected targetPort 8081, got %d", m.targetPort)
	}
}

func TestNewPorts(t *testing.T) {
	m := New(20182, 20181, nil, "test-secret")
	if m.listenPort != 20182 {
		t.Errorf("expected listenPort 20182, got %d", m.listenPort)
	}
	if m.targetPort != 20181 {
		t.Errorf("expected targetPort 20181, got %d", m.targetPort)
	}
}
