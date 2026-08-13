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

package datasource

import (
	"sort"
	"strings"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// Checksums collects the checksums set in the checksum section of a data
// source. Algorithm names match the field names of the section, and both the
// importer and the uploader Pod know how to calculate every one of them.
func Checksums(checksum *v1alpha2.Checksum) map[string]string {
	if checksum == nil {
		return nil
	}

	checksums := make(map[string]string)
	for algorithm, sum := range map[string]string{
		"md5":         checksum.MD5,
		"sha1":        checksum.SHA1,
		"sha256":      checksum.SHA256,
		"sha512":      checksum.SHA512,
		"streebog256": checksum.Streebog256,
		"streebog512": checksum.Streebog512,
	} {
		if sum != "" {
			checksums[algorithm] = strings.ToLower(sum)
		}
	}

	if len(checksums) == 0 {
		return nil
	}

	return checksums
}

// FormatChecksums renders the checksums into a single environment variable
// value the Pod parses back, e.g. "sha256:78be...,streebog512:8e94...".
// The order is stable so that the Pod specification does not change on its own.
func FormatChecksums(checksums map[string]string) string {
	algorithms := make([]string, 0, len(checksums))
	for algorithm := range checksums {
		algorithms = append(algorithms, algorithm)
	}
	sort.Strings(algorithms)

	pairs := make([]string, 0, len(algorithms))
	for _, algorithm := range algorithms {
		pairs = append(pairs, algorithm+":"+checksums[algorithm])
	}

	return strings.Join(pairs, ",")
}
