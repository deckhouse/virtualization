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

package api

import (
	"k8s.io/apiserver/pkg/registry/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmpoolstorage "github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vmpool/storage"
)

// installEnterpriseResources registers subresources for paid-edition (EE/SE+)
// features into the aggregated apiserver group. The endpoints are always served;
// availability is governed by the resource's CRD (installed only when the
// VirtualMachinePool feature gate is on) and by the controller that self-gates.
func installEnterpriseResources(resources map[string]rest.Storage, c client.Client) {
	poolStorage := vmpoolstorage.NewStorage(c)
	resources["virtualmachinepools"] = poolStorage
	resources["virtualmachinepools/scaledownwith"] = poolStorage.ScaleDownWithREST()
}
