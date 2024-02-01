package kvapi

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

type Kubevirt interface {
	HotplugVolumesEnabled() bool
}

func New(cli client.Client, kv Kubevirt) *KvApi {
	return &KvApi{
		Client:   cli,
		kubevirt: kv,
	}
}

type KvApi struct {
	client.Client
	kubevirt Kubevirt
}

func (api *KvApi) AddVolume(ctx context.Context, namespace, name string, opts *virtv1.AddVolumeOptions) error {
	return api.addVolume(ctx, namespace, name, opts)
}

func (api *KvApi) RemoveVolume(ctx context.Context, namespace, name string, opts *virtv1.RemoveVolumeOptions) error {
	return api.removeVolume(ctx, namespace, name, opts)
}

func (api *KvApi) addVolume(ctx context.Context, namespace, name string, opts *virtv1.AddVolumeOptions) error {
	if !api.kubevirt.HotplugVolumesEnabled() {
		return fmt.Errorf("unable to add volume because HotplugVolumes feature gate is not enabled")
	}
	// Validate AddVolumeOptions
	switch {
	case opts.Name == "":
		return fmt.Errorf("AddVolumeOptions requires name to be set")
	case opts.Disk == nil:
		return fmt.Errorf("AddVolumeOptions requires disk to not be nil")
	case opts.VolumeSource == nil:
		return fmt.Errorf("AddVolumeOptions requires VolumeSource to not be nil")
	}

	opts.Disk.Name = opts.Name

	volumeRequest := virtv1.VirtualMachineVolumeRequest{
		AddVolumeOptions: opts,
	}

	switch {
	case opts.VolumeSource.DataVolume != nil:
		opts.VolumeSource.DataVolume.Hotpluggable = true
	case opts.VolumeSource.PersistentVolumeClaim != nil:
		opts.VolumeSource.PersistentVolumeClaim.Hotpluggable = true
	}

	return api.vmVolumePatchStatus(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &volumeRequest)
}

func (api *KvApi) removeVolume(ctx context.Context, namespace, name string, opts *virtv1.RemoveVolumeOptions) error {
	if !api.kubevirt.HotplugVolumesEnabled() {
		return fmt.Errorf("unable to remove volume because HotplugVolumes feature gate is not enabled")
	}

	if opts.Name == "" {
		return fmt.Errorf("RemoveVolumeOptions requires name to be set")
	}

	volumeRequest := virtv1.VirtualMachineVolumeRequest{
		RemoveVolumeOptions: opts,
	}

	return api.vmVolumePatchStatus(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &volumeRequest)
}

func (api *KvApi) vmVolumePatchStatus(ctx context.Context, key types.NamespacedName, volumeRequest *virtv1.VirtualMachineVolumeRequest) error {
	vm, err := helper.FetchObject(ctx, key, api.Client, &virtv1.VirtualMachine{})
	if err != nil {
		return err
	}
	err = verifyVolumeOption(vm.Spec.Template.Spec.Volumes, volumeRequest)
	if err != nil {
		return err
	}

	jp, err := generateVMVolumeRequestPatch(vm, volumeRequest)
	if err != nil {
		return err
	}

	dryRunOption := api.getDryRunOption(volumeRequest)
	err = api.Client.Status().Patch(ctx, vm,
		client.RawPatch(types.JSONPatchType, []byte(jp)),
		&client.SubResourcePatchOptions{
			PatchOptions: client.PatchOptions{DryRun: dryRunOption},
		})
	if err != nil {
		return fmt.Errorf("unable to patch kvvm: %w", err)
	}

	return nil
}

func (api *KvApi) getDryRunOption(volumeRequest *virtv1.VirtualMachineVolumeRequest) []string {
	var dryRunOption []string
	if options := volumeRequest.AddVolumeOptions; options != nil && options.DryRun != nil && options.DryRun[0] == metav1.DryRunAll {
		dryRunOption = volumeRequest.AddVolumeOptions.DryRun
	} else if options := volumeRequest.RemoveVolumeOptions; options != nil && options.DryRun != nil && options.DryRun[0] == metav1.DryRunAll {
		dryRunOption = volumeRequest.RemoveVolumeOptions.DryRun
	}
	return dryRunOption
}

