package common

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
