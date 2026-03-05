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

package protocol

import "bytes"

func ToDevicePath(path string) [256]byte {
	var result [256]byte
	writeCString(result[:], path)
	return result
}

func ToBusID(busID string) [32]byte {
	var result [32]byte
	writeCString(result[:], busID)
	return result
}

func fromCString(buf []byte) string {
	newBytes := buf
	if ib := bytes.IndexByte(newBytes, 0); ib != -1 {
		newBytes = newBytes[:ib]
	}
	return string(newBytes)
}

func writeCString(dst []byte, s string) {
	for i := range dst {
		dst[i] = 0
	}

	n := len(s)
	if n >= len(dst) {
		n = len(dst) - 1
	}

	copy(dst[:n], s)
}
