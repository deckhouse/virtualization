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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/migration/internal/service"
	genericservice "github.com/deckhouse/virtualization-controller/pkg/controller/vmop/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("LifecycleHandler", func() {
	const (
		name      = "test"
		namespace = "default"
	)

	var (
		ctx          context.Context
		fakeClient   client.WithWatch
		srv          *reconciler.Resource[*virtv2.VirtualMachineOperation, virtv2.VirtualMachineOperationStatus]
		recorderMock *eventrecord.EventRecorderLoggerMock
	)

	BeforeEach(func() {
		ctx = testutil.ContextBackgroundWithNoOpLogger()
		recorderMock = &eventrecord.EventRecorderLoggerMock{
			EventFunc:  func(_ client.Object, _, _, _ string) {},
			EventfFunc: func(_ client.Object, _, _, _ string, _ ...interface{}) {},
		}
		recorderMock.WithLoggingFunc = func(logger eventrecord.InfoLogger) eventrecord.EventRecorderLogger {
			return recorderMock
		}
	})

	newVMOPEvictPending := func(opts ...vmopbuilder.Option) *virtv2.VirtualMachineOperation {
		options := []vmopbuilder.Option{
			vmopbuilder.WithName(name),
			vmopbuilder.WithNamespace(namespace),
			vmopbuilder.WithType(virtv2.VMOPTypeEvict),
			vmopbuilder.WithVirtualMachine(name),
		}
		options = append(options, opts...)
		vmop := vmopbuilder.New(options...)
		vmop.Status.Phase = virtv2.VMOPPhasePending
		return vmop
	}

	newVM := func(vmPolicy virtv2.LiveMigrationPolicy) *virtv2.VirtualMachine {
		vm := vmbuilder.NewEmpty(name, namespace)
		vm.Spec.LiveMigrationPolicy = vmPolicy
		vm.Spec.RunPolicy = virtv2.AlwaysOnPolicy
		vm.Status.Phase = virtv2.MachineRunning

		return vm
	}

	DescribeTable("Evict operation for migration policy", func(vmop *virtv2.VirtualMachineOperation, vmPolicy virtv2.LiveMigrationPolicy, expectedPhase virtv2.VMOPPhase) {
		vm := newVM(vmPolicy)

		fakeClient, srv = setupEnvironment(vmop, vm)
		migrationService := service.NewMigrationService(fakeClient)
		base := genericservice.NewBaseVMOPService(fakeClient, recorderMock)

		h := NewLifecycleHandler(fakeClient, migrationService, base, recorderMock)
		_, err := h.Handle(ctx, srv.Changed())
		Expect(err).NotTo(HaveOccurred())

		Expect(srv.Changed().Status.Phase).To(Equal(expectedPhase), "should updated status phase, conditions: %+v", srv.Changed().Status.Conditions)
	},
		// AlwaysSafe cases.
		Entry("is ok for AlwaysSafe and force=nil",
			newVMOPEvictPending(),
			virtv2.AlwaysSafeMigrationPolicy,
			virtv2.VMOPPhasePending,
		),
		Entry("is ok for AlwaysSafe and force=false",
			newVMOPEvictPending(vmopbuilder.WithForce(ptr.To(false))),
			virtv2.AlwaysSafeMigrationPolicy,
			virtv2.VMOPPhasePending,
		),
		Entry("should become Failed for AlwaysSafe and force=true",
			newVMOPEvictPending(vmopbuilder.WithForce(ptr.To(true))),
			virtv2.AlwaysSafeMigrationPolicy,
			virtv2.VMOPPhaseFailed,
		),

		// PreferSafe cases.
		Entry("is ok for PreferSafe and force=nil",
			newVMOPEvictPending(),
			virtv2.PreferSafeMigrationPolicy,
			virtv2.VMOPPhasePending,
		),
		Entry("is ok for PreferSafe and force=false",
			newVMOPEvictPending(vmopbuilder.WithForce(ptr.To(false))),
			virtv2.PreferSafeMigrationPolicy,
			virtv2.VMOPPhasePending,
		),
		Entry("is ok for PreferSafe and force=true",
			newVMOPEvictPending(vmopbuilder.WithForce(ptr.To(true))),
			virtv2.PreferSafeMigrationPolicy,
			virtv2.VMOPPhasePending,
		),

		// AlwaysForced cases.
		Entry("is ok for AlwaysForced and force=nil",
			newVMOPEvictPending(),
			virtv2.AlwaysForcedMigrationPolicy,
			virtv2.VMOPPhasePending,
		),
		Entry("should become Failed for AlwaysForced and force=false",
			newVMOPEvictPending(vmopbuilder.WithForce(ptr.To(false))),
			virtv2.AlwaysForcedMigrationPolicy,
			virtv2.VMOPPhaseFailed,
		),
		Entry("is ok for AlwaysForced and force=true",
			newVMOPEvictPending(vmopbuilder.WithForce(ptr.To(true))),
			virtv2.AlwaysForcedMigrationPolicy,
			virtv2.VMOPPhasePending,
		),

		// PreferForced cases.
		Entry("is ok for PreferForced and force=nil",
			newVMOPEvictPending(),
			virtv2.PreferForcedMigrationPolicy,
			virtv2.VMOPPhasePending,
		),
		Entry("is ok for PreferForced and force=false",
			newVMOPEvictPending(vmopbuilder.WithForce(ptr.To(false))),
			virtv2.PreferForcedMigrationPolicy,
			virtv2.VMOPPhasePending,
		),
		Entry("is ok for PreferForced and force=true",
			newVMOPEvictPending(vmopbuilder.WithForce(ptr.To(true))),
			virtv2.PreferForcedMigrationPolicy,
			virtv2.VMOPPhasePending,
		),
	)
})
