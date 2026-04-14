//go:build !linux

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

import "fmt"

func GetBlockDeviceSize(device string) (uint64, error) {
	_ = device
	return 0, fmt.Errorf("block device size detection is supported only on linux")
}

func GetDirectorySize(path string) (uint64, uint64, error) {
	_ = path
	return 0, 0, fmt.Errorf("directory size calculation is supported only on linux in this helper")
}

func FormatBytes(s float64) string {
	base := 1024.0
	sizes := []string{"B", "kB", "MB", "GB", "TB", "PB", "EB"}

	unitsLimit := len(sizes)
	i := 0
	for s >= base && i < unitsLimit {
		s /= base
		i++
	}

	f := "%.0f %s"
	if i > 1 {
		f = "%.2f %s"
	}

	return fmt.Sprintf(f, s, sizes[i])
}
