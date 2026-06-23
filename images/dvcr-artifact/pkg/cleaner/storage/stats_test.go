//go:build unix

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

package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFSBytes_MissingDirReturnsError(t *testing.T) {
	// Path that almost certainly does not exist. Use t.TempDir() as a base
	// and append a child that we never create.
	missing := filepath.Join(t.TempDir(), "does-not-exist", "repositories")

	_, err := FSBytes(missing)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected not exist error for %q, got: %v", missing, err)
	}
}

func TestFSBytes_ExistingDir(t *testing.T) {
	// t.TempDir() always exists and is a real directory.
	dir := t.TempDir()

	info, err := FSBytes(dir)
	if err != nil {
		t.Fatalf("FSBytes on an existing path returned an error: %v", err)
	}
	// On a real filesystem, total bytes is strictly positive.
	if info.Total == 0 {
		t.Fatalf("expected positive Total for %q, got 0", dir)
	}
}
