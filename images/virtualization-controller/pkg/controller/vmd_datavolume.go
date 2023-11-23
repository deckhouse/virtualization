package controller

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	dvutil "github.com/deckhouse/virtualization-controller/pkg/common/datavolume"
	vmdutil "github.com/deckhouse/virtualization-controller/pkg/common/vmd"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/copier"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/monitoring"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

func (r *VMDReconciler) getPVCSize(vmd *virtv2.VirtualMachineDisk, state *VMDReconcilerState, opts two_phase_reconciler.ReconcilerOptions) (resource.Quantity, error) {
	pvcSize := vmd.Spec.PersistentVolumeClaim.Size

	if vmd.Spec.DataSource == nil {
		if vmd.Spec.PersistentVolumeClaim.Size.IsZero() {
			return resource.Quantity{}, errors.New("spec.pvc.size cannot be zero for blank VMD")
		}

		return pvcSize, nil
	}

	finalReport, err := monitoring.GetFinalReportFromPod(state.Pod)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("cannot create PVC without final report from the Pod: %w", err)
	}

	unpackedSize := *resource.NewQuantity(int64(finalReport.UnpackedSizeBytes), resource.BinarySI)
	if unpackedSize.IsZero() {
		return resource.Quantity{}, errors.New("no unpacked size in final report")
	}

	switch {
	case pvcSize.IsZero():
		// Set the resulting size from the importer/uploader pod.
		pvcSize = unpackedSize
	case pvcSize.Cmp(unpackedSize) == -1:
		opts.Recorder.Event(state.VMD.Current(), corev1.EventTypeWarning, virtv2.ReasonErrWrongPVCSize, "The specified spec.PersistentVolumeClaim.size cannot be smaller than the size of image in spec.dataSource")

		return resource.Quantity{}, errors.New("the specified spec.PersistentVolumeClaim.size cannot be smaller than the size of image in spec.dataSource")
	}

	return pvcSize, nil
}

// createDataVolume creates DataVolume resource to copy image from DVCR to PVC.
func (r *VMDReconciler) createDataVolume(ctx context.Context, vmd *virtv2.VirtualMachineDisk, state *VMDReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	// Retrieve PVC size.
	pvcSize, err := r.getPVCSize(vmd, state, opts)
	if err != nil {
		return err
	}

	dvName := types.NamespacedName{Name: vmd.GetAnnotations()[cc.AnnVMDDataVolume], Namespace: vmd.GetNamespace()}

	dv, err := r.makeDataVolumeFromVMD(vmd, dvName, pvcSize)
	if err != nil {
		return fmt.Errorf("apply VMD spec to DataVolume: %w", err)
	}

	opts.Log.V(2).Info(fmt.Sprintf("DV gvk before Create: %s", dv.GetObjectKind().GroupVersionKind().String()))

	if err = opts.Client.Create(ctx, dv); err != nil {
		opts.Log.V(2).Info("Error create new DV spec", "dv.spec", dv.Spec)
		return fmt.Errorf("create DataVolume/%s for VMD/%s: %w", dv.GetName(), vmd.GetName(), err)
	}
	opts.Log.Info("Created new DV", "dv.name", dv.GetName())
	opts.Log.V(2).Info("Created new DV spec", "dv.spec", dv.Spec, "dv.gvk", dv.GetObjectKind().GroupVersionKind())

	if vmdutil.IsTwoPhaseImport(vmd) {
		// Copy auth credentials and ca bundle to access DVCR as 'registry' data source.
		// Set DV as an ownerRef to auto-cleanup these copies.
		dvRef := dvutil.MakeOwnerReference(dv)
		opts.Log.V(2).Info("Created DV OwnerRef", "ownerRef", dvRef)
		return r.copyDVCRSecrets(ctx, opts.Client, vmd, dv.Name, dvRef)
	}

	return nil
}

// makeDataVolumeFromVMD makes DataVolume with 'registry' dataSource to import
// DVCR image onto PVC.
func (r *VMDReconciler) makeDataVolumeFromVMD(vmd *virtv2.VirtualMachineDisk, dvName types.NamespacedName, pvcSize resource.Quantity) (*cdiv1.DataVolume, error) {
	dvBuilder := kvbuilder.NewDV(dvName)

	// Set datasource:
	// 'registry' if import is two phased.
	// 'blank' if vmd has no datasource.
	switch {
	case vmdutil.IsTwoPhaseImport(vmd):
		// The image was preloaded from source into dvcr.
		// We can't use the same data source a second time, but we can set dvcr as the data source.
		// Use DV name for the Secret with DVCR auth and the ConfigMap with DVCR CA Bundle.
		dvcrImageName := dvcr.RegistryImageName(r.dvcrSettings, dvcr.ImagePathForVMD(vmd))
		dvBuilder.SetRegistryDataSource(dvcrImageName, dvName.Name, dvName.Name)
	case vmdutil.IsBlankPVC(vmd):
		dvBuilder.SetBlankDataSource()
	default:
		return nil, fmt.Errorf("unsupported dataSource type %q", vmdutil.GetDataSourceType(vmd))
	}

	dvBuilder.SetPVC(vmd.Spec.PersistentVolumeClaim.StorageClassName, pvcSize)

	dvBuilder.SetOwnerRef(vmd, vmd.GetObjectKind().GroupVersionKind())
	dvBuilder.AddFinalizer(virtv2.FinalizerDVProtection)

	return dvBuilder.GetResource(), nil
}

func (r *VMDReconciler) copyDVCRSecrets(ctx context.Context, client client.Client, vmd *virtv2.VirtualMachineDisk, targetName string, ownerRef metav1.OwnerReference) error {
	authCopier := &copier.AuthSecret{
		Source: types.NamespacedName{
			Name:      r.dvcrSettings.AuthSecret,
			Namespace: r.dvcrSettings.AuthSecretNamespace,
		},
		Destination: types.NamespacedName{
			Name:      targetName,
			Namespace: vmd.GetNamespace(),
		},
		OwnerReference: ownerRef,
	}

	err := authCopier.CopyCDICompatible(ctx, client, r.dvcrSettings.RegistryURL)
	if err != nil {
		return err
	}

	caBundleCopier := &copier.CABundleConfigMap{
		SourceSecret: types.NamespacedName{
			Name:      r.dvcrSettings.CertsSecret,
			Namespace: r.dvcrSettings.CertsSecretNamespace,
		},
		Destination: types.NamespacedName{
			Name:      targetName,
			Namespace: vmd.GetNamespace(),
		},
		OwnerReference: ownerRef,
	}

	return caBundleCopier.Copy(ctx, client)
}
