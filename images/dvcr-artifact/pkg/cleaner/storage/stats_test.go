/*
Copyright 2025 Flant JSC

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
	"path/filepath"
	"testing"
)

func TestFSBytes_MissingDirReturnsEmpty(t *testing.T) {
	// Path that almost certainly does not exist. Use t.TempDir() as a base
	// and append a child that we never create.
	missing := filepath.Join(t.TempDir(), "does-not-exist", "repositories")

	info, err := FSBytes(missing)
	if err != nil {
		t.Fatalf("FSBytes on a missing path must not return an error, got: %v", err)
	}
	if info.Total != 0 || info.Available != 0 {
		t.Fatalf("expected zero-valued FSInfo on missing path, got: %+v", info)
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
