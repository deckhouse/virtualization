package controller_test

import (
	"context"
	"encoding/json"
	"fmt"
	"k8s.io/client-go/tools/record"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v1alpha2"
	"github.com/deckhouse/virtualization-controller/pkg/controller"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmchange"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/testutil"
)

var testVMLabels = map[string]string{
	"test-label-1": "test-value-1",
}

var testVMAnno = map[string]string{
	"test-anno-1": "test-value-1",
}

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
					Labels:      testVMLabels,
					Annotations: testVMAnno,
				},
				Spec: virtv2.VirtualMachineSpec{
					VirtualMachineIPAddressClaimName: "test-vmip",
					RunPolicy:                        virtv2.AlwaysOnPolicy,
					EnableParavirtualization:         true,
					OsType:                           virtv2.GenericOs,
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
					Disruptions: &virtv2.Disruptions{RestartApprovalMode: virtv2.Automatic},
				},
				Status: virtv2.VirtualMachineStatus{},
			}

			reconciler = controller.NewTestVMReconciler(controller.TestReconcilerOptions{
				KnownObjects: []client.Object{
					&virtv2.VirtualMachine{},
					&virtv2.VirtualMachineDisk{},
					&virtv2.ClusterVirtualMachineImage{},
					&virtv2.VirtualMachineIPAddressClaim{},
					&virtv1.VirtualMachine{},
					&virtv1.VirtualMachineInstance{},
				},
				RuntimeObjects: []runtime.Object{vm},
			})
			reconcileExecutor = testutil.NewReconcileExecutor(types.NamespacedName{Name: "test-vm", Namespace: "test-ns"})
		}

		{
			err := reconciler.Client.Create(ctx, &virtv2.VirtualMachineIPAddressClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vmip",
					Namespace: "test-ns",
				},
				Spec: virtv2.VirtualMachineIPAddressClaimSpec{
					Address: "10.0.0.1",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			vmip, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vmip", Namespace: "test-ns"}, reconciler.Client, &virtv2.VirtualMachineIPAddressClaim{})
			Expect(err).NotTo(HaveOccurred())
			Expect(vmip).NotTo(BeNil())
			Expect(vmip.Spec.Address).To(Equal("10.0.0.1"))
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
			storageClassName := "local-path"
			vmd := &virtv2.VirtualMachineDisk{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "test-ns",
					Name:        "test-vmd",
					Labels:      nil,
					Annotations: nil,
				},
				Spec: virtv2.VirtualMachineDiskSpec{
					DataSource: &virtv2.VMDDataSource{
						HTTP: &virtv2.DataSourceHTTP{
							URL: "http://mydomain.org/image.img",
						},
					},
					PersistentVolumeClaim: virtv2.VMDPersistentVolumeClaim{
						Size:             resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
						StorageClassName: &storageClassName,
					},
				},
				Status: virtv2.VirtualMachineDiskStatus{
					Phase:    virtv2.DiskPending,
					Capacity: "10Gi",
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
			// Check custom labels and annotations
			for k, v := range testVMLabels {
				Expect(kvvm.Labels).To(HaveKey(k), "kvvm should have label %s from vm", k)
				Expect(kvvm.Labels[k]).To(Equal(v), "kvvm should have label %s=%s", k, v)
			}
			for k, v := range testVMAnno {
				Expect(kvvm.Annotations).To(HaveKey(k), "kvvm should have annotation %s from vm", k)
				Expect(kvvm.Annotations[k]).To(Equal(v), "kvvm should have annotation %s=%s", k, v)
			}
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
		}

		{
			kvvm, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vm", Namespace: "test-ns"}, reconciler.Client, &virtv1.VirtualMachine{})
			Expect(err).NotTo(HaveOccurred())
			Expect(kvvm).NotTo(BeNil())

			kvvm.Status.PrintableStatus = virtv1.VirtualMachineStatusProvisioning
			err = reconciler.Client.Status().Update(ctx, kvvm)
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

			kvvm, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vm", Namespace: "test-ns"}, reconciler.Client, &virtv1.VirtualMachine{})
			Expect(err).NotTo(HaveOccurred())
			Expect(kvvm).NotTo(BeNil())

			kvvmi.Status.GuestOSInfo = virtv1.VirtualMachineInstanceGuestOSInfo{
				Name: "linux",
				ID:   "12345",
			}
			kvvmi.Status.Interfaces = append(kvvmi.Status.Interfaces, virtv1.VirtualMachineInstanceNetworkInterface{
				IP:   "10.0.0.1",
				Name: "default",
			})
			kvvmi.Status.NodeName = "k3d-k3s-default-server-0"
			kvvmi.Status.VolumeStatus = append(kvvmi.Status.VolumeStatus, virtv1.VolumeStatus{
				Name:   kvbuilder.GenerateVMDDiskName("test-vmd"),
				Target: "vda",
			})
			kvvmi.Status.Phase = virtv1.Scheduling
			err = reconciler.Client.Status().Update(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			kvvm.Status.PrintableStatus = virtv1.VirtualMachineStatusProvisioning
			err = reconciler.Client.Status().Update(ctx, kvvm)
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
			kvvm.Status.PrintableStatus = virtv1.VirtualMachineStatusRunning
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

		{
			// Test propagating labels.
			vm, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vm", Namespace: "test-ns"}, reconciler.Client, &virtv2.VirtualMachine{})
			Expect(err).NotTo(HaveOccurred())
			Expect(vm).NotTo(BeNil())

			// Set new label.
			vm.Labels["new-label"] = "new-value"

			err = reconciler.Client.Update(ctx, vm)
			Expect(err).NotTo(HaveOccurred())

			err = reconcileExecutor.Execute(ctx, reconciler)
			Expect(err).NotTo(HaveOccurred())

			// Check labels in underlying resources.
			kvvm, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vm", Namespace: "test-ns"}, reconciler.Client, &virtv1.VirtualMachine{})
			Expect(err).NotTo(HaveOccurred())
			Expect(kvvm).NotTo(BeNil())
			Expect(kvvm.Labels).To(HaveKey("new-label"))
			Expect(kvvm.Labels["new-label"]).To(Equal("new-value"))

			kvvmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vm", Namespace: "test-ns"}, reconciler.Client, &virtv1.VirtualMachineInstance{})
			Expect(err).NotTo(HaveOccurred())
			Expect(kvvmi).NotTo(BeNil())
			Expect(kvvmi.Labels).To(HaveKey("new-label"))
			Expect(kvvmi.Labels["new-label"]).To(Equal("new-value"))
		}
	})
})

