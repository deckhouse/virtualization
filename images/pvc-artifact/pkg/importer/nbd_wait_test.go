package importer

import (
	"fmt"
	"net"
	"testing"
	"time"
)

func TestWaitForNBDEndpointRejectsInvalidURL(t *testing.T) {
	t.Parallel()

	err := WaitForNBDEndpoint("://bad-url", time.Second)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestWaitForNBDEndpointRejectsUnsupportedScheme(t *testing.T) {
	t.Parallel()

	err := WaitForNBDEndpoint("http://127.0.0.1:1", time.Second)
	if err == nil {
		t.Fatal("expected scheme error")
	}
}

func TestWaitForNBDEndpointRejectsEmptyHost(t *testing.T) {
	t.Parallel()

	err := WaitForNBDEndpoint("nbd://", time.Second)
	if err == nil {
		t.Fatal("expected empty host error")
	}
}

func TestWaitForNBDEndpointSucceedsWhenListenerIsReady(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	endpoint := fmt.Sprintf("nbd://127.0.0.1:%d", port)

	if err := WaitForNBDEndpoint(endpoint, 2*time.Second); err != nil {
		t.Fatalf("WaitForNBDEndpoint: %v", err)
	}
}

func TestWaitForNBDEndpointTimesOutWhenNothingListens(t *testing.T) {
	t.Parallel()

	start := time.Now()
	err := WaitForNBDEndpoint("nbd://127.0.0.1:1", 1500*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed < 1500*time.Millisecond {
		t.Fatalf("expected to wait at least 1.5s, got %v", elapsed)
	}
}
