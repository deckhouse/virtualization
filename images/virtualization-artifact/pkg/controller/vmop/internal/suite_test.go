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

package internal

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
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestVmopHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VMOP handlers Suite")
}

func setupEnvironment(vmop *virtv2.VirtualMachineOperation, objs ...client.Object) (client.WithWatch, *reconciler.Resource[*virtv2.VirtualMachineOperation, virtv2.VirtualMachineOperationStatus], state.VMOperationState) {
	GinkgoHelper()
	Expect(vmop).ToNot(BeNil())

	allObjects := make([]client.Object, len(objs)+1)
	allObjects[0] = vmop
	for i, obj := range objs {
		allObjects[i+1] = obj
	}

	fakeClient, err := testutil.NewFakeClientWithObjects(allObjects...)
	Expect(err).NotTo(HaveOccurred())

	key := types.NamespacedName{
		Name:      vmop.GetName(),
		Namespace: vmop.GetNamespace(),
	}
	srv := reconciler.NewResource(key, fakeClient,
		func() *virtv2.VirtualMachineOperation {
			return &virtv2.VirtualMachineOperation{}
		},
		func(obj *virtv2.VirtualMachineOperation) virtv2.VirtualMachineOperationStatus {
			return obj.Status
		})
	err = srv.Fetch(context.Background())
	Expect(err).NotTo(HaveOccurred())

	s := state.New(fakeClient, srv)

	return fakeClient, srv, s
}

func newSimpleMigration(name, namespace, vm string) *virtv1.VirtualMachineInstanceMigration {
	return &virtv1.VirtualMachineInstanceMigration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: virtv1.SchemeGroupVersion.String(),
			Kind:       "VirtualMachineInstanceMigration",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: virtv1.VirtualMachineInstanceMigrationSpec{
			VMIName: vm,
		},
	}
}
