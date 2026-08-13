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

package errors

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Test_BadImageChecksumError pins down the order of the constructor arguments:
// the message ends up in the Ready condition of the resource, so swapping them
// leaves the user with a sentence that names neither the algorithm nor the sum
// to fix.
func Test_BadImageChecksumError(t *testing.T) {
	err := NewBadImageChecksumError(
		"2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		"0000000000000000000000000000000000000000000000000000000000000000",
		"sha256",
	)

	require.Equal(t, "BadImageChecksum", err.Reason())
	require.True(t, err.Permanent(), "a checksum mismatch must not be retried")
	require.Equal(t,
		"sha256 sum mismatch: 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824 != 0000000000000000000000000000000000000000000000000000000000000000",
		err.Error(),
	)
}
