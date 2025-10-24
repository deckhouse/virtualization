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

package framework

import (
	"os"

	"github.com/onsi/ginkgo/v2"
)

// Ginkgo decorators helpers:
// - Common decorators for e2e: Ordered.
// - ContinueOnFailure decorator is switchable and can be enabled with CONTINUE_ON_FAILURE=yes env.
//
// A quote from Ginkgo documentation:
// Moreover, Ginkgo also supports passing in arbitrarily nested slices of decorators.
// Ginkgo will unroll these slices and process the flattened list. This makes it easier
// to pass around groups of decorators. For example, this is valid:
// markFlaky := []interface{}{Label("flaky"), FlakeAttempts(3)}
// var _ = Describe("a bunch of flaky controller tests", markFlaky, Label("controller"), func() {
//  ...
// }
// The resulting tests will be decorated with FlakeAttempts(3) and the two labels flaky and controller.
//
// This helper uses this "flattening" feature, so DecoratorsFromEnv implements
// dynamic list of switchable decorators by returning an array of decorators.

type EnvSwitchable interface {
	Decorator() interface{}
}

func DecoratorsFromEnv(decorators ...interface{}) []interface{} {
	out := make([]interface{}, 0)

	for _, decorator := range decorators {
		switch v := decorator.(type) {
		case EnvSwitchable:
			gdeco := v.Decorator()
			if gdeco != nil {
				out = append(out, gdeco)
			}
		default:
			out = append(out, decorator)
		}
	}

	return out
}

const ContinueOnFailureEnv = "CONTINUE_ON_FAILURE"

type FailureBehaviourEnvSwitcher struct{}

func (f FailureBehaviourEnvSwitcher) Decorator() interface{} {
	if f.IsContinueOnFailure() {
		return ginkgo.ContinueOnFailure
	}
	return nil
}

// IsContinueOnFailure returns true if Continue on error is enabled.
func (f FailureBehaviourEnvSwitcher) IsContinueOnFailure() bool {
	return os.Getenv(ContinueOnFailureEnv) == "yes"
}

// CommonE2ETestDecorators returns common decorators for e2e tests: Ordered and ContinueOnFailure switchable with env.
func CommonE2ETestDecorators() []interface{} {
	return DecoratorsFromEnv(
		ginkgo.Ordered,
		FailureBehaviourEnvSwitcher{},
	)
}
