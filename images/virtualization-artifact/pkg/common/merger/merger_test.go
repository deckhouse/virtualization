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

package merger

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyMapChanges(t *testing.T) {
	type asset struct {
		old map[string]string
		new map[string]string
		res map[string]string
	}

	empty := map[string]string{}
	foo := map[string]string{
		"key": "foo",
	}
	bar := map[string]string{
		"key": "bar",
	}

	cases := []asset{
		{
			foo, empty, empty,
		},
		{
			foo, empty, empty,
		},
		{
			empty, foo, foo,
		},
		{
			foo, bar, bar,
		},
		{
			bar, bar, bar,
		},
	}

	for _, c := range cases {
		from := copyMap(c.old)

		ApplyMapChanges(from, c.old, c.new)
		require.Equal(t, from, c.res)
	}
}

func copyMap(m map[string]string) map[string]string {
	res := make(map[string]string, len(m))

	for k, v := range m {
		res[k] = v
	}

	return res
}
