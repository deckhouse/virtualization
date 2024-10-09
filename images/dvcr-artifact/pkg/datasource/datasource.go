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

package datasource

import "io"

const (
	// containerDiskImageDir - Expected disk image location in container image as described in
	// https://github.com/kubevirt/kubevirt/blob/v1.3.1/docs/container-register-disks.md
	containerDiskImageDir = "disk"
)

type DataSourceInterface interface {
	Filename() (string, error)
	Length() (int, error)
	ReadCloser() (io.ReadCloser, error)
	Close() error
}
