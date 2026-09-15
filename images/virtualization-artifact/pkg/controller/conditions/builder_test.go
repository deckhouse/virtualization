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

package conditions

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

type testConditionType string

func (t testConditionType) String() string {
	return string(t)
}

func TestBuilderMessageIsBounded(t *testing.T) {
	t.Run("a message within the limit is kept as is", func(t *testing.T) {
		msg := strings.Repeat("a", maxMessageLen)

		got := NewConditionBuilder(testConditionType("Test")).Message(msg).Condition().Message

		require.Equal(t, msg, got)
	})

	t.Run("a longer message is cut down to the limit", func(t *testing.T) {
		msg := strings.Repeat("a", maxMessageLen+1)

		got := NewConditionBuilder(testConditionType("Test")).Message(msg).Condition().Message

		require.Len(t, got, maxMessageLen)
		require.True(t, strings.HasSuffix(got, "..."))
	})

	t.Run("a cut message stays valid utf-8", func(t *testing.T) {
		// The cut lands in the middle of the last two-byte rune.
		msg := strings.Repeat("é", maxMessageLen/2+1)

		got := TruncateMessage(msg)

		require.True(t, utf8.ValidString(got))
		require.LessOrEqual(t, len(got), maxMessageLen)
		require.True(t, strings.HasSuffix(got, "..."))
	})
}