func verifyVolumeOption(volumes []virtv1.Volume, volumeRequest *virtv1.VirtualMachineVolumeRequest) error {
	foundRemoveVol := false
	for _, volume := range volumes {
		if volumeRequest.AddVolumeOptions != nil {
			volSourceName := volumeSourceName(volumeRequest.AddVolumeOptions.VolumeSource)
			if volumeNameExists(volume, volumeRequest.AddVolumeOptions.Name) {
				return fmt.Errorf("unable to add volume [%s] because volume with that name already exists", volumeRequest.AddVolumeOptions.Name)
			}
			if volumeSourceExists(volume, volSourceName) {
				return fmt.Errorf("unable to add volume source [%s] because it already exists", volSourceName)
			}
		} else if volumeRequest.RemoveVolumeOptions != nil && volumeExists(volume, volumeRequest.RemoveVolumeOptions.Name) {
			if !volumeHotpluggable(volume) {
				return fmt.Errorf("unable to remove volume [%s] because it is not hotpluggable", volume.Name)
			}
			foundRemoveVol = true
			break
		}
	}

	if volumeRequest.RemoveVolumeOptions != nil && !foundRemoveVol {
		return fmt.Errorf("unable to remove volume [%s] because it does not exist", volumeRequest.RemoveVolumeOptions.Name)
	}

	return nil
}

func volumeSourceName(volumeSource *virtv1.HotplugVolumeSource) string {
	if volumeSource.DataVolume != nil {
		return volumeSource.DataVolume.Name
	}
	if volumeSource.PersistentVolumeClaim != nil {
		return volumeSource.PersistentVolumeClaim.ClaimName
	}
	return ""
}

func volumeExists(volume virtv1.Volume, volumeName string) bool {
	return volumeNameExists(volume, volumeName) || volumeSourceExists(volume, volumeName)
}

func volumeNameExists(volume virtv1.Volume, volumeName string) bool {
	return volume.Name == volumeName
}

func volumeSourceExists(volume virtv1.Volume, volumeName string) bool {
	return (volume.DataVolume != nil && volume.DataVolume.Name == volumeName) ||
		(volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == volumeName)
}

func volumeHotpluggable(volume virtv1.Volume) bool {
	return (volume.DataVolume != nil && volume.DataVolume.Hotpluggable) || (volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.Hotpluggable)
}

func generateVMVolumeRequestPatch(vm *virtv1.VirtualMachine, volumeRequest *virtv1.VirtualMachineVolumeRequest) (string, error) {
	vmCopy := vm.DeepCopy()

	// We only validate the list against other items in the list at this point.
	// The VM validation webhook will validate the list against the VMI spec
	// during the Patch command
	if volumeRequest.AddVolumeOptions != nil {
		if err := addAddVolumeRequests(vmCopy, volumeRequest); err != nil {
			return "", err
		}
	} else if volumeRequest.RemoveVolumeOptions != nil {
		if err := addRemoveVolumeRequests(vmCopy, volumeRequest); err != nil {
			return "", err
		}
	}

	verb := patch.PatchAddOp
	if len(vm.Status.VolumeRequests) > 0 {
		verb = patch.PatchReplaceOp
	}
	jop := patch.NewJsonPatchOperation(verb, "/status/volumeRequests", vmCopy.Status.VolumeRequests)
	jp := patch.NewJsonPatch(jop)

	return jp.String()
}

func addAddVolumeRequests(vm *virtv1.VirtualMachine, volumeRequest *virtv1.VirtualMachineVolumeRequest) error {
	name := volumeRequest.AddVolumeOptions.Name
	for _, request := range vm.Status.VolumeRequests {
		if err := validateAddVolumeRequest(request, name); err != nil {
			return err
		}
	}
	vm.Status.VolumeRequests = append(vm.Status.VolumeRequests, *volumeRequest)
	return nil
}

func validateAddVolumeRequest(request virtv1.VirtualMachineVolumeRequest, name string) error {
	if addVolumeRequestExists(request, name) {
		return fmt.Errorf("add volume request for volume [%s] already exists", name)
	}
	if removeVolumeRequestExists(request, name) {
		return fmt.Errorf("unable to add volume since a remove volume request for volume [%s] already exists and is still being processed", name)
	}
	return nil
}

func addRemoveVolumeRequests(vm *virtv1.VirtualMachine, volumeRequest *virtv1.VirtualMachineVolumeRequest) error {
	name := volumeRequest.RemoveVolumeOptions.Name
	var volumeRequestsList []virtv1.VirtualMachineVolumeRequest
	for _, request := range vm.Status.VolumeRequests {
		if addVolumeRequestExists(request, name) {
			// Filter matching AddVolume requests from the new list.
			continue
		}
		if removeVolumeRequestExists(request, name) {
			return fmt.Errorf("a remove volume request for volume [%s] already exists and is still being processed", name)
		}
		volumeRequestsList = append(volumeRequestsList, request)
	}
	volumeRequestsList = append(volumeRequestsList, *volumeRequest)
	vm.Status.VolumeRequests = volumeRequestsList
	return nil
}

func addVolumeRequestExists(request virtv1.VirtualMachineVolumeRequest, name string) bool {
	return request.AddVolumeOptions != nil && request.AddVolumeOptions.Name == name
}

func removeVolumeRequestExists(request virtv1.VirtualMachineVolumeRequest, name string) bool {
	return request.RemoveVolumeOptions != nil && request.RemoveVolumeOptions.Name == name
}
