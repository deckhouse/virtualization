/*
Copyright 2024 Flant JSC

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

package internal

import (
	"k8s.io/apimachinery/pkg/api/resource"
)

// AdjustImageSize calculates increased image size to feat disk image onto scratch PVC.
//
// Virtualization-controller calculates unpacked size while importing image
// into DVCR. This unpacked size is used to create PVC if size is not specified,
// i.e. VirtualImage with storage: Kubernetes.
// The unpacked size is no enough, as CDI uses scratch PVC with Filesystem mode
// to unpack disk image from dvcr image.
//
// A quote from Kubernetes documentation:
// A volume with volumeMode: Filesystem is mounted into Pods into a directory.
// If the volume is backed by a block device and the device is empty, Kubernetes
// creates a filesystem on the device before mounting it for the first time.
//
// That is why virtualization-controller increases image size to feat disk image.
// There is no strict formula for ext4 filesystem overhead, so these ratios are from experiments:
// - return 0 for size == 0
// - add 25% for size < 512Mi
// - add 15% for size < 4096Mi
// - add 10% for size >= 4096Mi
//
// Also, increased size is aligned to MiB by rounding up.
func AdjustImageSize(in resource.Quantity) resource.Quantity {
	if in.IsZero() {
		return in
	}

	size := int64(adjustRatio(in.Value()) * float64(in.Value()))

	// Align to MiB and round up.
	mibs := (size / 1024 / 1024) + 1

	return *resource.NewQuantity(mibs*1024*1024, resource.BinarySI)
}

const (
	Size24Mi   = 24 * 1024 * 1024
	Size512Mi  = 512 * 1024 * 1024
	Size4096Mi = 4096 * 1024 * 1024
)

// adjustRatio returns a ratio for size adjustment.
func adjustRatio(size int64) float64 {
	switch {
	case size < Size24Mi:
		return 1.4
	case size < Size512Mi:
		return 1.25
	case size < Size4096Mi:
		return 1.15
	}
	return 1.1
}
