package controller

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v1alpha2"
	dvutil "github.com/deckhouse/virtualization-controller/pkg/common/datavolume"
	vmiutil "github.com/deckhouse/virtualization-controller/pkg/common/vmi"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/monitoring"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

// getPVCSize retrieves PVC size from importer Pod final report after import is done.
func (r *VMIReconciler) getPVCSize(vmi *virtv2.VirtualMachineImage, state *VMIReconcilerState) (resource.Quantity, error) {
	var unpackedSize resource.Quantity

	switch {
	case vmiutil.IsTwoPhaseImport(vmi):
		// Get size from the importer Pod to detect if specified PVC size is enough.
		finalReport, err := monitoring.GetFinalReportFromPod(state.Pod)
		if err != nil {
			return resource.Quantity{}, fmt.Errorf("cannot create PVC without final report from the Pod: %w", err)
		}

		unpackedSize = *resource.NewQuantity(int64(finalReport.UnpackedSizeBytes), resource.BinarySI)
	case vmiutil.IsDVCRSource(vmi):
		var err error
		unpackedSize, err = resource.ParseQuantity(state.DVCRDataSource.GetSize().UnpackedBytes)
		if err != nil {
			return resource.Quantity{}, err
		}
	default:
		return resource.Quantity{}, errors.New("failed to get unpacked size from data source")
	}

	if unpackedSize.IsZero() {
		return resource.Quantity{}, errors.New("got zero unpacked size from data source")
	}

	// Adjust PVC size to feat image onto scratch PVC.
	// TODO(future): remove after get rid of scratch.
	size := dvutil.AdjustPVCSize(unpackedSize)

	return size, nil
}

func (r *VMIReconciler) createDataVolume(ctx context.Context, vmi *virtv2.VirtualMachineImage, state *VMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	// Retrieve PVC size.
	pvcSize, err := r.getPVCSize(vmi, state)
	if err != nil {
		return err
	}

	dv, err := r.makeDataVolumeFromVMI(state, state.Supplements.DataVolume(), pvcSize)
	if err != nil {
		return err
	}

	if err = opts.Client.Create(ctx, dv); err != nil {
		opts.Log.V(2).Info("Error create new DV spec", "dv.spec", dv.Spec)
		return fmt.Errorf("create DataVolume/%s for VMI/%s: %w", dv.GetName(), vmi.GetName(), err)
	}
	opts.Log.Info("Created new DV", "dv.name", dv.GetName())
	opts.Log.V(2).Info("Created new DV spec", "dv.spec", dv.Spec)

	if vmiutil.IsTwoPhaseImport(vmi) || vmiutil.IsDVCRSource(vmi) {
		// Copy auth credentials and ca bundle to access DVCR as 'registry' data source.
		// Set DV as an ownerRef to auto-cleanup these copies.
		err = supplements.EnsureForDataVolume(ctx, opts.Client, state.Supplements, dv, r.dvcrSettings)
		if err != nil {
			return fmt.Errorf("failed to ensure data volume supplements: %w", err)
		}
	}

	return nil
}

// makeDataVolumeFromVMD makes DataVolume with 'registry' dataSource to import
// DVCR image onto PVC.
func (r *VMIReconciler) makeDataVolumeFromVMI(state *VMIReconcilerState, dvName types.NamespacedName, pvcSize resource.Quantity) (*cdiv1.DataVolume, error) {
	dvBuilder := kvbuilder.NewDV(dvName)
	vmi := state.VMI.Current()
	ds := vmi.Spec.DataSource

	authSecretName := state.Supplements.DVCRAuthSecretForDV().Name
	caBundleName := state.Supplements.DVCRCABundleConfigMapForDV().Name

	// Set datasource:
	// 'registry' if import is two phased.
	switch {
	case vmiutil.IsTwoPhaseImport(vmi):
		// The image was preloaded from source into dvcr.
		// We can't use the same data source a second time, but we can set dvcr as the data source.
		// Use DV name for the Secret with DVCR auth and the ConfigMap with DVCR CA Bundle.
		dvcrSourceImageName := r.dvcrSettings.RegistryImageForVMI(vmi.Name, vmi.Namespace)
		dvBuilder.SetRegistryDataSource(dvcrSourceImageName, authSecretName, caBundleName)
	case ds.Type == virtv2.DataSourceTypeClusterVirtualMachineImage:
		dvcrSourceImageName := r.dvcrSettings.RegistryImageForCVMI(ds.ClusterVirtualMachineImage.Name)
		dvBuilder.SetRegistryDataSource(dvcrSourceImageName, authSecretName, caBundleName)
	case ds.Type == virtv2.DataSourceTypeVirtualMachineImage:
		vmiRef := ds.VirtualMachineImage
		// NOTE: use namespace from current VMI.
		dvcrSourceImageName := r.dvcrSettings.RegistryImageForVMI(vmiRef.Name, vmi.Namespace)
		dvBuilder.SetRegistryDataSource(dvcrSourceImageName, authSecretName, caBundleName)
	default:
		return nil, fmt.Errorf("unsupported dataSource type %q", vmiutil.GetDataSourceType(vmi))
	}

	dvBuilder.SetPVC(vmi.Spec.PersistentVolumeClaim.StorageClassName, pvcSize)

	dvBuilder.SetOwnerRef(vmi, vmi.GetObjectKind().GroupVersionKind())
	dvBuilder.AddFinalizer(virtv2.FinalizerDVProtection)

	return dvBuilder.GetResource(), nil
}
