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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

// VMSyncDeniedByWebhook fails the spec when the KVVM sync of a namespace VM
// is rejected by an admission webhook: the controller retries the same denied
// patch forever, so nothing the spec waits for on this VM can ever happen.
func VMSyncDeniedByWebhook(vms Client[*v1alpha2.VirtualMachine], namespace string) FailFast {
	return New("VirtualMachine "+namespace+"/", vms, func(vm *v1alpha2.VirtualMachine) *Finding {
		cond, _ := conditions.GetCondition(vmcondition.TypeConfigurationApplied, vm.Status.Conditions)
		if cond.Status != metav1.ConditionFalse || !strings.Contains(cond.Message, "admission webhook") {
			return nil
		}
		return &Finding{
			Message: "KVVM sync is denied by an admission webhook, the controller cannot proceed: " + cond.Message,
			Grace:   defaultGrace,
		}
	})
}
