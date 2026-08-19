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

package failfast

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// VirtualDisks fails the spec when a VirtualDisk of the namespace enters a
// terminal phase: every wait that depends on the disk (provisioning, VM boot,
// agent readiness) is doomed.
func VirtualDisks(vds Client[*v1alpha2.VirtualDisk], namespace string) FailFast {
	return New("VirtualDisk "+namespace+"/", vds, func(vd *v1alpha2.VirtualDisk) *Finding {
		if vd.Status.Phase != v1alpha2.DiskFailed && vd.Status.Phase != v1alpha2.DiskLost {
			return nil
		}
		return &Finding{
			Message: fmt.Sprintf("entered the terminal %s phase: %s",
				vd.Status.Phase, failureDiagnostics(vd.Status.Conditions)),
			Grace: defaultGrace,
		}
	})
}

// VirtualImages fails the spec when a VirtualImage of the namespace enters a
// terminal phase.
func VirtualImages(vis Client[*v1alpha2.VirtualImage], namespace string) FailFast {
	return New("VirtualImage "+namespace+"/", vis, func(vi *v1alpha2.VirtualImage) *Finding {
		switch vi.Status.Phase {
		case v1alpha2.ImageFailed, v1alpha2.ImageLost, v1alpha2.ImagePVCLost:
		default:
			return nil
		}
		return &Finding{
			Message: fmt.Sprintf("entered the terminal %s phase: %s",
				vi.Status.Phase, failureDiagnostics(vi.Status.Conditions)),
			Grace: defaultGrace,
		}
	})
}

// failureDiagnostics renders the failing conditions of a resource, ready to
// be appended to a fail-fast message.
func failureDiagnostics(conds []metav1.Condition) string {
	parts := make([]string, 0, len(conds))
	for _, cond := range conds {
		if cond.Status == metav1.ConditionFalse && cond.Message != "" {
			parts = append(parts, fmt.Sprintf("%s=%s %s: %s", cond.Type, cond.Status, cond.Reason, cond.Message))
		}
	}
	if len(parts) == 0 {
		return "no diagnostics in conditions"
	}
	return strings.Join(parts, "; ")
}
