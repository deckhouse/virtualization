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
	"errors"
	"strings"
	"testing"
)

func TestNoDiskImageError(t *testing.T) {
	// A failed write to the target volume must not be reported as a missing disk
	// image in the registry: the two fail for different reasons and send whoever
	// reads the log to different places.
	writeErr := errors.New(`could not open file "/data/disk.img": file exists`)

	tests := map[string]struct {
		layerErrs error
		want      string
	}{
		"no layer failed": {
			layerErrs: nil,
			want:      "Failed to find VM disk image file in the container image",
		},
		"a layer failed": {
			layerErrs: writeErr,
			want:      writeErr.Error(),
		},
		"every layer failed": {
			layerErrs: errors.Join(errors.New("Could not read layer"), writeErr),
			want:      writeErr.Error(),
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := noDiskImageError(tt.layerErrs)
			if err == nil {
				t.Fatal("no error returned")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not mention %q", err, tt.want)
			}
			if tt.layerErrs != nil && strings.Contains(err.Error(), "Failed to find VM disk image file") {
				t.Fatalf("layer failure reported as a missing disk image: %q", err)
			}
		})
	}
}
