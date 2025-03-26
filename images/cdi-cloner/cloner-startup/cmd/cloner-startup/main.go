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
	"log/slog"
	"os"

	"cloner-startup/internal/helpers"
)

func main() {
	var uploadBytes uint64

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	volumeMode, err := helpers.GetEnv("VOLUME_MODE")
	if err != nil {
		logger.Error("Failed to get env VOLUME_MODE", slog.String("error", err.Error()))
		os.Exit(1)
	}

	mountPoint, err := helpers.GetEnv("MOUNT_POINT")
	if err != nil {
		logger.Error("Failed to get env MOUNT_POINT", slog.String("error", err.Error()))
		os.Exit(1)
	}

	preallocation, err := helpers.GetBoolEnv("PREALLOCATION")
	if err != nil {
		logger.Error("Failed to get env PREALLOCATION", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info(fmt.Sprintf(
		"VOLUME_MODE=%s\n"+
			"MOUNT_POINT=%s\n"+
			"PREALLOCATION=%v",
		volumeMode,
		mountPoint,
		preallocation))

	if volumeMode == "block" {
		uploadBytes, err = helpers.GetBlockDeviceSize(mountPoint)
		if err != nil {
			logger.Error("Block size calculation failed", slog.String("error", err.Error()))
			os.Exit(1)
		}

		logger.Info(fmt.Sprintf("Start clone block with %s", helpers.FormatBytes(float64(uploadBytes))))

		if err = helpers.RunCloner("blockdevice-clone", uploadBytes, mountPoint); err != nil {
			logger.Error("Error running cdi-cloner", slog.String("error", err.Error()))
			os.Exit(1)
		}
	} else {
		// Check if directory accesseble
		if err := os.Chdir(mountPoint); err != nil {
			logger.Error("Mount point access failed", slog.String("error", err.Error()))
			os.Exit(1)
		}

		totalBytes, totalUsedBytes, err := helpers.GetDirectorySize(".")
		if err != nil {
			logger.Error("Directory size calculation failed", slog.String("error", err.Error()))
			os.Exit(1)
		}

		if preallocation {
			uploadBytes = totalBytes
			logger.Info("Preallocating filesystem, uploading all bytes")
		} else {
			uploadBytes = totalUsedBytes
			logger.Info("Not preallocating filesystem, get only used blocks in bytes")
		}

		logger.Info(fmt.Sprintf("Start clone with %d bytes", uploadBytes))
		logger.Info(fmt.Sprintf("Start clone with %s", helpers.FormatBytes(float64(uploadBytes))))

		if err = helpers.RunCloner("filesystem-clone", uploadBytes, mountPoint); err != nil {
			logger.Error("Error running cdi-cloner", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}
}