var _ = Describe("Apply VM changes", func() {
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

	It("Restart VM on memory change", func(ctx SpecContext) {
		nsName := "test-ns-2"
		vmName := "test-vm-2"
		vmipName := "test-vmip"
		vmdName := "test-vmd"
		storageClassName := "local-path"

		{
			vm := &virtv2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: nsName,
					Name:      vmName,
				},
				Spec: virtv2.VirtualMachineSpec{
					VirtualMachineIPAddressClaimName: vmipName,
					RunPolicy:                        virtv2.AlwaysOnPolicy,
					EnableParavirtualization:         true,
					OsType:                           virtv2.GenericOs,
					CPU: virtv2.CPUSpec{
						Cores: 2,
					},
					Memory: virtv2.MemorySpec{
						Size: "2Gi",
					},
					BlockDevices: []virtv2.BlockDeviceSpec{
						{
							Type:               virtv2.DiskDevice,
							VirtualMachineDisk: &virtv2.DiskDeviceSpec{Name: vmdName},
						},
					},
					Disruptions: &virtv2.Disruptions{RestartApprovalMode: virtv2.Automatic},
				},
				Status: virtv2.VirtualMachineStatus{},
			}

			vmd := &virtv2.VirtualMachineDisk{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: nsName,
					Name:      vmdName,
				},
				Spec: virtv2.VirtualMachineDiskSpec{
					DataSource: &virtv2.VMDDataSource{
						HTTP: &virtv2.DataSourceHTTP{
							URL: "http://mydomain.org/image.img",
						},
					},
					PersistentVolumeClaim: virtv2.VMDPersistentVolumeClaim{
						Size:             resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
						StorageClassName: &storageClassName,
					},
				},
				Status: virtv2.VirtualMachineDiskStatus{
					Phase:    virtv2.DiskReady,
					Capacity: "10Gi",
				},
			}

			reconciler = controller.NewTestVMReconciler(controller.TestReconcilerOptions{
				KnownObjects: []client.Object{
					&virtv2.VirtualMachine{},
					&virtv2.VirtualMachineDisk{},
					&virtv2.ClusterVirtualMachineImage{},
					&virtv1.VirtualMachine{},
					&virtv1.VirtualMachineInstance{},
				},
				RuntimeObjects: []runtime.Object{vm},
			})
			reconcileExecutor = testutil.NewReconcileExecutor(types.NamespacedName{Name: vmName, Namespace: nsName})

			err := reconciler.Client.Create(ctx, &virtv2.VirtualMachineIPAddressClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vmipName,
					Namespace: nsName,
				},
				Spec: virtv2.VirtualMachineIPAddressClaimSpec{
					Address: "10.0.0.1",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			vmip, err := helper.FetchObject(ctx, types.NamespacedName{Name: vmipName, Namespace: nsName}, reconciler.Client, &virtv2.VirtualMachineIPAddressClaim{})
			Expect(err).NotTo(HaveOccurred())
			Expect(vmip).NotTo(BeNil())

			CreateReadyVM(ctx, reconciler, reconcileExecutor, vm, vmd)
		}

		{
			// Ensure kubevirt VMI is present.
			kvvmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: vmName, Namespace: nsName}, reconciler.Client, &virtv1.VirtualMachineInstance{})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(kvvmi).ShouldNot(BeNil(), "kubevirt VirtualMachineInstance should present")

			// Change memory settings.
			vm, err := helper.FetchObject(ctx, types.NamespacedName{Name: vmName, Namespace: nsName}, reconciler.Client, &virtv2.VirtualMachine{})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(vm).ShouldNot(BeNil())
			vm.Spec.Memory.Size = "1" + vm.Spec.Memory.Size
			err = reconciler.Client.Update(ctx, vm)
			Expect(err).ShouldNot(HaveOccurred())

			// Reconcile new memory settings.
			err = reconcileExecutor.Execute(ctx, reconciler)
			Expect(err).ShouldNot(HaveOccurred())

			// Check that kubevirt VMI was deleted because memory changes require restart.
			kvvmi, err = helper.FetchObject(ctx, types.NamespacedName{Name: vmName, Namespace: nsName}, reconciler.Client, &virtv1.VirtualMachineInstance{})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(kvvmi).To(BeNil(), "kubevirt VirtualMachineInstance should be deleted")
		}
	})
})

