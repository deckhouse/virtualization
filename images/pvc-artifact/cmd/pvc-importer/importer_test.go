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

package main

import "testing"

func TestNewDataSourceRejectsUnknownSource(t *testing.T) {
	// An unknown source used to exit the process on the spot, past the reporting the caller
	// does: the pod died without telling the controller why.
	ds, err := newDataSource("nowhere")
	if err == nil {
		t.Fatal("an unknown data source must be reported, not accepted")
	}
	if ds != nil {
		t.Fatalf("got a data source %v for an unknown source", ds)
	}
}

func TestNewDataSourceBuildsRegistrySource(t *testing.T) {
	ds, err := newDataSource(sourceRegistry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ds == nil {
		t.Fatal("no data source for the registry source")
	}
}
