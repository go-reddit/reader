package browserhttp

import (
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	c := NewClient(5 * time.Second)
	if c == nil || c.Jar == nil || c.Transport == nil || c.Timeout != 5*time.Second {
		t.Fatalf("client not configured: %+v", c)
	}
}

func TestNewTransport(t *testing.T) {
	tr := NewTransport()
	if tr == nil || tr.DialTLSContext == nil {
		t.Fatal("transport missing DialTLSContext")
	}
}
