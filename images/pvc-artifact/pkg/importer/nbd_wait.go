/*
Copyright 2026 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
