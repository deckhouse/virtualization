package controller_test

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/controller"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/util"
)

var _ = Describe("VMD", func() {
	var reconciler *two_phase_reconciler.ReconcilerCore[*controller.VMDReconcilerState]

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

	It("Successfully imports image by HTTP source", func() {
		ctx := context.Background()

		var pvcName string

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
			}
			reconciler = controller.NewVMDReconciler(vmd)
		}

		{
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-vmd", Namespace: "test-ns"}})
			Expect(err).NotTo(HaveOccurred())

			vmd, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vmd", Namespace: "test-ns"}, reconciler.Client, &virtv2.VirtualMachineDisk{})
			Expect(err).NotTo(HaveOccurred())
			Expect(vmd).NotTo(BeNil())
			Expect(vmd.Status.Phase).To(Equal(virtv2.DiskPending))
			Expect(vmd.Status.Progress).To(Equal(virtv2.DiskProgress("N/A")))
			Expect(vmd.Status.Size).To(Equal(""))

			pvcName = vmd.Status.PersistentVolumeClaimName

			dv, err := helper.FetchObject(ctx, types.NamespacedName{Name: pvcName, Namespace: "test-ns"}, reconciler.Client, &cdiv1.DataVolume{})
			Expect(err).NotTo(HaveOccurred())
			Expect(dv).NotTo(BeNil())
		}

		{
			dv, err := helper.FetchObject(ctx, types.NamespacedName{Name: pvcName, Namespace: "test-ns"}, reconciler.Client, &cdiv1.DataVolume{})
			Expect(err).NotTo(HaveOccurred())
			Expect(dv).NotTo(BeNil())
			dv.Status.Phase = cdiv1.Pending
			err = reconciler.Client.Status().Update(ctx, dv)
			Expect(err).NotTo(HaveOccurred())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-vmd", Namespace: "test-ns"}})
			Expect(err).NotTo(HaveOccurred())

			vmd, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vmd", Namespace: "test-ns"}, reconciler.Client, &virtv2.VirtualMachineDisk{})
			Expect(err).NotTo(HaveOccurred())
			Expect(vmd).NotTo(BeNil())
			Expect(vmd.Status.Phase).To(Equal(virtv2.DiskPending))
			Expect(vmd.Status.Progress).To(Equal(virtv2.DiskProgress("N/A")))
			Expect(vmd.Status.Size).To(Equal(""))
		}

		{
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "test-ns",
					Name:        pvcName,
					Labels:      nil,
					Annotations: nil,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: util.GetPointer("local-path"),
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Capacity: corev1.ResourceList{
						corev1.ResourceRequestsStorage: resource.MustParse("15Gi"),
					},
				},
			}
			err := reconciler.Client.Create(ctx, pvc)
			Expect(err).NotTo(HaveOccurred())

			dv, err := helper.FetchObject(ctx, types.NamespacedName{Name: pvcName, Namespace: "test-ns"}, reconciler.Client, &cdiv1.DataVolume{})
			Expect(err).NotTo(HaveOccurred())
			Expect(dv).NotTo(BeNil())
			dv.Status.Phase = cdiv1.CloneInProgress
			dv.Status.Progress = "50%"
			err = reconciler.Client.Status().Update(ctx, dv)
			Expect(err).NotTo(HaveOccurred())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-vmd", Namespace: "test-ns"}})
			Expect(err).NotTo(HaveOccurred())

			vmd, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vmd", Namespace: "test-ns"}, reconciler.Client, &virtv2.VirtualMachineDisk{})
			Expect(err).NotTo(HaveOccurred())
			Expect(vmd).NotTo(BeNil())
			Expect(vmd.Status.Phase).To(Equal(virtv2.DiskProvisioning))
			Expect(vmd.Status.Progress).To(Equal(virtv2.DiskProgress("50%")))
			Expect(vmd.Status.Size).To(Equal("15Gi"))
		}

		{
			dv, err := helper.FetchObject(ctx, types.NamespacedName{Name: pvcName, Namespace: "test-ns"}, reconciler.Client, &cdiv1.DataVolume{})
			Expect(err).NotTo(HaveOccurred())
			Expect(dv).NotTo(BeNil())
			dv.Status.Phase = cdiv1.Succeeded
			dv.Status.Progress = "100%"
			err = reconciler.Client.Status().Update(ctx, dv)
			Expect(err).NotTo(HaveOccurred())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-vmd", Namespace: "test-ns"}})
			Expect(err).NotTo(HaveOccurred())

			vmd, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vmd", Namespace: "test-ns"}, reconciler.Client, &virtv2.VirtualMachineDisk{})
			Expect(err).NotTo(HaveOccurred())
			Expect(vmd).NotTo(BeNil())
			Expect(vmd.Status.Phase).To(Equal(virtv2.DiskReady))
			Expect(strings.HasPrefix(vmd.Status.PersistentVolumeClaimName, "virtual-machine-disk-")).To(BeTrue(), fmt.Sprintf("unexpected PVC name %q", vmd.Status.PersistentVolumeClaimName))
			// UUID suffix
			Expect(len(vmd.Status.PersistentVolumeClaimName)).To(Equal(21 + 36))
			Expect(vmd.Status.Progress).To(Equal(virtv2.DiskProgress("100%")))
			Expect(vmd.Status.Size).To(Equal("15Gi"))
		}
	})
})
