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

package handler

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = FDescribe("TestHotplugResourcesHandler", func() {
	const (
		name      = "vm-hotplug-resources"
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

	newVMAndKVVMI := func(hasHotMemoryChange bool) (*v1alpha2.VirtualMachine, *virtv1.VirtualMachineInstance) {
		vm := vmbuilder.NewEmpty(name, namespace)
		kvvmi := newEmptyKVVMI(name, namespace)

		if hasHotMemoryChange {
			kvvmi.Status.Conditions = append(kvvmi.Status.Conditions, virtv1.VirtualMachineInstanceCondition{
				Type:   virtv1.VirtualMachineInstanceMemoryChange,
				Status: corev1.ConditionTrue,
			})
		}
		return vm, kvvmi
	}

	newOnceMigrationMock := func(shouldMigrate bool) *OneShotMigrationMock {
		return &OneShotMigrationMock{
			OnceMigrateFunc: func(ctx context.Context, vm *v1alpha2.VirtualMachine, annotationKey, annotationExpectedValue string) (bool, error) {
				if shouldMigrate {
					return true, serviceCompleteErr
				}
				return false, nil
			},
		}
	}

	type testResourcesSettings struct {
		hasHotMemoryChangeCondition bool
		shouldMigrate               bool
	}

	DescribeTable("HotplugResourcesHandler should return serviceCompleteErr if migration executed",
		func(settings testResourcesSettings) {
			vm, kvvmi := newVMAndKVVMI(settings.hasHotMemoryChangeCondition)
			fakeClient = setupEnvironment(vm, kvvmi)

			mockMigration := newOnceMigrationMock(settings.shouldMigrate)

			h := NewHotplugHandler(fakeClient, mockMigration)
			_, err := h.Handle(ctx, vm)

			if settings.hasHotMemoryChangeCondition && !settings.shouldMigrate {
				Expect(err).ToNot(HaveOccurred())
			} else if settings.hasHotMemoryChangeCondition {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(serviceCompleteErr))
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry(
			"Migration should be executed on hotMemoryChange condition",
			testResourcesSettings{
				hasHotMemoryChangeCondition: true,
				shouldMigrate:               true,
			},
		),
		Entry(
			"Migration should not be executed the second time",
			testResourcesSettings{
				hasHotMemoryChangeCondition: true,
				shouldMigrate:               false,
			},
		),
		Entry(
			"Migration should not be executed without hotMemoryChange condition",
			testResourcesSettings{
				hasHotMemoryChangeCondition: false,
			},
		),
	)

})