var _ = Describe("Apply VM changes with manual approval", func() {
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

	It("Restart VM on memory change after approval", func(ctx SpecContext) {
		nsName := "test-ns-3"
		vmName := "test-vm"
		vmipName := "test-vmip"
		vmdName := "test-vmd"
		storageClassName := "local-path"
		memoryStartingSize := "2Gi"
		memoryNewSize := "3Gi"
		cpuStartingCores := 2
		cpuNewCores := 4
		cpuStartingCoreFraction := "80%"
		cpuNewCoreFraction := "50%"

		{
			By("Creating VM in Ready status")
			vm := &virtv2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: nsName,
					Name:      vmName,
				},
				Spec: virtv2.VirtualMachineSpec{
					VirtualMachineIPAddressClaimName: vmipName,
					RunPolicy:                        virtv2.AlwaysOnPolicy,
					EnableParavirtualization:         true,
					OsType:                           virtv2.GenericOs,
					CPU: virtv2.CPUSpec{
						Cores:        cpuStartingCores,
						CoreFraction: cpuStartingCoreFraction,
					},
					Memory: virtv2.MemorySpec{
						Size: memoryStartingSize,
					},
					BlockDevices: []virtv2.BlockDeviceSpec{
						{
							Type:               virtv2.DiskDevice,
							VirtualMachineDisk: &virtv2.DiskDeviceSpec{Name: vmdName},
						},
					},
					Disruptions: &virtv2.Disruptions{RestartApprovalMode: virtv2.Manual},
				},
				Status: virtv2.VirtualMachineStatus{},
			}

			vmd := &virtv2.VirtualMachineDisk{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: nsName,
					Name:      vmdName,
				},
				Spec: virtv2.VirtualMachineDiskSpec{
					DataSource: &virtv2.VMDDataSource{
						HTTP: &virtv2.DataSourceHTTP{
							URL: "http://mydomain.org/image.img",
						},
					},
					PersistentVolumeClaim: virtv2.VMDPersistentVolumeClaim{
						Size:             resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
						StorageClassName: &storageClassName,
					},
				},
				Status: virtv2.VirtualMachineDiskStatus{
					Phase:    virtv2.DiskReady,
					Capacity: "10Gi",
				},
			}

			reconciler = controller.NewTestVMReconciler(controller.TestReconcilerOptions{
				KnownObjects: []client.Object{
					&virtv2.VirtualMachine{},
					&virtv2.VirtualMachineDisk{},
					&virtv2.ClusterVirtualMachineImage{},
					&virtv1.VirtualMachine{},
					&virtv1.VirtualMachineInstance{},
				},
				RuntimeObjects: []runtime.Object{vm},
			})
			reconcileExecutor = testutil.NewReconcileExecutor(types.NamespacedName{Name: vmName, Namespace: nsName})

			err := reconciler.Client.Create(ctx, &virtv2.VirtualMachineIPAddressClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vmipName,
					Namespace: nsName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			vmip, err := helper.FetchObject(ctx, types.NamespacedName{Name: vmipName, Namespace: nsName}, reconciler.Client, &virtv2.VirtualMachineIPAddressClaim{})
			Expect(err).NotTo(HaveOccurred())
			Expect(vmip).NotTo(BeNil())

			CreateReadyVM(ctx, reconciler, reconcileExecutor, vm, vmd)

			By("Emulating kubevirt: create kubevirt VMI in Ready status")

			// Ensure kubevirt VMI is present.
			kvvmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: vmName, Namespace: nsName}, reconciler.Client, &virtv1.VirtualMachineInstance{})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(kvvmi).ShouldNot(BeNil(), "kubevirt VirtualMachineInstance should present")
		}

		{
			By("Changing memory size and cpu settings")
			// Change memory settings.
			vm, err := helper.FetchObject(ctx, types.NamespacedName{Name: vmName, Namespace: nsName}, reconciler.Client, &virtv2.VirtualMachine{})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(vm).ShouldNot(BeNil())

			// Set new memory size and cpu settings.
			vm.Spec.Memory.Size = memoryNewSize
			vm.Spec.CPU.Cores = cpuNewCores
			vm.Spec.CPU.CoreFraction = cpuNewCoreFraction

			// Update vm and reconcile new settings.
			err = reconciler.Client.Update(ctx, vm)
			Expect(err).ShouldNot(HaveOccurred())
			err = reconcileExecutor.Execute(ctx, reconciler)
			Expect(err).ShouldNot(HaveOccurred())
		}

		{
			By("Checking kubevirt VMI was not deleted")
			// Check that kubevirt VMI was not deleted.
			kvvmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: vmName, Namespace: nsName}, reconciler.Client, &virtv1.VirtualMachineInstance{})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(kvvmi).NotTo(BeNil(), "kubevirt VirtualMachineInstance should not be deleted")

			By("Checking changes are pending")
			vm, err := helper.FetchObject(ctx, types.NamespacedName{Name: vmName, Namespace: nsName}, reconciler.Client, &virtv2.VirtualMachine{})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(vm).ShouldNot(BeNil())

			Expect(vm.Status.RestartID).ShouldNot(BeEmpty(), "Should put changeID to the status")

			id := func(elem interface{}) string {
				return elem.(map[string]interface{})["path"].(string)
			}

			// Pending changes are Raw bytes, conversion to map[string]interface{} is needed to work with gstruct matchers.
			pendingChanges := make([]map[string]interface{}, 0, len(vm.Status.RestartAwaitingChanges))
			for _, change := range vm.Status.RestartAwaitingChanges {
				var changeMap map[string]interface{}
				err := json.Unmarshal(change.Raw, &changeMap)
				Expect(err).ShouldNot(HaveOccurred(), "should not fail unmarshaling for '%s'", string(change.Raw))
				pendingChanges = append(pendingChanges, changeMap)
			}

			Expect(pendingChanges).To(MatchAllElements(id, Elements{
				"cpu": MatchAllKeys(Keys{
					"path":      Equal("cpu"),
					"operation": Equal(string(vmchange.ChangeReplace)),
					"currentValue": MatchAllKeys(Keys{
						"cores":        BeEquivalentTo(cpuStartingCores),
						"coreFraction": Equal(cpuStartingCoreFraction),
					}),
					"desiredValue": MatchAllKeys(Keys{
						"cores":        BeEquivalentTo(cpuNewCores),
						"coreFraction": Equal(cpuNewCoreFraction),
					}),
				}),
				"memory.size": MatchAllKeys(Keys{
					"path":         Equal("memory.size"),
					"operation":    Equal(string(vmchange.ChangeReplace)),
					"currentValue": Equal(memoryStartingSize),
					"desiredValue": Equal(memoryNewSize),
				}),
			}))

			By("Approving pending changes")
			vm.Spec.RestartApprovalID = vm.Status.RestartID
			// Update vm and reconcile approval.
			err = reconciler.Client.Update(ctx, vm)
			Expect(err).ShouldNot(HaveOccurred())
			err = reconcileExecutor.Execute(ctx, reconciler)
			Expect(err).ShouldNot(HaveOccurred())
		}

		{
			By("Checking kubevirt VMI is gone")
			kvvmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: vmName, Namespace: nsName}, reconciler.Client, &virtv1.VirtualMachineInstance{})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(kvvmi).To(BeNil(), "kubevirt VirtualMachineInstance should be deleted after manual approval")

			By("Checking status is cleared")
			vm, err := helper.FetchObject(ctx, types.NamespacedName{Name: vmName, Namespace: nsName}, reconciler.Client, &virtv2.VirtualMachine{})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(vm).ShouldNot(BeNil())

			Expect(vm.Status.RestartID).Should(BeEmpty(), "Should clear changeID after manual approval")
			Expect(vm.Status.RestartAwaitingChanges).Should(BeEmpty(), "Should clear pendingChanges after manual approval")
		}
	})
})

