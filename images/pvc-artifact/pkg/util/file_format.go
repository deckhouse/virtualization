/*
Copyright 2018 The CDI Authors.
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

package util

import "os"

func GetFormat(path string) (string, error) {
	const (
		formatQcow2 = "qcow2"
		formatRaw   = "raw"
	)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return formatQcow2, nil
		}
		return "", err
	}
	mode := info.Mode()
	if mode&os.ModeDevice != 0 {
		return formatRaw, nil
	}
	return formatQcow2, nil
}
