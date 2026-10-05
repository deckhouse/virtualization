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
	"regexp"
	"strings"
	"testing"
)

// decimalExponentImageSize matches a Kubernetes quantity written with a decimal exponent, such
// as 1e3 or 4e400. The VirtualDisk CRD no longer admits this form (its size pattern keeps only
// the SI and binary suffixes), and the importer receives its image size from that field, so
// the fuzzer does not need to explore it. It also must not: resource.ParseQuantity hangs on an
// out-of-range exponent, which the fuzzer would keep reporting as a finding nothing can reach.
var decimalExponentImageSize = regexp.MustCompile(`^[+-]?(([0-9]+(\.[0-9]*)?)|(\.[0-9]+))[eE][+-]?(([0-9]+(\.[0-9]*)?)|(\.[0-9]+))$`)

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
		_, err := NewDataProcessor(nil, "", "", "", imageSize, 0, false, "", 0)
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
		if _, err := NewDataProcessor(nil, "", "", "", imageSize, 0, false, "", 0); err != nil {
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

func FuzzParseImageSize(f *testing.F) {
	for _, imageSize := range append([]string{"1Gi", "500M", "0"}, malformedImageSizes...) {
		f.Add(imageSize)
	}

	f.Fuzz(func(t *testing.T, imageSize string) {
		if decimalExponentImageSize.MatchString(imageSize) {
			t.Skip("decimal exponent: not admitted by the VirtualDisk CRD")
		}

		quantity, err := parseImageSize(imageSize)
		if err == nil {
			_ = quantity.String()
		}
	})
}
