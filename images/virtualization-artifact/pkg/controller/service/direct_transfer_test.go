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

package service

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

func TestDirectTransferEligible(t *testing.T) {
	block := ptr.To(corev1.PersistentVolumeBlock)
	fs := ptr.To(corev1.PersistentVolumeFilesystem)

	cases := []struct {
		name       string
		format     string
		volumeMode *corev1.PersistentVolumeMode
		want       bool
	}{
		{"raw into block is streamed directly", "raw", block, true},
		{"qcow2 into filesystem is streamed directly", "qcow2", fs, true},
		{"qcow2 into block needs conversion", "qcow2", block, false},
		{"raw into filesystem needs conversion", "raw", fs, false},
		{"unknown format into block falls back to scratch", "", block, false},
		{"unknown format into filesystem falls back to scratch", "", fs, false},
		{"iso into block is never direct", "iso", block, false},
		{"iso into filesystem is never direct", "iso", fs, false},
		{"nil volume mode falls back to scratch", "raw", nil, false},
	}

	for _, tc := range cases {
		if got := directTransferEligible(tc.format, tc.volumeMode); got != tc.want {
			t.Errorf("%s: directTransferEligible(%q) = %v, want %v", tc.name, tc.format, got, tc.want)
		}
	}
}
