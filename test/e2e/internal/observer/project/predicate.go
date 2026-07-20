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

package project

import (
	dv1alpha2 "github.com/deckhouse/virtualization/test/e2e/internal/api/deckhouse/v1alpha2"
)

// BeDeployed reports that the Project has reached the "Deployed" state, i.e. the
// Project and every resource it renders (namespace, quotas, network policy, ...)
// have been successfully applied.
func BeDeployed() Predicate {
	return func(p *dv1alpha2.Project) (bool, error) {
		if p == nil {
			return false, nil
		}
		return p.Status.State == dv1alpha2.ProjectStateDeployed, nil
	}
}
