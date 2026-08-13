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
	"testing"
)

// BenchmarkChecksumAlgorithms measures the throughput of every supported
// algorithm, which is what the user guide states next to the algorithm table.
// The numbers depend on the processor: SHA-1 and SHA-256 are computed with
// dedicated instructions when the processor has them (sha_ni on x86-64, the
// crypto extension on arm64), while Streebog is pure Go on every platform.
//
// Pin the benchmark to a single performance core, otherwise a hybrid processor
// schedules it on an efficiency core half of the time and the results scatter:
//
//	taskset -c 0 go test -run xxx -bench ChecksumAlgorithms -benchtime 2s -count 5 ./pkg/registry/
func BenchmarkChecksumAlgorithms(b *testing.B) {
	// A chunk large enough to hide the per-call overhead and small enough to
	// stay in the cache, so that the memory bus is not what is measured.
	data := make([]byte, 4<<20)

	for _, algorithm := range sortedChecksumAlgorithms(checksumAlgorithms) {
		b.Run(algorithm, func(b *testing.B) {
			hash := checksumAlgorithms[algorithm]()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				hash.Write(data)
			}

			b.StopTimer()
			hash.Sum(nil)
		})
	}
}
