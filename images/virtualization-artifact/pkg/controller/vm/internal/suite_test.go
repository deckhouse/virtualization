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
	"reflect"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/fake"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestVirtualMachine(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VirtualMachine Handlers Suite")
}

func setupEnvironment(vm *virtv2.VirtualMachine, objs ...client.Object) (client.WithWatch, *reconciler.Resource[*virtv2.VirtualMachine, virtv2.VirtualMachineStatus], state.VirtualMachineState) {
	GinkgoHelper()
	Expect(vm).ToNot(BeNil())
	allObjects := []client.Object{vm}
	allObjects = append(allObjects, objs...)

	fakeClient, err := testutil.NewFakeClientWithObjects(allObjects...)
	Expect(err).NotTo(HaveOccurred())

	resource := reconciler.NewResource(client.ObjectKeyFromObject(vm), fakeClient,
		func() *virtv2.VirtualMachine {
			return &virtv2.VirtualMachine{}
		},
		func(obj *virtv2.VirtualMachine) virtv2.VirtualMachineStatus {
			return obj.Status
		})
	err = resource.Fetch(context.Background())
	Expect(err).NotTo(HaveOccurred())

	var rtObjects []runtime.Object
	for _, obj := range objs {
		if reflect.ValueOf(obj).IsNil() {
			continue
		}

		if obj.GetObjectKind().GroupVersionKind().Group == virtv2.SchemeGroupVersion.Group {
			if rtObj, ok := obj.(runtime.Object); ok {
				rtObjects = append(rtObjects, rtObj)
			}
		}
	}

	fakeVirtClient := fake.NewSimpleClientset(rtObjects...)
	vmState := state.New(fakeClient, fakeVirtClient.VirtualizationV1alpha2(), resource)

	return fakeClient, resource, vmState
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

func newEmptyPOD(name, namespace, vmName string) *corev1.Pod {
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				virtv1.VirtualMachineNameLabel: vmName,
			},
		},
	}
}
