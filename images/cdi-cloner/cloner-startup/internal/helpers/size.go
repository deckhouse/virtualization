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

package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	/*
		It is a standardized ioctl request code that queries the size of a block device (e.g., disks, partitions) directly from the kernel

		Structure of ioctl Codes :

		Data Type : 0x8008 (upper 16 bits) indicates:
			Direction : Read from the kernel (0x80).
			Size : 8 bytes (64 bits), matching the uint64 return type
		Magic Number : 0x12 (lower 8 bits of the third byte) identifies this as a block device ioctl
		Command Number : 0x72 (lower 8 bits of the fourth byte) specifies the exact operation (size query)

	*/
	BLKGETSIZE64 = 0x80081272
)

func GetBlockSize(device string) (uint64, error) {
	fd, err := unix.Open(device, unix.O_RDONLY, 0)
	if err != nil {
		return 0, fmt.Errorf("open device %s: %w", device, err)
	}
	defer unix.Close(fd)

	var size uint64
	_, _, errno := unix.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		uintptr(BLKGETSIZE64),
		uintptr(unsafe.Pointer(&size)),
	)
	if errno != 0 {
		return 0, fmt.Errorf("get size for block device %s: %w", device, errno)
	}
	return size, nil
}

// getDirectorySize calculates directory size using Go's filepath.Walk
func GetDirectorySize(path string, preallocate bool) (uint64, error) {
	var total uint64

	err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if preallocate {
			total += uint64(info.Size())
		} else {
			if stat, ok := info.Sys().(*syscall.Stat_t); ok {
				total += uint64(stat.Blocks * 512) // 512-byte blocks
			}
		}
		return nil
	})
	return total, err
}
