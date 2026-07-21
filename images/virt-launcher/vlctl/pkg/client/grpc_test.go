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

package client

import (
	"net"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"
)

func TestDialSocketConnectsEagerly(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	conn, err := DialSocket(socketPath)
	if err != nil {
		t.Fatalf("expected eager dial to succeed: %v", err)
	}
	_ = conn.Close()
}

func TestDialSocketFailsEarlyOnMissingSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "missing.sock")
	if _, err := DialSocket(socketPath); err == nil {
		t.Fatal("expected eager dial to fail on missing socket")
	}
}
