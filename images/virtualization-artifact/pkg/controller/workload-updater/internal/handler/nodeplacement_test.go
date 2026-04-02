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
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("TestNodePlacementHandler", func() {
	const (
		name      = "vm-nodeplacement"
		namespace = "default"
	)

	var (
		serviceCompleteErr = errors.New("service is complete")
		ctx                = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient         client.Client
	)

	AfterEach(func() {
		fakeClient = nil
	})

	newVMAndKVVMI := func(needMigrate bool) (*v1alpha2.VirtualMachine, *virtv1.VirtualMachineInstance) {
		vm := vmbuilder.NewEmpty(name, namespace)
		kvvmi := newEmptyKVVMI(name, namespace)
		status := corev1.ConditionFalse
		if needMigrate {
			status = corev1.ConditionTrue
		}
		if needMigrate {
			kvvmi.Status.Conditions = append(kvvmi.Status.Conditions, virtv1.VirtualMachineInstanceCondition{
				Type:   conditions.VirtualMachineInstanceNodePlacementNotMatched,
				Status: status,
			})
		}
		return vm, kvvmi
	}

	DescribeTable("NodePlacementHandler should return serviceCompleteErr if migration executed",
		func(needMigrate bool) {
			vm, kvvmi := newVMAndKVVMI(needMigrate)
			fakeClient = setupEnvironment(vm, kvvmi)

			mockMigration := &OneShotMigrationMock{
				OnceMigrateFunc: func(ctx context.Context, vm *v1alpha2.VirtualMachine, annotationKey, annotationExpectedValue string) (bool, error) {
					return true, serviceCompleteErr
				},
			}

			h := NewNodePlacementHandler(fakeClient, mockMigration)
			_, err := h.Handle(ctx, vm)

			if needMigrate {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(serviceCompleteErr))
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry("Migration should be executed", true),
		Entry("Migration not should be executed", false),
	)

	It("should return nil when vm is nil", func() {
		h := NewNodePlacementHandler(nil, &OneShotMigrationMock{
			OnceMigrateFunc: func(ctx context.Context, vm *v1alpha2.VirtualMachine, annotationKey, annotationExpectedValue string) (bool, error) {
				return false, nil
			},
		})

		_, err := h.Handle(ctx, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should return nil when vm has deletion timestamp", func() {
		vm := vmbuilder.NewEmpty(name, namespace)
		now := metav1.Now()
		vm.DeletionTimestamp = &now

		h := NewNodePlacementHandler(nil, &OneShotMigrationMock{
			OnceMigrateFunc: func(ctx context.Context, vm *v1alpha2.VirtualMachine, annotationKey, annotationExpectedValue string) (bool, error) {
				return false, nil
			},
		})

		_, err := h.Handle(ctx, vm)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should return error when kvvmi get fails", func() {
		vm := vmbuilder.NewEmpty(name, namespace)
		getErr := errors.New("get kvvmi failed")
		interceptClient, err := testutil.NewFakeClientWithInterceptorWithObjects(interceptor.Funcs{
			Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*virtv1.VirtualMachineInstance); ok {
					return getErr
				}
				return client.Get(ctx, key, obj, opts...)
			},
		}, vm)
		Expect(err).NotTo(HaveOccurred())

		h := NewNodePlacementHandler(interceptClient, &OneShotMigrationMock{
			OnceMigrateFunc: func(ctx context.Context, vm *v1alpha2.VirtualMachine, annotationKey, annotationExpectedValue string) (bool, error) {
				return false, nil
			},
		})

		_, err = h.Handle(ctx, vm)
		Expect(err).To(MatchError(getErr))
	})

	It("should ignore not found error when kvvmi get returns not found", func() {
		vm := vmbuilder.NewEmpty(name, namespace)
		notFoundErr := k8serrors.NewNotFound(schema.GroupResource{Group: virtv1.GroupVersion.Group, Resource: "virtualmachineinstances"}, name)
		interceptClient, err := testutil.NewFakeClientWithInterceptorWithObjects(interceptor.Funcs{
			Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*virtv1.VirtualMachineInstance); ok {
					return notFoundErr
				}
				return client.Get(ctx, key, obj, opts...)
			},
		}, vm)
		Expect(err).NotTo(HaveOccurred())

		h := NewNodePlacementHandler(interceptClient, &OneShotMigrationMock{
			OnceMigrateFunc: func(ctx context.Context, vm *v1alpha2.VirtualMachine, annotationKey, annotationExpectedValue string) (bool, error) {
				return false, nil
			},
		})

		_, err = h.Handle(ctx, vm)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should return node placement handler name", func() {
		h := NewNodePlacementHandler(nil, nil)
		Expect(h.Name()).To(Equal(nodePlacementHandler))
	})

	It("should return error for nil kvvmi in genNodePlacementSum", func() {
		sum, err := genNodePlacementSum(nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("kvvmi is nil"))
		Expect(sum).To(BeEmpty())
	})
})
