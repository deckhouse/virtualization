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

package object

import (
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewMinimalVM(prefix, namespace string, opts ...vm.Option) *v1alpha2.VirtualMachine {
	baseOpts := []vm.Option{
		vm.WithGenerateName(prefix),
		vm.WithNamespace(namespace),
		vm.WithCPU(1, ptr.To("100%")),
		vm.WithMemory(*resource.NewQuantity(Mi256, resource.BinarySI)),
		vm.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vm.WithProvisioningUserData(DefaultCloudInit),
	}
	baseOpts = append(baseOpts, opts...)
	return vm.New(baseOpts...)
}
