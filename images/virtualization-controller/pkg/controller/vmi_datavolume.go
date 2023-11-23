package controller

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	dvutil "github.com/deckhouse/virtualization-controller/pkg/common/datavolume"
	vmiutil "github.com/deckhouse/virtualization-controller/pkg/common/vmi"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/copier"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/monitoring"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

// getPVCSize retrieves PVC size from importer Pod final report after import is done.
func (r *VMIReconciler) getPVCSize(state *VMIReconcilerState) (resource.Quantity, error) {
	finalReport, err := monitoring.GetFinalReportFromPod(state.Pod)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("importer Pod final report missing: %w", err)
	}

	unpackedSize := *resource.NewQuantity(int64(finalReport.UnpackedSizeBytes), resource.BinarySI)
	if unpackedSize.IsZero() {
		return resource.Quantity{}, errors.New("no unpacked size in final report")
	}

	return unpackedSize, nil
}

func (r *VMIReconciler) createDataVolume(ctx context.Context, vmi *virtv2.VirtualMachineImage, state *VMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	// Retrieve PVC size.
	pvcSize, err := r.getPVCSize(state)
	if err != nil {
		return err
	}

	dvName := types.NamespacedName{Name: vmi.GetAnnotations()[cc.AnnVMIDataVolume], Namespace: vmi.GetNamespace()}

	dv, err := r.makeDataVolumeFromVMI(vmi, dvName, pvcSize)
	if err != nil {
		return err
	}

	if err = opts.Client.Create(ctx, dv); err != nil {
		opts.Log.V(2).Info("Error create new DV spec", "dv.spec", dv.Spec)
		return fmt.Errorf("create DataVolume/%s for VMI/%s: %w", dv.GetName(), vmi.GetName(), err)
	}
	opts.Log.Info("Created new DV", "dv.name", dv.GetName())
	opts.Log.V(2).Info("Created new DV spec", "dv.spec", dv.Spec)

	if vmiutil.IsTwoPhaseImport(vmi) {
		// Copy auth credentials and ca bundle to access DVCR as 'registry' data source.
		// Set DV as an ownerRef to auto-cleanup these copies.
		dvRef := dvutil.MakeOwnerReference(dv)
		return r.copyDVCRSecrets(ctx, opts.Client, vmi, dv.Name, dvRef)
	}

	return nil
}

// makeDataVolumeFromVMD makes DataVolume with 'registry' dataSource to import
// DVCR image onto PVC.
func (r *VMIReconciler) makeDataVolumeFromVMI(vmi *virtv2.VirtualMachineImage, dvName types.NamespacedName, pvcSize resource.Quantity) (*cdiv1.DataVolume, error) {
	dvBuilder := kvbuilder.NewDV(dvName)

	// Set datasource:
	// 'registry' if import is two phased.
	switch {
	case vmiutil.IsTwoPhaseImport(vmi):
		// The image was preloaded from source into dvcr.
		// We can't use the same data source a second time, but we can set dvcr as the data source.
		// Use DV name for the Secret with DVCR auth and the ConfigMap with DVCR CA Bundle.
		dvcrSourceImageName := r.dvcrSettings.RegistryImageForVMI(vmi.Name, vmi.Namespace)
		dvBuilder.SetRegistryDataSource(dvcrSourceImageName, dvName.Name, dvName.Name)
	default:
		return nil, fmt.Errorf("unsupported dataSource type %q", vmiutil.GetDataSourceType(vmi))
	}

	dvBuilder.SetPVC(vmi.Spec.PersistentVolumeClaim.StorageClassName, pvcSize)

	dvBuilder.SetOwnerRef(vmi, vmi.GetObjectKind().GroupVersionKind())
	dvBuilder.AddFinalizer(virtv2.FinalizerDVProtection)

	return dvBuilder.GetResource(), nil
}

// copyDVCRSecrets copies auth Secret and ca bundle ConfigMap to access DVCR by CDI.
func (r *VMIReconciler) copyDVCRSecrets(ctx context.Context, client client.Client, vmi *virtv2.VirtualMachineImage, targetName string, ownerRef metav1.OwnerReference) error {
	if r.dvcrSettings.AuthSecret != "" {
		authCopier := &copier.AuthSecret{
			Source: types.NamespacedName{
				Name:      r.dvcrSettings.AuthSecret,
				Namespace: r.dvcrSettings.AuthSecretNamespace,
			},
			Destination: types.NamespacedName{
				Name:      targetName,
				Namespace: vmi.GetNamespace(),
			},
			OwnerReference: ownerRef,
		}

		err := authCopier.CopyCDICompatible(ctx, client, r.dvcrSettings.RegistryURL)
		if err != nil {
			return err
		}
	}

	if r.dvcrSettings.CertsSecret != "" {
		caBundleCopier := &copier.CABundleConfigMap{
			SourceSecret: types.NamespacedName{
				Name:      r.dvcrSettings.CertsSecret,
				Namespace: r.dvcrSettings.CertsSecretNamespace,
			},
			Destination: types.NamespacedName{
				Name:      targetName,
				Namespace: vmi.GetNamespace(),
			},
			OwnerReference: ownerRef,
		}

		return caBundleCopier.Copy(ctx, client)
	}

	return nil
}
