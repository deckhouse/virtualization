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

package state

import (
	"context"
	"log/slog"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestState(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "State Test Suite")
}

type StateTestArgs struct {
	specRefs   []v1alpha2.BlockDeviceSpecRef
	statusRefs []v1alpha2.BlockDeviceStatusRef
	uniqueRefs int
}

var _ = Describe("State fill check", func() {
	scheme := apiruntime.NewScheme()
	for _, f := range []func(*apiruntime.Scheme) error{
		v1alpha2.AddToScheme,
		virtv1.AddToScheme,
		corev1.AddToScheme,
	} {
		err := f(scheme)
		Expect(err).NotTo(HaveOccurred(), "failed to add scheme: %s", err)
	}

	namespacedName := types.NamespacedName{
		Namespace: "ns",
		Name:      "vm",
	}

	var ctx context.Context
	var vm *v1alpha2.VirtualMachine

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())

		vm = &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      namespacedName.Name,
				Namespace: namespacedName.Namespace,
			},
			Spec: v1alpha2.VirtualMachineSpec{
				BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{},
			},
			Status: v1alpha2.VirtualMachineStatus{
				Phase:           v1alpha2.MachinePending,
				BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{},
			},
		}
	})

	DescribeTable("Checking Forbid events",
		func(args StateTestArgs) {
			vm.Spec.BlockDeviceRefs = args.specRefs
			vm.Status.BlockDeviceRefs = args.statusRefs

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm).Build()
			vmResource := reconciler.NewResource(namespacedName, fakeClient, vmFactoryByVM(vm), vmStatusGetter)

			err := vmResource.Fetch(ctx)
			Expect(err).NotTo(HaveOccurred())

			state := &state{client: fakeClient, vm: vmResource}

			state.fill()

			Expect(state.bdRefs).To(HaveLen(args.uniqueRefs))
		},
		Entry("Should has 3 refs; all non unique", StateTestArgs{
			uniqueRefs: 3,
			specRefs: []v1alpha2.BlockDeviceSpecRef{
				{Kind: v1alpha2.DiskDevice, Name: "vd1"},
				{Kind: v1alpha2.DiskDevice, Name: "vd2"},
				{Kind: v1alpha2.DiskDevice, Name: "vd3"},
			},
			statusRefs: []v1alpha2.BlockDeviceStatusRef{
				{Kind: v1alpha2.DiskDevice, Name: "vd1"},
				{Kind: v1alpha2.DiskDevice, Name: "vd2"},
				{Kind: v1alpha2.DiskDevice, Name: "vd3"},
			},
		}),
		Entry("Should has 3 refs; some of them are unique", StateTestArgs{
			uniqueRefs: 3,
			specRefs: []v1alpha2.BlockDeviceSpecRef{
				{Kind: v1alpha2.DiskDevice, Name: "vd2"},
				{Kind: v1alpha2.DiskDevice, Name: "vd3"},
			},
			statusRefs: []v1alpha2.BlockDeviceStatusRef{
				{Kind: v1alpha2.DiskDevice, Name: "vd1"},
				{Kind: v1alpha2.DiskDevice, Name: "vd2"},
			},
		}),
		Entry("Should has 5 refs; some of them have the different kind", StateTestArgs{
			uniqueRefs: 5,
			specRefs: []v1alpha2.BlockDeviceSpecRef{
				{Kind: v1alpha2.DiskDevice, Name: "vd2"},
				{Kind: v1alpha2.DiskDevice, Name: "vd3"},
				{Kind: v1alpha2.ImageDevice, Name: "vd3"},
			},
			statusRefs: []v1alpha2.BlockDeviceStatusRef{
				{Kind: v1alpha2.DiskDevice, Name: "vd1"},
				{Kind: v1alpha2.ClusterImageDevice, Name: "vd2"},
			},
		}),
	)
})

func vmFactoryByVM(vm *v1alpha2.VirtualMachine) func() *v1alpha2.VirtualMachine {
	return func() *v1alpha2.VirtualMachine {
		return vm
	}
}

func vmStatusGetter(obj *v1alpha2.VirtualMachine) v1alpha2.VirtualMachineStatus {
	return obj.Status
}