func CreateReadyVM(ctx context.Context, reconciler *two_phase_reconciler.ReconcilerCore[*controller.VMReconcilerState], reconcileExecutor *testutil.ReconcileExecutor, vm *virtv2.VirtualMachine, vmd *virtv2.VirtualMachineDisk) {
	// Create Disk.
	err := reconciler.Client.Create(ctx, vmd)
	Expect(err).NotTo(HaveOccurred())

	// Emulate CDI converge: set Ready status directly.
	vmdObj, err := helper.FetchObject(ctx, types.NamespacedName{Name: vmd.Name, Namespace: vmd.Namespace}, reconciler.Client, &virtv2.VirtualMachineDisk{})
	Expect(err).NotTo(HaveOccurred())
	Expect(vmd).NotTo(BeNil())
	vmdObj.Status.Phase = virtv2.DiskReady
	err = reconciler.Client.Status().Update(ctx, vmd)
	Expect(err).NotTo(HaveOccurred())

	// Emulate Add VM with DisksReady.
	err = reconcileExecutor.Execute(ctx, reconciler)
	Expect(err).NotTo(HaveOccurred())

	kvvm, err := helper.FetchObject(ctx, types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}, reconciler.Client, &virtv1.VirtualMachine{})
	Expect(err).ShouldNot(HaveOccurred())
	Expect(kvvm).ShouldNot(BeNil())

	// Emulate kubevirt converge: create kubevirt VirtualMachineInstance based on specs from kubevirt VirtualMachine.
	kvvmi := &virtv1.VirtualMachineInstance{
		ObjectMeta: kvvm.Spec.Template.ObjectMeta,
		Spec:       kvvm.Spec.Template.Spec,
	}
	kvvmi.ObjectMeta.Name = kvvm.Name
	kvvmi.ObjectMeta.Namespace = kvvm.Namespace
	err = reconciler.Client.Create(ctx, kvvmi)
	Expect(err).NotTo(HaveOccurred())

	kvvmi.Status.Phase = virtv1.Running
	err = reconciler.Client.Status().Update(ctx, kvvmi)
	Expect(err).NotTo(HaveOccurred())

	kvvm.Status.Created = true
	kvvm.Status.Ready = true
	err = reconciler.Client.Status().Update(ctx, kvvm)
	Expect(err).NotTo(HaveOccurred())

	err = reconcileExecutor.Execute(ctx, reconciler)
	Expect(err).NotTo(HaveOccurred())
}
