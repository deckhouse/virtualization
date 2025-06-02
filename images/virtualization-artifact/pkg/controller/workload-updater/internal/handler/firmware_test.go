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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

		firmwareImage = "firmware-image:latest"

		virtControllerName      = "virt-controller"
		virtControllerNamespace = "d8-virtualization"
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

	newVirtController := func(ready bool, launcherImage string) *appsv1.Deployment {
		deploy := &appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      virtControllerName,
				Namespace: virtControllerNamespace,
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "virt-controller",
								Args: []string{"--launcher-image", launcherImage},
							},
						},
					},
				},
			},
		}
		if ready {
			deploy.Status.ReadyReplicas = 1
			deploy.Status.Replicas = 1
		}

		return deploy
	}

	DescribeTable("FirmwareHandler should return serviceCompleteErr if migration executed",
		func(vm *v1alpha2.VirtualMachine, deploy *appsv1.Deployment, needMigrate bool) {
			fakeClient, _ = setupEnvironment(vm, deploy)

			mockMigration := &OneShotMigrationMock{
				OnceMigrateFunc: func(ctx context.Context, vm *v1alpha2.VirtualMachine, annotationKey string, annotationExpectedValue string) (bool, error) {
					return true, serviceCompleteErr
				},
			}

			h := NewFirmwareHandler(fakeClient, mockMigration, firmwareImage, virtControllerNamespace, virtControllerName)
			_, err := h.Handle(ctx, vm)

			if needMigrate {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(serviceCompleteErr))
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry("Migration should be executed", newVM(true), newVirtController(true, firmwareImage), true),
		Entry("Migration not should be executed", newVM(false), newVirtController(true, firmwareImage), false),
		Entry("Migration not should be executed because virt-controller not ready", newVM(false), newVirtController(false, firmwareImage), false),
		Entry("Migration not should be executed because virt-controller ready but has wrong image", newVM(false), newVirtController(true, "wrong-image"), false),
	)
})
