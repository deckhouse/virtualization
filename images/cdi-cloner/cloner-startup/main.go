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

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

func getEnv(key string) (string, error) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", fmt.Errorf("error getting env var %s", key)
	}
	return value, nil
}

func main() {
	volumeMode, err := getEnv("VOLUME_MODE")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	mountPoint, err := getEnv("MOUNT_POINT")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	boolValue, err := getEnv("PREALLOCATION")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	preallocation, err := strconv.ParseBool(boolValue)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if preallocation {
		fmt.Println("Preallocation enabled")
	} else {
		fmt.Println("Preallocation not enabled")
	}

	fmt.Printf("VOLUME_MODE=%s\n", volumeMode)
	fmt.Printf("MOUNT_POINT=%s\n", mountPoint)

	var uploadBytes uint64
	// var err error

	if volumeMode == "block" {
		uploadBytes, err = getBlockSize(mountPoint)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting block size: %v\n", err)
			os.Exit(1)
		}
		runCloner("blockdevice-clone", uploadBytes, mountPoint)
	} else {
		currentDir, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
			os.Exit(1)
		}
		defer os.Chdir(currentDir)

		if err := os.Chdir(mountPoint); err != nil {
			fmt.Fprintf(os.Stderr, "Error changing directory: %v\n", err)
			os.Exit(1)
		}

		if preallocation {
			fmt.Println("Preallocating filesystem, uploading all bytes")
		} else {
			fmt.Println("Not preallocating filesystem, uploading only used bytes")
		}

		uploadBytes, err = getDirectorySize(".", preallocation)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error calculating directory size: %v\n", err)
			os.Exit(1)
		}
		runCloner("filesystem-clone", uploadBytes, mountPoint)
	}
}

func getBlockSize(device string) (uint64, error) {
	fd, err := unix.Open(device, unix.O_RDONLY, 0)
	if err != nil {
		return 0, fmt.Errorf("open device: %w", err)
	}
	defer unix.Close(fd)
	var (
		size         uint64
		BLKGETSIZE64 = 0x80081272 // BLKGETSIZE64 ioctl command
	)

	// Use BLKGETSIZE64 (0x80081272)
	_, _, errno := unix.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		uintptr(BLKGETSIZE64),
		uintptr(unsafe.Pointer(&size)),
	)
	if errno != 0 {
		return 0, fmt.Errorf("ioctl BLKGETSIZE64: %w", errno)
	}
	return size, nil
}

// getBlockSize uses ioctl to get block device size
// func getBlockSize(device string) (uint64, error) {
// 	fd, err := syscall.Open(device, syscall.O_RDONLY, 0)
// 	if err != nil {
// 		return 0, err
// 	}
// 	defer syscall.Close(fd)

// 	var size uint64
// 	// BLKGETSIZE64 ioctl command (0x80081272)
// 	// if err := syscall.Ioctl(fd, 0x80081272, uintptr(unsafe.Pointer(&size))); err != nil {
// 	if err := unix.Ioctl(fd, unix.BLK, uintptr(unsafe.Pointer(&size))); err != nil {
// 		// if err := unix.Ioctl(fd, 0x80081272, uintptr(unsafe.Pointer(&size))); err != nil {
// 		return 0, err
// 	}
// 	return size, nil
// }

// getDirectorySize calculates directory size using Go's filepath.Walk
func getDirectorySize(path string, preallocate bool) (uint64, error) {
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

func runCloner(contentType string, uploadBytes uint64, mountPoint string) {
	cmd := exec.Command("/usr/bin/cdi-cloner",
		"-v=3",
		"-alsologtostderr",
		"-content-type="+contentType,
		"-upload-bytes="+strconv.FormatUint(uploadBytes, 10),
		"-mount="+mountPoint,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running cdi-cloner: %v\n", err)
		os.Exit(1)
	}
}
