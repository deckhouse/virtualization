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

package service

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("StopOperation.Execute", func() {
	const (
		name      = "vm"
		namespace = "default"
	)

	var (
		ctx        context.Context
		fakeClient client.WithWatch
		key        types.NamespacedName
	)

	BeforeEach(func() {
		ctx = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient = nil
		key = types.NamespacedName{Name: name, Namespace: namespace}
	})

	stopVMOP := func() *v1alpha2.VirtualMachineOperation {
		return &v1alpha2.VirtualMachineOperation{
			ObjectMeta: metav1.ObjectMeta{Name: "stop", Namespace: namespace},
			Spec: v1alpha2.VirtualMachineOperationSpec{
				Type:           v1alpha2.VMOPTypeStop,
				VirtualMachine: name,
			},
		}
	}

	newKVVM := func(runStrategy virtv1.VirtualMachineRunStrategy) *virtv1.VirtualMachine {
		return &virtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: virtv1.VirtualMachineSpec{
				RunStrategy: ptr.To(runStrategy),
				Template:    &virtv1.VirtualMachineInstanceTemplateSpec{},
			},
		}
	}

	newKVVMI := func(phase virtv1.VirtualMachineInstancePhase) *virtv1.VirtualMachineInstance {
		return &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Status:     virtv1.VirtualMachineInstanceStatus{Phase: phase},
		}
	}

	execute := func(objs ...client.Object) {
		GinkgoHelper()
		var err error
		fakeClient, err = testutil.NewFakeClientWithObjects(objs...)
		Expect(err).NotTo(HaveOccurred())
		Expect(NewStopOperation(fakeClient, stopVMOP()).Execute(ctx)).To(Succeed())
	}

	liveKVVM := func() *virtv1.VirtualMachine {
		GinkgoHelper()
		live := &virtv1.VirtualMachine{}
		Expect(fakeClient.Get(ctx, key, live)).To(Succeed())
		return live
	}

	// A machine that is still starting keeps the create-time RunStrategyAlways, under which
	// KubeVirt recreates a deleted instance. Without settling it first the stop is undone.
	It("settles RunStrategyAlways before deleting a still-Pending instance", func() {
		execute(newKVVM(virtv1.RunStrategyAlways), newKVVMI(virtv1.Pending))

		Expect(liveKVVM().Spec.RunStrategy).To(HaveValue(Equal(virtv1.RunStrategyManual)))
		Expect(fakeClient.Get(ctx, key, &virtv1.VirtualMachineInstance{})).To(MatchError(apierrors.IsNotFound, "not found"))
	})

	It("deletes a running instance", func() {
		execute(newKVVM(virtv1.RunStrategyManual), newKVVMI(virtv1.Running))

		Expect(fakeClient.Get(ctx, key, &virtv1.VirtualMachineInstance{})).To(MatchError(apierrors.IsNotFound, "not found"))
	})

	It("does not touch an internal virtual machine that is already Manual", func() {
		kvvm := newKVVM(virtv1.RunStrategyManual)
		execute(kvvm, newKVVMI(virtv1.Running))

		Expect(liveKVVM().ResourceVersion).To(Equal(kvvm.ResourceVersion))
	})

	It("does nothing when the instance is already gone", func() {
		execute(newKVVM(virtv1.RunStrategyAlways))

		Expect(liveKVVM().Spec.RunStrategy).To(HaveValue(Equal(virtv1.RunStrategyAlways)))
	})
})
