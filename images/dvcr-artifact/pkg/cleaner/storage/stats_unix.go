//go:build unix

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

package storage

import (
	"golang.org/x/sys/unix"
)

func FSBytes(dir string) (FSInfo, error) {
	var stat unix.Statfs_t
	err := unix.Statfs(dir, &stat)
	if err != nil {
		return FSInfo{}, err
	}

	// Most of Statfs_t fields are number of blocks, and block size is Bsize.
	return FSInfo{
		Total:     stat.Blocks * uint64(stat.Bsize),
		Available: stat.Bavail * uint64(stat.Bsize),
	}, nil
}
