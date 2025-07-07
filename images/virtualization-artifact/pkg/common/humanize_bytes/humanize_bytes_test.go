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

package humanize_bytes //nolint:stylecheck,nolintlint

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHumanizeIBytes(t *testing.T) {
	type testCase struct {
		in  uint64
		out string
	}

	testCases := []testCase{
		{
			in:  0,
			out: "0B",
		},
		{
			in:  1,
			out: "1B",
		},
		{
			in:  2,
			out: "2B",
		},
		{
			in:  2 * 1024,
			out: "2Ki",
		},
		{
			in:  2*1024 + 1,
			out: "2Ki",
		},
		{
			in:  2*1024 - 1,
			out: "2Ki",
		},
		{
			in:  2 * 1024 * 1024,
			out: "2Mi",
		},
		{
			in:  2*1024*1024 + 1,
			out: "2Mi",
		},
		{
			in:  2*1024*1024 - 1,
			out: "2Mi",
		},
	}

	for _, tc := range testCases {
		require.Equal(t, tc.out, HumanizeIBytes(tc.in))
	}
}
