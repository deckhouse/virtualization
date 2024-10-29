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

package ginkgoutil

import (
	"os"

	"github.com/onsi/ginkgo/v2"
)

type EnvSwitchable interface {
	Decorator() interface{}
}

func DecoratorsFromEnv(decorators ...interface{}) []interface{} {
	out := make([]interface{}, 0)

	for _, decorator := range decorators {
		switch decorator.(type) {
		case EnvSwitchable:
			gdeco := decorator.(EnvSwitchable).Decorator()
			if gdeco != nil {
				out = append(out, gdeco)
			}
		default:
			out = append(out, decorator)
		}
	}

	return out
}

const StopOnFailureEnv = "STOP_ON_FAILURE"

type FailureBehaviourEnvSwitcher struct{}

func (c FailureBehaviourEnvSwitcher) Decorator() interface{} {
	if !c.IsStopOnFailure() {
		return ginkgo.ContinueOnFailure
	}
	return nil
}

// IsStopOnFailure returns true if Stop on error is enabled.
func (c FailureBehaviourEnvSwitcher) IsStopOnFailure() bool {
	return os.Getenv(StopOnFailureEnv) == "yes"
}

// CommonE2ETestDecorators returns common decorators for e2e tests: Ordered and ContinueOnFailure switchable with env.
func CommonE2ETestDecorators() []interface{} {
	return DecoratorsFromEnv(
		ginkgo.Ordered,
		FailureBehaviourEnvSwitcher{},
	)
}
