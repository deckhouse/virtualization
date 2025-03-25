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

	"cloner-startup/internal/helpers"
)

func main() {
	volumeMode, err := helpers.GetEnv("VOLUME_MODE")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	mountPoint, err := helpers.GetEnv("MOUNT_POINT")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	preallocation, err := helpers.GetBoolEnv("PREALLOCATION")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("VOLUME_MODE=%s\n", volumeMode)
	fmt.Printf("MOUNT_POINT=%s\n", mountPoint)
	fmt.Printf("PREALLOCATION=%v\n", preallocation)

	if volumeMode == "block" {
		uploadBytes, err := helpers.GetBlockSize(mountPoint)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Block size calculation failed: %v\n", err)
			os.Exit(1)
		}
		helpers.RunCloner("blockdevice-clone", uploadBytes, mountPoint)
	} else {
		// Directory handling with safe context management
		currentDir, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Directory context error: %v\n", err)
			os.Exit(1)
		}
		defer os.Chdir(currentDir)

		if err := os.Chdir(mountPoint); err != nil {
			fmt.Fprintf(os.Stderr, "Mount point access failed: %v\n", err)
			os.Exit(1)
		}

		if preallocation {
			fmt.Println("Preallocating filesystem, uploading all bytes")
		} else {
			fmt.Println("Not preallocating, uploading used bytes only")
		}

		uploadBytes, err := helpers.GetDirectorySize(".", preallocation)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Directory size calculation failed: %v\n", err)
			os.Exit(1)
		}
		helpers.RunCloner("filesystem-clone", uploadBytes, mountPoint)
	}
}
