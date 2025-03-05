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

package handler

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestWorkloadUpdateHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "WorkloadUpdate Handlers Suite")
}

func setupEnvironment(vm *virtv2.VirtualMachine, objs ...client.Object) (client.WithWatch, *reconciler.Resource[*virtv2.VirtualMachine, virtv2.VirtualMachineStatus]) {
	GinkgoHelper()
	Expect(vm).ToNot(BeNil())
	allObjects := []client.Object{vm}
	allObjects = append(allObjects, objs...)

	fakeClient, err := testutil.NewFakeClientWithObjects(allObjects...)
	Expect(err).NotTo(HaveOccurred())

	key := types.NamespacedName{
		Name:      vm.GetName(),
		Namespace: vm.GetNamespace(),
	}
	resource := reconciler.NewResource(key, fakeClient,
		func() *virtv2.VirtualMachine {
			return &virtv2.VirtualMachine{}
		},
		func(obj *virtv2.VirtualMachine) virtv2.VirtualMachineStatus {
			return obj.Status
		})
	err = resource.Fetch(context.Background())
	Expect(err).NotTo(HaveOccurred())

	return fakeClient, resource
}

func newEmptyKVVMI(name, namespace string) *virtv1.VirtualMachineInstance {
	return &virtv1.VirtualMachineInstance{
		TypeMeta: metav1.TypeMeta{
			APIVersion: virtv1.GroupVersion.String(),
			Kind:       "VirtualMachineInstance",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}
