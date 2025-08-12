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

package featuregates

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"k8s.io/component-base/featuregate"
)

var defaultFeatures = []string{"AllAlpha", "AllBeta"}

func TestDefault(t *testing.T) {
	require.NotNil(t, instance)
	require.NotNil(t, addFlags)

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	AddFlags(fs)
	err := fs.Parse([]string{
		fmt.Sprintf("--feature-gates=%s=true", string(SDN)),
	})
	require.NoError(t, err)
	require.True(t, Default().Enabled(SDN))

	testKnownFeatures(t, Default())
}

func TestNew(t *testing.T) {
	gate, addFlagsFunc, setFromMapFunc, err := New()
	require.NoError(t, err)
	require.NotNil(t, gate)
	require.NotNil(t, addFlagsFunc)
	require.NotNil(t, setFromMapFunc)

	err = setFromMapFunc(map[string]bool{
		string(SDN): true,
	})
	require.NoError(t, err)

	testKnownFeatures(t, gate)
	require.True(t, gate.Enabled(SDN))
}

func testKnownFeatures(t *testing.T, gate featuregate.FeatureGate) {
	t.Helper()
	known := gate.KnownFeatures()
	require.Len(t, known, 1+len(defaultFeatures))
	for _, featureStr := range known {
		parts := strings.Split(featureStr, "=")
		require.NotEmpty(t, parts)
		feature := parts[0]

		if slices.Contains(defaultFeatures, feature) {
			continue
		}

		require.Equal(t, feature, string(SDN))
	}
}
