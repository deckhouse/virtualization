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
	"math"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

var malformedImageSizes = []string{
	"not-a-quantity",
	"10 Gi",
	"1Gi\n",
	"--1",
}

func stubAvailableSpace(t *testing.T) {
	t.Helper()

	origBlock, origFS := getAvailableSpaceBlockFunc, getAvailableSpaceFunc
	getAvailableSpaceBlockFunc = func(string) (int64, error) { return -1, nil }
	getAvailableSpaceFunc = func(string) (int64, error) { return 10 << 30, nil }
	t.Cleanup(func() {
		getAvailableSpaceBlockFunc, getAvailableSpaceFunc = origBlock, origFS
	})
}

func TestNewDataProcessorRejectsMalformedImageSize(t *testing.T) {
	stubAvailableSpace(t)

	for _, imageSize := range malformedImageSizes {
		_, err := NewDataProcessor(nil, "", "", "", imageSize, 0, false, "")
		if err == nil {
			t.Errorf("image size %q: expected an error", imageSize)
			continue
		}
		if !strings.Contains(err.Error(), fmt.Sprintf("%q", imageSize)) {
			t.Errorf("image size %q: error does not name the offending value: %v", imageSize, err)
		}
	}
}

func TestNewDataProcessorAcceptsValidImageSize(t *testing.T) {
	stubAvailableSpace(t)

	for _, imageSize := range []string{"1Gi", ""} {
		if _, err := NewDataProcessor(nil, "", "", "", imageSize, 0, false, ""); err != nil {
			t.Errorf("image size %q: unexpected error: %v", imageSize, err)
		}
	}
}

func TestResizeImageRejectsMalformedImageSize(t *testing.T) {
	for _, imageSize := range malformedImageSizes {
		err := ResizeImage("", imageSize, 0, false)
		if err == nil {
			t.Errorf("image size %q: expected an error", imageSize)
			continue
		}
		if !strings.Contains(err.Error(), fmt.Sprintf("%q", imageSize)) {
			t.Errorf("image size %q: error does not name the offending value: %v", imageSize, err)
		}
	}
}

// fuzzImageSizeSeeds are the shapes an operator or a controller could put into
// IMPORTER_IMAGE_SIZE: every suffix family resource.Quantity knows, fractions,
// exponents, signs, the int64 boundary, and the near misses of each.
var fuzzImageSizeSeeds = []string{
	"1Gi", "500M", "0",
	"1", "1k", "1Ki", "1Ti", "1Pi", "1Ei",
	"1e3", "1E3", "1.5Gi", "0.5", ".5", "5.",
	"-1Gi", "+1Gi",
	"9223372036854775807", "9223372036854775808", "1e999",
	"", " ", "1Mi ", "1GI", "1Gib", "0x10", "1_000",
	"Inf", "NaN",
	"\uff11Gi",      // fullwidth digit
	"1\u0413\u0431", // a Cyrillic unit
	"1\u200bGi",     // zero-width space inside the number
}

func FuzzParseImageSize(f *testing.F) {
	for _, imageSize := range append(fuzzImageSizeSeeds, malformedImageSizes...) {
		f.Add(imageSize)
	}

	f.Fuzz(func(t *testing.T, imageSize string) {
		if len(imageSize) > 64<<10 {
			t.Skip("oversized input")
		}

		quantity, err := parseImageSize(imageSize)
		if err != nil {
			// The value comes from the environment; the error has to name it.
			if !strings.Contains(err.Error(), fmt.Sprintf("%q", imageSize)) {
				t.Fatalf("image size %q: error does not name the offending value: %v", imageSize, err)
			}
			return
		}

		// Above int64 resource.Quantity is on its own: "1e21" prints as "1e21"
		// but Value() is 0, the same number written out in digits prints as
		// "1", and 1e19 has a negative Value(). The importer sizes the target
		// through Value(), so that range is recorded in FUZZING.md rather than
		// asserted here.
		if quantity.Sign() < 0 || quantity.Cmp(*resource.NewQuantity(math.MaxInt64, resource.DecimalSI)) > 0 {
			return
		}
		if quantity.Value() < 0 {
			t.Fatalf("image size %q: non-negative quantity %s has Value() %d", imageSize, quantity.String(), quantity.Value())
		}

		// An accepted size must survive its own canonical form: the importer
		// logs and compares quantities through String().
		again, err := parseImageSize(quantity.String())
		if err != nil {
			t.Fatalf("image size %q: canonical form %q is rejected: %v", imageSize, quantity.String(), err)
		}
		if again.Cmp(quantity) != 0 {
			t.Fatalf("image size %q: canonical form %q parses to %s, not %s", imageSize, quantity.String(), again.String(), quantity.String())
		}
	})
}
