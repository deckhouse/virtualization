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

package registry

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"sort"
	"strings"

	"go.cypherpunks.ru/gogost/v5/gost34112012256"
	"go.cypherpunks.ru/gogost/v5/gost34112012512"

	importerrs "github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/errors"
)

// Checksum algorithms supported for the HTTP data source. Keys match the field
// names of the checksum section in the resource spec.
var checksumAlgorithms = map[string]func() hash.Hash{
	"md5":         md5.New,
	"sha1":        sha1.New,
	"sha256":      sha256.New,
	"sha512":      sha512.New,
	"streebog256": gost34112012256.New,
	"streebog512": gost34112012512.New,
}

// ParseChecksums parses the algorithm:sum pairs an importer Pod receives in a
// single environment variable, e.g. "sha256:78be...,streebog512:8e94...".
func ParseChecksums(value string) (map[string]string, error) {
	if value == "" {
		return nil, nil
	}

	checksums := make(map[string]string)

	for _, pair := range strings.Split(value, ",") {
		algorithm, sum, found := strings.Cut(pair, ":")
		if !found || algorithm == "" || sum == "" {
			return nil, fmt.Errorf("malformed checksum %q, expected the algorithm:sum format", pair)
		}

		if _, ok := checksumAlgorithms[algorithm]; !ok {
			return nil, fmt.Errorf("unsupported checksum algorithm %q, supported algorithms are %s", algorithm, SupportedChecksumAlgorithms())
		}

		checksums[algorithm] = strings.ToLower(sum)
	}

	return checksums, nil
}

// newChecksumVerifiers prepares one hash per checksum given in the spec. The
// writers have to be fed with the data of the source, and every returned check
// reports whether its algorithm agrees with the expected sum once the data has
// been read to the end.
func newChecksumVerifiers(checksums map[string]string) ([]io.Writer, []func() error) {
	var (
		writers []io.Writer
		checks  []func() error
	)

	for _, algorithm := range sortedChecksumAlgorithms(checksums) {
		expectedSum := checksums[algorithm]
		hash := checksumAlgorithms[algorithm]()
		writers = append(writers, hash)
		checks = append(checks, func() error {
			sum := hex.EncodeToString(hash.Sum(nil))
			if sum != expectedSum {
				return importerrs.NewBadImageChecksumError(expectedSum, sum, algorithm)
			}

			return nil
		})
	}

	return writers, checks
}

// SupportedChecksumAlgorithms lists algorithm names for error messages.
func SupportedChecksumAlgorithms() string {
	return strings.Join(sortedChecksumAlgorithms(checksumAlgorithms), ", ")
}

// sortedChecksumAlgorithms keeps the order of the hash calculation and of the
// error messages stable regardless of the map iteration order.
func sortedChecksumAlgorithms[V any](algorithms map[string]V) []string {
	names := make([]string, 0, len(algorithms))
	for name := range algorithms {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}
