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
