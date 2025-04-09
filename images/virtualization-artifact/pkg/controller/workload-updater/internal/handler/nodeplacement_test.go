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
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
		fakeClient         client.WithWatch
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
			fakeClient, _ = setupEnvironment(vm, kvvmi)

			mockMigration := &OneShotMigrationMock{
				OnceMigrateFunc: func(ctx context.Context, vm *v1alpha2.VirtualMachine, annotationKey string, annotationExpectedValue string) (bool, error) {
					return true, serviceCompleteErr
				},
				SetLoggerFunc: func(log *slog.Logger) {},
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
})
