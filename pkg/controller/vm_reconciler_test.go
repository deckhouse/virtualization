package controller_test

import (
	"context"
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/controller"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/testutil"
)

var _ = Describe("VM", func() {
	var reconciler *two_phase_reconciler.ReconcilerCore[*controller.VMReconcilerState]
	var reconcileExecutor *testutil.ReconcileExecutor

	AfterEach(func() {
		if reconciler != nil {
			reconciler = nil
		}
	})

	AfterEach(func() {
		if reconciler != nil && reconciler.Recorder != nil {
			close(reconciler.Recorder.(*record.FakeRecorder).Events)
		}
	})

	It("Successfully runs linux vm with vmd", func() {
		ctx := context.Background()

		{
			vm := &virtv2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "test-ns",
					Name:        "test-vm",
					Labels:      nil,
					Annotations: nil,
				},
				Spec: virtv2.VirtualMachineSpec{
					RunPolicy:                virtv2.AlwaysOnPolicy,
					EnableParavirtualization: true,
					OsType:                   virtv2.GenericOs,
					CPU: virtv2.CPUSpec{
						Cores: 2,
					},
					Memory: virtv2.MemorySpec{
						Size: "2Gi",
					},
					BlockDevices: []virtv2.BlockDeviceSpec{
						{
							Type:               virtv2.DiskDevice,
							VirtualMachineDisk: &virtv2.DiskDeviceSpec{Name: "test-vmd"},
						},
					},
				},
			}

			reconciler = controller.NewVMReconciler(vm)
			reconcileExecutor = testutil.NewReconcileExecutor(types.NamespacedName{Name: "test-vm", Namespace: "test-ns"})
		}

		{
			err := reconcileExecutor.Execute(ctx, reconciler)
			Expect(err).NotTo(HaveOccurred())

			vm, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vm", Namespace: "test-ns"}, reconciler.Client, &virtv2.VirtualMachine{})
			Expect(err).NotTo(HaveOccurred())
			Expect(vm).NotTo(BeNil())
			Expect(vm.Status.Phase).To(Equal(virtv2.MachinePending))
			Expect(controllerutil.ContainsFinalizer(vm, virtv2.FinalizerVMCleanup)).To(BeTrue())

			kvvm, err := helper.FetchObject(ctx, types.NamespacedName{Name: vm.Name, Namespace: "test-ns"}, reconciler.Client, &virtv1.VirtualMachine{})
			Expect(err).NotTo(HaveOccurred())
			Expect(kvvm).To(BeNil(), fmt.Sprintf("Unexpected KubeVirt VM %q to be existing when no VMD exists in the system", vm.Name))
		}

		{
			vmd := &virtv2.VirtualMachineDisk{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "test-ns",
					Name:        "test-vmd",
					Labels:      nil,
					Annotations: nil,
				},
				Spec: virtv2.VirtualMachineDiskSpec{
					DataSource: virtv2.DataSource{
						HTTP: &virtv2.DataSourceHTTP{
							URL: "http://mydomain.org/image.img",
						},
					},
					PersistentVolumeClaim: virtv2.VirtualMachinePersistentVolumeClaim{
						Size:             "10Gi",
						StorageClassName: "local-path",
					},
				},
				Status: virtv2.VirtualMachineDiskStatus{
					Phase: virtv2.DiskPending,
					Size:  "10Gi",
				},
			}
			err := reconciler.Client.Create(ctx, vmd)
			Expect(err).NotTo(HaveOccurred())

			err = reconcileExecutor.Execute(ctx, reconciler)
			Expect(err).NotTo(HaveOccurred())

			vm, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vm", Namespace: "test-ns"}, reconciler.Client, &virtv2.VirtualMachine{})
			Expect(err).NotTo(HaveOccurred())
			Expect(vm).NotTo(BeNil())
			Expect(vm.Status.Phase).To(Equal(virtv2.MachinePending))

			kvvm, err := helper.FetchObject(ctx, types.NamespacedName{Name: vm.Name, Namespace: "test-ns"}, reconciler.Client, &virtv1.VirtualMachine{})
			Expect(err).NotTo(HaveOccurred())
			Expect(kvvm).To(BeNil(), fmt.Sprintf("Unexpected KubeVirt VM %q to be existing when VMD not ready yet", vm.Name))
		}

		{
			vmd, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vmd", Namespace: "test-ns"}, reconciler.Client, &virtv2.VirtualMachineDisk{})
			Expect(err).NotTo(HaveOccurred())
			Expect(vmd).NotTo(BeNil())
			vmd.Status.Phase = virtv2.DiskReady
			err = reconciler.Client.Status().Update(ctx, vmd)
			Expect(err).NotTo(HaveOccurred())

			err = reconcileExecutor.Execute(ctx, reconciler)
			Expect(err).NotTo(HaveOccurred())

			vm, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vm", Namespace: "test-ns"}, reconciler.Client, &virtv2.VirtualMachine{})
			Expect(err).NotTo(HaveOccurred())
			Expect(vm).NotTo(BeNil())
			Expect(vm.Status.Phase).To(Equal(virtv2.MachinePending))

			kvvm, err := helper.FetchObject(ctx, types.NamespacedName{Name: vm.Name, Namespace: "test-ns"}, reconciler.Client, &virtv1.VirtualMachine{})
			Expect(err).NotTo(HaveOccurred())
			Expect(kvvm).NotTo(BeNil(), fmt.Sprintf("Expected KubeVirt VM %q to exist", vm.Name))
			Expect(controllerutil.ContainsFinalizer(kvvm, virtv2.FinalizerKVVMProtection)).To(BeTrue())
		}

		{
			kvvm, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vm", Namespace: "test-ns"}, reconciler.Client, &virtv1.VirtualMachine{})
			Expect(err).NotTo(HaveOccurred())
			Expect(kvvm).NotTo(BeNil())

			kvvmi := &virtv1.VirtualMachineInstance{
				ObjectMeta: kvvm.Spec.Template.ObjectMeta,
				Spec:       kvvm.Spec.Template.Spec,
			}
			kvvmi.ObjectMeta.Name = kvvm.Name
			kvvmi.ObjectMeta.Namespace = kvvm.Namespace
			err = reconciler.Client.Create(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			kvvm.Status.Created = true
			err = reconciler.Client.Status().Update(ctx, kvvm)
			Expect(err).NotTo(HaveOccurred())

			err = reconcileExecutor.Execute(ctx, reconciler)
			Expect(err).NotTo(HaveOccurred())

			kvvmi, err = helper.FetchObject(ctx, types.NamespacedName{Name: "test-vm", Namespace: "test-ns"}, reconciler.Client, &virtv1.VirtualMachineInstance{})
			Expect(err).NotTo(HaveOccurred())
			Expect(kvvmi).NotTo(BeNil())
			Expect(controllerutil.ContainsFinalizer(kvvmi, virtv2.FinalizerKVVMIProtection)).To(BeTrue())
		}

		{
			kvvmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vm", Namespace: "test-ns"}, reconciler.Client, &virtv1.VirtualMachineInstance{})
			Expect(err).NotTo(HaveOccurred())
			Expect(kvvmi).NotTo(BeNil())

			kvvmi.Status.Phase = virtv1.Scheduled
			err = reconciler.Client.Status().Update(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			err = reconcileExecutor.Execute(ctx, reconciler)
			Expect(err).NotTo(HaveOccurred())

			vm, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vm", Namespace: "test-ns"}, reconciler.Client, &virtv2.VirtualMachine{})
			Expect(err).NotTo(HaveOccurred())
			Expect(vm).NotTo(BeNil())
			Expect(vm.Status.Phase).To(Equal(virtv2.MachineScheduling))
		}

		{
			kvvmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vm", Namespace: "test-ns"}, reconciler.Client, &virtv1.VirtualMachineInstance{})
			Expect(err).NotTo(HaveOccurred())
			Expect(kvvmi).NotTo(BeNil())

			kvvmi.Status.GuestOSInfo = virtv1.VirtualMachineInstanceGuestOSInfo{
				Name: "linux",
				ID:   "12345",
			}
			kvvmi.Status.Interfaces = append(kvvmi.Status.Interfaces, virtv1.VirtualMachineInstanceNetworkInterface{
				IP:   "1.2.4.8",
				Name: "default",
			})
			kvvmi.Status.NodeName = "k3d-k3s-default-server-0"
			kvvmi.Status.VolumeStatus = append(kvvmi.Status.VolumeStatus, virtv1.VolumeStatus{
				Name:   "test-vmd",
				Target: "vda",
			})
			kvvmi.Status.Phase = virtv1.Scheduling
			err = reconciler.Client.Status().Update(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			err = reconcileExecutor.Execute(ctx, reconciler)
			Expect(err).NotTo(HaveOccurred())

			vm, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vm", Namespace: "test-ns"}, reconciler.Client, &virtv2.VirtualMachine{})
			Expect(err).NotTo(HaveOccurred())
			Expect(vm).NotTo(BeNil())
			Expect(vm.Status.Phase).To(Equal(virtv2.MachineScheduling))
		}

		{
			kvvmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vm", Namespace: "test-ns"}, reconciler.Client, &virtv1.VirtualMachineInstance{})
			Expect(err).NotTo(HaveOccurred())
			Expect(kvvmi).NotTo(BeNil())

			kvvmi.Status.Phase = virtv1.Running
			err = reconciler.Client.Status().Update(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			kvvm, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vm", Namespace: "test-ns"}, reconciler.Client, &virtv1.VirtualMachine{})
			Expect(err).NotTo(HaveOccurred())
			Expect(kvvm).NotTo(BeNil())
			kvvm.Status.Ready = true
			err = reconciler.Client.Status().Update(ctx, kvvm)
			Expect(err).NotTo(HaveOccurred())

			err = reconcileExecutor.Execute(ctx, reconciler)
			Expect(err).NotTo(HaveOccurred())

			vm, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vm", Namespace: "test-ns"}, reconciler.Client, &virtv2.VirtualMachine{})
			Expect(err).NotTo(HaveOccurred())
			Expect(vm).NotTo(BeNil())
			Expect(vm.Status.Phase).To(Equal(virtv2.MachineRunning))
			Expect(reflect.DeepEqual(vm.Status.GuestOSInfo, kvvmi.Status.GuestOSInfo)).To(BeTrue(), fmt.Sprintf("unequal GuestOSInfo %#v != %#v", vm.Status.GuestOSInfo, kvvmi.Status.GuestOSInfo))
			Expect(vm.Status.NodeName).To(Equal(kvvmi.Status.NodeName))
			Expect(vm.Status.IPAddress).To(Equal(kvvmi.Status.Interfaces[0].IP))
			Expect(vm.Status.BlockDevicesAttached[0].Type).To(Equal(virtv2.DiskDevice))
			Expect(vm.Status.BlockDevicesAttached[0].VirtualMachineImage).To(BeNil())
			Expect(reflect.DeepEqual(
				*vm.Status.BlockDevicesAttached[0].VirtualMachineDisk,
				virtv2.DiskDeviceSpec{Name: "test-vmd"},
			)).To(BeTrue())
			Expect(vm.Status.BlockDevicesAttached[0].Target).To(Equal(kvvmi.Status.VolumeStatus[0].Target))
			Expect(vm.Status.BlockDevicesAttached[0].Size).To(Equal("10Gi"))
		}
	})
})
