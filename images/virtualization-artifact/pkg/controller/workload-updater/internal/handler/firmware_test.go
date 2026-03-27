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
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

type firmwareMigrationStub struct {
	onceMigrate func(ctx context.Context, vm *v1alpha2.VirtualMachine, annotationKey, annotationExpectedValue string) (bool, error)
}

func (s *firmwareMigrationStub) OnceMigrate(ctx context.Context, vm *v1alpha2.VirtualMachine, annotationKey, annotationExpectedValue string) (bool, error) {
	return s.onceMigrate(ctx, vm, annotationKey, annotationExpectedValue)
}

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
		fakeClient         client.Client
	)

	AfterEach(func() {
		fakeClient = nil
	})

	newVM := func(firmwareUpToDateStatus metav1.ConditionStatus) *v1alpha2.VirtualMachine {
		vm := vmbuilder.NewEmpty(name, namespace)
		vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
			Type:   vmcondition.TypeFirmwareUpToDate.String(),
			Status: firmwareUpToDateStatus,
		})
		return vm
	}

	newVMNeedMigrate := func() *v1alpha2.VirtualMachine {
		return newVM(metav1.ConditionFalse)
	}

	newVMUpToDate := func() *v1alpha2.VirtualMachine {
		return newVM(metav1.ConditionTrue)
	}

	newKVVMI := func(phase virtv1.VirtualMachineInstancePhase) *virtv1.VirtualMachineInstance {
		return &virtv1.VirtualMachineInstance{
			TypeMeta: metav1.TypeMeta{
				APIVersion: virtv1.GroupVersion.String(),
				Kind:       "VirtualMachineInstance",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Status: virtv1.VirtualMachineInstanceStatus{
				Phase: phase,
			},
		}
	}

	setupFirmwareEnvironment := func(vm *v1alpha2.VirtualMachine, objs ...client.Object) client.Client {
		allObjects := []client.Object{vm}
		allObjects = append(allObjects, objs...)
		fakeClient, err := testutil.NewFakeClientWithObjects(allObjects...)
		Expect(err).NotTo(HaveOccurred())
		return fakeClient
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
		deploy.Status.Replicas = 1
		if ready {
			deploy.Status.ReadyReplicas = 1
		}

		return deploy
	}

	DescribeTable("FirmwareHandler should return serviceCompleteErr if migration executed",
		func(vm *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, deploy *appsv1.Deployment, needMigrate bool) {
			objs := []client.Object{deploy}
			if kvvmi != nil {
				objs = append(objs, kvvmi)
			}
			fakeClient = setupFirmwareEnvironment(vm, objs...)

			mockMigration := &firmwareMigrationStub{
				onceMigrate: func(ctx context.Context, vm *v1alpha2.VirtualMachine, annotationKey, annotationExpectedValue string) (bool, error) {
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
		Entry("Migration should be executed", newVMNeedMigrate(), newKVVMI(virtv1.Running), newVirtController(true, firmwareImage), true),
		Entry("Migration should be executed when kvvmi is not found", newVMNeedMigrate(), nil, newVirtController(true, firmwareImage), true),
		Entry("Migration not should be executed", newVMUpToDate(), newKVVMI(virtv1.Running), newVirtController(true, firmwareImage), false),
		Entry("Migration not should be executed because kvvmi is stopped", newVMNeedMigrate(), newKVVMI(virtv1.Succeeded), newVirtController(true, firmwareImage), false),
		Entry("Migration not should be executed because kvvmi is pending", newVMNeedMigrate(), newKVVMI(virtv1.Pending), newVirtController(true, firmwareImage), false),
		Entry("Migration not should be executed because virt-controller not ready", newVMNeedMigrate(), newKVVMI(virtv1.Running), newVirtController(false, firmwareImage), false),
		Entry("Migration not should be executed because virt-controller ready but has wrong image", newVMNeedMigrate(), newKVVMI(virtv1.Running), newVirtController(true, "wrong-image"), false),
	)

	It("should return error when kvvmi get returns non not-found error", func() {
		vm := newVMNeedMigrate()
		deploy := newVirtController(true, firmwareImage)
		kvvmiGetErr := errors.New("get kvvmi failed")

		interceptClient, err := testutil.NewFakeClientWithInterceptorWithObjects(interceptor.Funcs{
			Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*virtv1.VirtualMachineInstance); ok {
					return kvvmiGetErr
				}
				return client.Get(ctx, key, obj, opts...)
			},
		}, vm, deploy)
		Expect(err).NotTo(HaveOccurred())

		migrationCalled := false
		h := NewFirmwareHandler(interceptClient, &firmwareMigrationStub{
			onceMigrate: func(ctx context.Context, vm *v1alpha2.VirtualMachine, annotationKey, annotationExpectedValue string) (bool, error) {
				migrationCalled = true
				return true, nil
			},
		}, firmwareImage, virtControllerNamespace, virtControllerName)

		_, err = h.Handle(ctx, vm)
		Expect(err).To(MatchError(kvvmiGetErr))
		Expect(migrationCalled).To(BeFalse())
	})

	It("should not call migration when kvvmi is not running", func() {
		vm := newVMNeedMigrate()
		kvvmi := newKVVMI(virtv1.Failed)
		deploy := newVirtController(true, firmwareImage)
		fakeClient = setupFirmwareEnvironment(vm, kvvmi, deploy)

		migrationCalled := false
		h := NewFirmwareHandler(fakeClient, &firmwareMigrationStub{
			onceMigrate: func(ctx context.Context, vm *v1alpha2.VirtualMachine, annotationKey, annotationExpectedValue string) (bool, error) {
				migrationCalled = true
				return true, nil
			},
		}, firmwareImage, virtControllerNamespace, virtControllerName)

		_, err := h.Handle(ctx, vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(migrationCalled).To(BeFalse())
	})

	It("should continue processing when kvvmi is not found", func() {
		vm := newVMNeedMigrate()
		deploy := newVirtController(true, firmwareImage)
		fakeClient = setupFirmwareEnvironment(vm, deploy)

		migrationCalled := false
		h := NewFirmwareHandler(fakeClient, &firmwareMigrationStub{
			onceMigrate: func(ctx context.Context, vm *v1alpha2.VirtualMachine, annotationKey, annotationExpectedValue string) (bool, error) {
				migrationCalled = true
				return false, nil
			},
		}, firmwareImage, virtControllerNamespace, virtControllerName)

		_, err := h.Handle(ctx, vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(migrationCalled).To(BeTrue())
	})

	It("should return nil when vm is nil", func() {
		h := NewFirmwareHandler(nil, nil, firmwareImage, virtControllerNamespace, virtControllerName)

		_, err := h.Handle(ctx, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should return nil when vm has deletion timestamp", func() {
		vm := newVMNeedMigrate()
		now := metav1.Now()
		vm.DeletionTimestamp = &now

		h := NewFirmwareHandler(nil, nil, firmwareImage, virtControllerNamespace, virtControllerName)
		_, err := h.Handle(ctx, vm)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should return error when virt-controller deployment get fails", func() {
		vm := newVMNeedMigrate()
		kvvmi := newKVVMI(virtv1.Running)
		fakeClient = setupFirmwareEnvironment(vm, kvvmi)

		h := NewFirmwareHandler(fakeClient, &firmwareMigrationStub{
			onceMigrate: func(ctx context.Context, vm *v1alpha2.VirtualMachine, annotationKey, annotationExpectedValue string) (bool, error) {
				return false, nil
			},
		}, firmwareImage, virtControllerNamespace, virtControllerName)

		_, err := h.Handle(ctx, vm)
		Expect(err).To(HaveOccurred())
	})

	Describe("needUpdate", func() {
		buildVMWithConditions := func(conditions []metav1.Condition) *v1alpha2.VirtualMachine {
			vm := vmbuilder.NewEmpty(name, namespace)
			vm.Status.Conditions = conditions
			return vm
		}

		DescribeTable("should evaluate FirmwareUpToDate condition",
			func(vm *v1alpha2.VirtualMachine, expected bool) {
				h := NewFirmwareHandler(nil, nil, firmwareImage, virtControllerNamespace, virtControllerName)
				Expect(h.needUpdate(vm)).To(Equal(expected))
			},
			Entry("FirmwareUpToDate is False", buildVMWithConditions([]metav1.Condition{{
				Type:   vmcondition.TypeFirmwareUpToDate.String(),
				Status: metav1.ConditionFalse,
			}}), true),
			Entry("FirmwareUpToDate is True", buildVMWithConditions([]metav1.Condition{{
				Type:   vmcondition.TypeFirmwareUpToDate.String(),
				Status: metav1.ConditionTrue,
			}}), false),
			Entry("FirmwareUpToDate is absent", buildVMWithConditions([]metav1.Condition{{
				Type:   "SomeOtherCondition",
				Status: metav1.ConditionTrue,
			}}), false),
		)
	})

	Describe("isVirtControllerUpToDate", func() {
		It("should return error when deployment is not found", func() {
			h := NewFirmwareHandler(setupFirmwareEnvironment(newVMNeedMigrate()), nil, firmwareImage, virtControllerNamespace, virtControllerName)

			ready, err := h.isVirtControllerUpToDate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(ready).To(BeFalse())
		})

		DescribeTable("should evaluate deployment state and launcher image",
			func(deploy *appsv1.Deployment, expectedReady bool) {
				h := NewFirmwareHandler(setupFirmwareEnvironment(newVMNeedMigrate(), deploy), nil, firmwareImage, virtControllerNamespace, virtControllerName)

				ready, err := h.isVirtControllerUpToDate(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(ready).To(Equal(expectedReady))
			},
			Entry("deployment is not ready", newVirtController(false, firmwareImage), false),
			Entry("deployment is ready but image differs", newVirtController(true, "other-image"), false),
			Entry("deployment is ready and image matches", newVirtController(true, firmwareImage), true),
		)
	})

	Describe("getVirtLauncherImage", func() {
		It("should return empty when virt-controller container is absent", func() {
			deploy := &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name: "sidecar",
								Args: []string{"--launcher-image", firmwareImage},
							}},
						},
					},
				},
			}
			Expect(getVirtLauncherImage(deploy)).To(BeEmpty())
		})

		It("should return empty when launcher image argument is absent", func() {
			deploy := &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name: "virt-controller",
								Args: []string{"--some-flag", "value"},
							}},
						},
					},
				},
			}
			Expect(getVirtLauncherImage(deploy)).To(BeEmpty())
		})

		It("should return launcher image from --launcher-image=<value>", func() {
			deploy := &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name: "virt-controller",
								Args: []string{"--launcher-image=" + firmwareImage},
							}},
						},
					},
				},
			}
			Expect(getVirtLauncherImage(deploy)).To(Equal(firmwareImage))
		})
	})

	It("should return firmware handler name", func() {
		h := NewFirmwareHandler(nil, nil, firmwareImage, virtControllerNamespace, virtControllerName)
		Expect(h.Name()).To(Equal(firmwareHandler))
	})
})
