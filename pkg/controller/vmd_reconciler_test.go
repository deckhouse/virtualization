package controller_test

import (
	"context"
	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/controller"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("VMD", func() {
	var (
		reconciler *two_phase_reconciler.ReconcilerCore[*controller.VMDReconcilerState]
	)

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

		{
			vmd := &virtv2.VirtualMachineDisk{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   metav1.NamespaceDefault,
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

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-vmd", Namespace: metav1.NamespaceDefault}})
		Expect(err).NotTo(HaveOccurred())

		dv, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vmd", Namespace: metav1.NamespaceDefault}, reconciler.Client, &cdiv1.DataVolume{})
		Expect(err).NotTo(HaveOccurred())
		Expect(dv).NotTo(BeNil())

		vmd, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vmd", Namespace: metav1.NamespaceDefault}, reconciler.Client, &virtv2.VirtualMachineDisk{})
		Expect(err).NotTo(HaveOccurred())
		Expect(vmd).NotTo(BeNil())
		Expect(vmd.Status.Phase).To(Equal(virtv2.DiskPending))
		Expect(vmd.Status.Progress).To(Equal(virtv2.DiskProgress("N/A")))

		dv.Status.Phase = cdiv1.CloneInProgress
		dv.Status.Progress = "50%"
		reconciler.Client.Status().Update(ctx, dv)

		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-vmd", Namespace: metav1.NamespaceDefault}})
		Expect(err).NotTo(HaveOccurred())

		vmd, err = helper.FetchObject(ctx, types.NamespacedName{Name: "test-vmd", Namespace: metav1.NamespaceDefault}, reconciler.Client, &virtv2.VirtualMachineDisk{})
		Expect(err).NotTo(HaveOccurred())
		Expect(vmd).NotTo(BeNil())
		Expect(vmd.Status.Phase).To(Equal(virtv2.DiskProvisioning))
		Expect(vmd.Status.Progress).To(Equal(virtv2.DiskProgress("50%")))

		dv.Status.Phase = cdiv1.Succeeded
		dv.Status.Progress = "100%"
		reconciler.Client.Status().Update(ctx, dv)

		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-vmd", Namespace: metav1.NamespaceDefault}})
		Expect(err).NotTo(HaveOccurred())

		vmd, err = helper.FetchObject(ctx, types.NamespacedName{Name: "test-vmd", Namespace: metav1.NamespaceDefault}, reconciler.Client, &virtv2.VirtualMachineDisk{})
		Expect(err).NotTo(HaveOccurred())
		Expect(vmd).NotTo(BeNil())
		Expect(vmd.Status.Phase).To(Equal(virtv2.DiskReady))
		Expect(vmd.Status.Progress).To(Equal(virtv2.DiskProgress("100%")))
	})
})
