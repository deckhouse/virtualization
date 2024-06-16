/*
Copyright 2024 Flant JSC

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

package controller

import (
	"context"
	"fmt"

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	vmiutil "github.com/deckhouse/virtualization-controller/pkg/common/vmi"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	virtv2alpha1 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func (r *VMIReconciler) startImporterPod(ctx context.Context, state *VMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	vmi := state.VMI.Current()
	opts.Log.V(1).Info("Creating importer POD for VI", "vi.Name", vmi.Name)

	importerSettings, err := r.createImporterSettings(state)
	if err != nil {
		return err
	}

	// all checks passed, let's create the importer pod!
	podSettings := r.createImporterPodSettings(state)

	imp := importer.NewImporter(podSettings, importerSettings)
	pod, err := imp.CreatePod(ctx, opts.Client)
	if err != nil {
		err = cc.PublishPodErr(err, podSettings.Name, vmi, opts.Recorder, opts.Client)
		if err != nil {
			return err
		}
	}

	opts.Log.V(1).Info("Created importer POD", "pod.Name", pod.Name)

	// Ensure supplement resources for the Pod.
	return supplements.EnsureForPod(ctx, opts.Client, state.Supplements, pod, datasource.NewCABundleForVMI(vmi.Spec.DataSource), r.dvcrSettings)
}

// createImporterSettings fills settings for the dvcr-importer binary.
func (r *VMIReconciler) createImporterSettings(state *VMIReconcilerState) (*importer.Settings, error) {
	settings := &importer.Settings{
		Verbose: r.verbose,
	}

	vmi := state.VMI.Current()
	ds := vmi.Spec.DataSource

	switch ds.Type {
	case virtv2alpha1.DataSourceTypeHTTP:
		if ds.HTTP == nil {
			return nil, fmt.Errorf("dataSource '%s' specified without related 'http' section", ds.Type)
		}

		importer.ApplyHTTPSourceSettings(settings, ds.HTTP, state.Supplements)
	case virtv2alpha1.DataSourceTypeContainerImage:
		if ds.ContainerImage == nil {
			return nil, fmt.Errorf("dataSource '%s' specified without related 'containerImage' section", ds.Type)
		}
		importer.ApplyRegistrySourceSettings(settings, ds.ContainerImage, state.Supplements)
	case virtv2alpha1.DataSourceTypeObjectRef:
		if ds.ObjectRef == nil {
			return nil, fmt.Errorf("dataSource '%s' specified without related 'objectRef' section", ds.Type)
		}

		switch ds.ObjectRef.Kind {
		case virtv2alpha1.VirtualImageObjectRefKindVirtualImage:
			dvcrSourceImageName := r.dvcrSettings.RegistryImageForVMI(ds.ObjectRef.Name, vmi.Namespace)
			importer.ApplyDVCRSourceSettings(settings, dvcrSourceImageName)
		case virtv2alpha1.VirtualImageObjectRefKindClusterVirtualImage:
			dvcrSourceImageName := r.dvcrSettings.RegistryImageForCVMI(ds.ObjectRef.Name)
			importer.ApplyDVCRSourceSettings(settings, dvcrSourceImageName)
		default:
			return nil, fmt.Errorf("unknown objectRef kind: %s", ds.ObjectRef.Kind)
		}
	default:
		return nil, fmt.Errorf("unknown dataSource: %s", ds.Type)
	}

	// Set DVCR destination settings.
	dvcrDestImageName := r.dvcrSettings.RegistryImageForVMI(vmi.Name, vmi.Namespace)
	importer.ApplyDVCRDestinationSettings(settings, r.dvcrSettings, state.Supplements, dvcrDestImageName)

	// TODO Update proxy settings.

	return settings, nil
}

func (r *VMIReconciler) createImporterPodSettings(state *VMIReconcilerState) *importer.PodSettings {
	importerPod := state.Supplements.ImporterPod()
	return &importer.PodSettings{
		Name:            importerPod.Name,
		Image:           r.importerImage,
		PullPolicy:      r.pullPolicy,
		Namespace:       importerPod.Namespace,
		OwnerReference:  vmiutil.MakeOwnerReference(state.VMI.Current()),
		ControllerName:  vmiControllerName,
		InstallerLabels: map[string]string{},
	}
}
