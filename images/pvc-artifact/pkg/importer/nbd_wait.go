package importer

import (
	"fmt"
	"net"
	"net/url"
	"time"
)

const nbdDialInterval = time.Second

// WaitForNBDEndpoint blocks until the NBD TCP endpoint accepts connections or timeout expires.
func WaitForNBDEndpoint(nbdEndpoint string, timeout time.Duration) error {
	parsed, err := url.Parse(nbdEndpoint)
	if err != nil {
		return fmt.Errorf("parse NBD endpoint %q: %w", nbdEndpoint, err)
	}
	if parsed.Scheme != "nbd" {
		return fmt.Errorf("unsupported NBD endpoint scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("NBD endpoint %q has empty host", nbdEndpoint)
	}

	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", parsed.Host, 2*time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
		time.Sleep(nbdDialInterval)
	}
	return fmt.Errorf("timed out waiting for NBD endpoint %q: %w", nbdEndpoint, lastErr)
}
