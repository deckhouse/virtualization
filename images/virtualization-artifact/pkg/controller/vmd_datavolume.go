package controller

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	dvutil "github.com/deckhouse/virtualization-controller/pkg/common/datavolume"
	vmdutil "github.com/deckhouse/virtualization-controller/pkg/common/vmd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/monitoring"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var (
	ErrDataSourceNotReady             = errors.New("data source is not ready")
	ErrPVCSizeSmallerImageVirtualSize = errors.New("persistentVolumeClaim size is smaller than image virtual size")
)

func (r *VMDReconciler) getPVCSize(vmd *virtv2.VirtualDisk, state *VMDReconcilerState, opts two_phase_reconciler.ReconcilerOptions) (resource.Quantity, error) {
	pvcSize := vmd.Spec.PersistentVolumeClaim.Size

	if vmdutil.IsBlankPVC(vmd) {
		if pvcSize == nil || pvcSize.IsZero() {
			return resource.Quantity{}, errors.New("spec.persistentVolumeClaim.size should be set for blank VMD")
		}

		return *pvcSize, nil
	}

	var unpackedSize resource.Quantity

	switch {
	case vmdutil.IsTwoPhaseImport(vmd):
		// Get size from the importer Pod to detect if specified PVC size is enough.
		finalReport, err := monitoring.GetFinalReportFromPod(state.Pod)
		if err != nil {
			return resource.Quantity{}, fmt.Errorf("cannot create PVC without final report from the Pod: %w", err)
		}

		unpackedSize = *resource.NewQuantity(int64(finalReport.UnpackedSizeBytes), resource.BinarySI)
	case vmdutil.IsDVCRSource(vmd):
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

	if pvcSize != nil && !pvcSize.IsZero() && pvcSize.Cmp(unpackedSize) == -1 {
		opts.Recorder.Event(state.VMD.Current(), corev1.EventTypeWarning, virtv2.ReasonErrWrongPVCSize, ErrPVCSizeSmallerImageVirtualSize.Error())

		return resource.Quantity{}, ErrPVCSizeSmallerImageVirtualSize
	}

	// Adjust PVC size to feat image onto scratch PVC.
	// TODO(future): remove size adjusting after get rid of scratch.
	adjustedSize := dvutil.AdjustPVCSize(unpackedSize)

	if pvcSize != nil && pvcSize.Cmp(adjustedSize) == 1 {
		return *pvcSize, nil
	}

	return adjustedSize, nil
}

// createDataVolume creates DataVolume resource to copy image from DVCR to PVC.
func (r *VMDReconciler) createDataVolume(ctx context.Context, vmd *virtv2.VirtualDisk, state *VMDReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	// Retrieve PVC size.
	pvcSize, err := r.getPVCSize(vmd, state, opts)
	if err != nil {
		return fmt.Errorf("failed to get pvc size: %w", err)
	}

	dv, err := r.makeDataVolumeFromVMD(state, state.Supplements.DataVolume(), pvcSize)
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

	if vmdutil.IsTwoPhaseImport(vmd) || vmdutil.IsDVCRSource(vmd) {
		// Copy auth credentials and ca bundle to access DVCR as 'registry' data source.
		// Set DV as an ownerRef to auto-cleanup these copies on DV deletion.
		err = supplements.EnsureForDataVolume(ctx, opts.Client, state.Supplements, dv, r.dvcrSettings)
		if err != nil {
			return fmt.Errorf("failed to ensure data volume supplements: %w", err)
		}
	}

	return nil
}

// makeDataVolumeFromVMD makes DataVolume with 'registry' dataSource to import
// DVCR image onto PVC.
func (r *VMDReconciler) makeDataVolumeFromVMD(state *VMDReconcilerState, dvName types.NamespacedName, pvcSize resource.Quantity) (*cdiv1.DataVolume, error) {
	dvBuilder := kvbuilder.NewDV(dvName)
	vmd := state.VMD.Current()
	ds := vmd.Spec.DataSource

	authSecretName := state.Supplements.DVCRAuthSecretForDV().Name
	caBundleName := state.Supplements.DVCRCABundleConfigMapForDV().Name

	// Set datasource:
	// 'registry' if import is two phased.
	// 'blank' if vmd has no datasource.
	// TODO(refactor) Remove switch if there are only 2 options for the DataVolume source: DVCR and blank.
	switch {
	case vmdutil.IsTwoPhaseImport(vmd):
		// The image was preloaded from source into dvcr.
		// We can't use the same data source a second time, but we can set dvcr as the data source.
		// Use DV name for the Secret with DVCR auth and the ConfigMap with DVCR CA Bundle.
		dvcrSourceImageName := r.dvcrSettings.RegistryImageForVMD(vmd.Name, vmd.Namespace)
		dvBuilder.SetRegistryDataSource(dvcrSourceImageName, authSecretName, caBundleName)
	case ds != nil && ds.Type == virtv2.DataSourceTypeObjectRef:
		if ds.ObjectRef == nil {
			return nil, fmt.Errorf("nil objectRef %q", vmdutil.GetDataSourceType(vmd))
		}

		switch ds.ObjectRef.Kind {
		case virtv2.VirtualDiskObjectRefKindVirtualImage:
			dvcrSourceImageName := r.dvcrSettings.RegistryImageForVMI(ds.ObjectRef.Name, vmd.Namespace)
			dvBuilder.SetRegistryDataSource(dvcrSourceImageName, authSecretName, caBundleName)
		case virtv2.VirtualDiskObjectRefKindClusterVirtualImage:
			dvcrSourceImageName := r.dvcrSettings.RegistryImageForCVMI(ds.ObjectRef.Name)
			dvBuilder.SetRegistryDataSource(dvcrSourceImageName, authSecretName, caBundleName)
		default:
			return nil, fmt.Errorf("unsupported object ref kind %q", ds.ObjectRef.Kind)
		}
	case vmdutil.IsBlankPVC(vmd):
		dvBuilder.SetBlankDataSource()
	default:
		return nil, fmt.Errorf("unsupported dataSource type %q", vmdutil.GetDataSourceType(vmd))
	}

	dvBuilder.SetPVC(vmd.Spec.PersistentVolumeClaim.StorageClass, pvcSize)

	dvBuilder.SetOwnerRef(vmd, vmd.GetObjectKind().GroupVersionKind())
	dvBuilder.AddFinalizer(virtv2.FinalizerDVProtection)

	return dvBuilder.GetResource(), nil
}
