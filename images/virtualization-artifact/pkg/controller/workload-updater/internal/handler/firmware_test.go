package handler

import (
	"context"
	"errors"
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("TestFirmwareHandler", func() {
	const (
		name      = "vm-firmware"
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

	newVM := func(needMigrate bool) *v1alpha2.VirtualMachine {
		vm := vmbuilder.NewEmpty(name, namespace)
		status := metav1.ConditionTrue
		if needMigrate {
			status = metav1.ConditionFalse
		}
		vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
			Type:   vmcondition.TypeFirmwareUpToDate.String(),
			Status: status,
		})
		return vm
	}

	DescribeTable("FirmwareHandler should return serviceCompleteErr if migration executed", func(needMigrate bool) {
		vm := newVM(needMigrate)
		fakeClient, _ = setupEnvironment(vm)

		mockMigration := &OneShotMigrationMock{
			OnceMigrateFunc: func(ctx context.Context, vm *v1alpha2.VirtualMachine, annotationKey string, annotationExpectedValue string) (bool, error) {
				return true, serviceCompleteErr
			},
			SetLoggerFunc: func(log *slog.Logger) {},
		}

		h := NewFirmwareHandler(fakeClient, mockMigration, "firmware-image:latest")
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
