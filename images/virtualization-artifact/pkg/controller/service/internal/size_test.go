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

package internal

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestAdjustPVCSize(t *testing.T) {
	tests := []struct {
		name string
		in   int64
		out  int64
	}{
		{
			"zero",
			0,
			0,
		},
		{
			"less than 512Mi",
			100 * 1024 * 1024,
			126 * 1024 * 1024, // 25% + 1Mi
		},
		{
			"less than 4096Mi",
			2000 * 1024 * 1024,
			2301 * 1024 * 1024, // 15% + 1Mi
		},
		{
			"more than 4096Mi",
			10000 * 1024 * 1024,
			11001 * 1024 * 1024, // 10% + 1Mi
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := AdjustImageSize(*resource.NewQuantity(tt.in, resource.BinarySI))
			require.Equal(t, tt.out, res.Value())
		})
	}
}
