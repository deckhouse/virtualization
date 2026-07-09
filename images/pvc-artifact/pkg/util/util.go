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

// This file was initially copied from the Containerized Data Importer (CDI)
// project (https://github.com/kubevirt/containerized-data-importer) and adapted
// for the virtualization module.

package util

import (
	"bufio"
	"encoding/base64"
	"io"
	"math"
	"os"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"kubevirt.io/containerized-data-importer/pkg/common"
)

const (
	blockdevFileName = "/usr/sbin/blockdev"
	// DefaultAlignBlockSize is the alignment size we use to align disk images, its a multiple of all known hardware block sizes 512/4k/8k/32k/64k.
	DefaultAlignBlockSize = 1024 * 1024
)

// CountingReader is a reader that keeps track of how much has been read.
type CountingReader struct {
	Reader  io.ReadCloser
	Current uint64
	Done    bool
}

// ParseEnvVar provides a wrapper to attempt to fetch the specified env var.
func ParseEnvVar(envVarName string, decode bool) (string, error) {
	value := os.Getenv(envVarName)
	if decode {
		v, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return "", errors.Errorf("error decoding environment variable %q", envVarName)
		}
		value = string(v)
	}
	return value, nil
}

func (r *CountingReader) Read(p []byte) (n int, err error) {
	n, err = r.Reader.Read(p)
	r.Current += uint64(n)
	r.Done = errors.Is(err, io.EOF)
	return n, err
}

func (r *CountingReader) Close() error {
	return r.Reader.Close()
}

// GetAvailableSpaceByVolumeMode calls another method based on the volumeMode parameter to get the amount of available space.
func GetAvailableSpaceByVolumeMode(volumeMode corev1.PersistentVolumeMode) (int64, error) {
	if volumeMode == corev1.PersistentVolumeBlock {
		return GetAvailableSpaceBlock(common.WriteBlockPath)
	}
	return GetAvailableSpace(common.ImporterVolumePath)
}

// MinQuantity calculates the minimum of two quantities.
func MinQuantity(availableSpace, imageSize *resource.Quantity) resource.Quantity {
	if imageSize.Cmp(*availableSpace) == 1 {
		return *availableSpace
	}
	return *imageSize
}

// WriteTerminationMessage writes the passed in message to the default termination message file.
func WriteTerminationMessage(message string) error {
	return WriteTerminationMessageToFile(common.PodTerminationMessageFile, message)
}

// WriteTerminationMessageToFile writes the passed in message to the passed in message file.
func WriteTerminationMessageToFile(file, message string) error {
	message = strings.ReplaceAll(message, "\n", " ")
	scanner := bufio.NewScanner(strings.NewReader(message))

	if scanner.Scan() {
		if err := os.WriteFile(file, scanner.Bytes(), 0o600); err != nil {
			return errors.Wrap(err, "could not create termination message file")
		}
	}
	return nil
}

// RoundDown returns the number rounded down to the nearest multiple.
func RoundDown(number, multiple int64) int64 {
	return number / multiple * multiple
}

// RoundUp returns the number rounded up to the nearest multiple.
func RoundUp(number, multiple int64) int64 {
	partitions := math.Ceil(float64(number) / float64(multiple))
	return int64(partitions) * multiple
}

// GetUsableSpace calculates usable space to use taking file system overhead into account.
func GetUsableSpace(filesystemOverhead float64, availableSpace int64) int64 {
	spaceWithOverhead := int64(math.Ceil((1 - filesystemOverhead) * float64(availableSpace)))
	return RoundDown(spaceWithOverhead, DefaultAlignBlockSize)
}
