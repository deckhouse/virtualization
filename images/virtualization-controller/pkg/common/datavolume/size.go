package datavolume

import (
	"k8s.io/apimachinery/pkg/api/resource"
)

// AdjustPVCSize calculates increased PVC size to feat disk image onto scratch PVC.
//
// Virtualization-controller calculates unpacked size while importing image
// into DVCR. This unpacked size is used to create PVC if size is not specified,
// i.e. VirtualMachineImage with storage: Kubernetes.
// The unpacked size is no enough, as CDI uses scratch PVC with Filesystem mode
// to unpack disk image from dvcr image.
//
// A quote from Kubernetes documentation:
// A volume with volumeMode: Filesystem is mounted into Pods into a directory.
// If the volume is backed by a block device and the device is empty, Kubernetes
// creates a filesystem on the device before mounting it for the first time.
//
// That is why virtualization-controller increases PVC size to feat disk image.
// There is no strict formula for ext4 filesystem overhead, so these ratios are from experiments:
// - add 25% for size < 512Mi
// - add 15% for size < 4096Mi
// - add 10% for size >= 4096Mi
//
// Also, increased size is aligned to MiB by rounding up.
func AdjustPVCSize(in resource.Quantity) resource.Quantity {
	size := int64(adjustRatio(in.Value()) * float64(in.Value()))

	// Align to MiB and round up.
	mibs := (size / 1024 / 1024) + 1

	return *resource.NewQuantity(mibs*1024*1024, resource.BinarySI)
}

const (
	Size512Mi  = 512 * 1024 * 1024
	Size4096Mi = 4096 * 1024 * 1024
)

// adjustRatio returns a ratio for size adjustment.
func adjustRatio(size int64) float64 {
	switch {
	case size < Size512Mi:
		return 1.25
	case size < Size4096Mi:
		return 1.15
	}
	return 1.1
}
