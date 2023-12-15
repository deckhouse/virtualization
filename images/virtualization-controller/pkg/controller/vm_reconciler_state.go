package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	merger "github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmattachee"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/util"
)

type VMReconcilerState struct {
	Client     client.Client
	VM         *helper.Resource[*virtv2.VirtualMachine, virtv2.VirtualMachineStatus]
	KVVM       *virtv1.VirtualMachine
	KVVMI      *virtv1.VirtualMachineInstance
	KVPods     *corev1.PodList
	VMDByName  map[string]*virtv2.VirtualMachineDisk
	VMIByName  map[string]*virtv2.VirtualMachineImage
	CVMIByName map[string]*virtv2.ClusterVirtualMachineImage

	IPAddressClaim *virtv2.VirtualMachineIPAddressClaim

	Result        *reconcile.Result
	StatusMessage string
	ChangeID      string
}

func NewVMReconcilerState(name types.NamespacedName, log logr.Logger, client client.Client, cache cache.Cache) *VMReconcilerState {
	return &VMReconcilerState{
		Client: client,
		VM: helper.NewResource(
			name, log, client, cache,
			func() *virtv2.VirtualMachine { return &virtv2.VirtualMachine{} },
			func(obj *virtv2.VirtualMachine) virtv2.VirtualMachineStatus { return obj.Status },
		),
	}
}

func (state *VMReconcilerState) ApplySync(ctx context.Context, _ logr.Logger) error {
	if err := state.VM.UpdateMeta(ctx); err != nil {
		return fmt.Errorf("unable to update VM %q meta: %w", state.VM.Name(), err)
	}
	return nil
}

func (state *VMReconcilerState) ApplyUpdateStatus(ctx context.Context, _ logr.Logger) error {
	return state.VM.UpdateStatus(ctx)
}

func (state *VMReconcilerState) SetReconcilerResult(result *reconcile.Result) {
	state.Result = result
}

func (state *VMReconcilerState) GetReconcilerResult() *reconcile.Result {
	return state.Result
}

func (state *VMReconcilerState) SetChangeID(changeID string) {
	state.ChangeID = changeID
}

func (state *VMReconcilerState) Reload(ctx context.Context, req reconcile.Request, log logr.Logger, _ client.Client) error {
	err := state.VM.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}

	if state.VM.IsEmpty() {
		log.Info("Reconcile observe an absent VM: it may be deleted", "VM", req.NamespacedName)
		return nil
	}

	claimName := state.VM.Current().Spec.VirtualMachineIPAddressClaimName
	if claimName == "" {
		claimName = state.VM.Name().Name
	}

	claimKey := types.NamespacedName{Name: claimName, Namespace: state.VM.Name().Namespace}
	state.IPAddressClaim, err = helper.FetchObject(ctx, claimKey, state.Client, &virtv2.VirtualMachineIPAddressClaim{})
	if err != nil {
		return fmt.Errorf("unable to get Claim %s: %w", claimKey, err)
	}

	kvvmName := state.VM.Name()
	kvvm, err := helper.FetchObject(ctx, kvvmName, state.Client, &virtv1.VirtualMachine{})
	if err != nil {
		return fmt.Errorf("unable to get KubeVirt VM %q: %w", kvvmName, err)
	}
	state.KVVM = kvvm

	if state.KVVM != nil {
		if state.KVVM.Status.Created {
			// FIXME(VM): ObservedGeneration & DesiredGeneration only available since KubeVirt 1.0.0 which is only prereleased at the moment
			// FIXME(VM): Uncomment following check when KubeVirt updated to 1.0.0
			// if state.KVVM.Status.ObservedGeneration == state.KVVM.Status.DesiredGeneration {
			kvvmi, err := helper.FetchObject(ctx, kvvmName, state.Client, &virtv1.VirtualMachineInstance{})
			if err != nil {
				return fmt.Errorf("unable to get KubeVirt VMI %q: %w", kvvmName, err)
			}
			state.KVVMI = kvvmi
			//}
		}
	}

	// Search for virt-launcher Pods if KubeVirt VMI exists for VM.
	if state.KVVMI != nil {
		pods := new(corev1.PodList)
		selector := labels.SelectorFromSet(map[string]string{"vm.kubevirt.io/name": state.KVVM.GetName()})
		err = state.Client.List(ctx, pods, &client.ListOptions{
			LabelSelector: selector,
			Namespace:     kvvm.Namespace,
		})
		if err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("unable to list virt-launcher Pod for KubeVirt VM %q: %w", kvvmName, err)
		}
		if len(pods.Items) > 0 {
			state.KVPods = pods
		}
	}

	var vmdByName map[string]*virtv2.VirtualMachineDisk
	var vmiByName map[string]*virtv2.VirtualMachineImage
	var cvmiByName map[string]*virtv2.ClusterVirtualMachineImage

	for _, bd := range state.VM.Current().Spec.BlockDevices {
		switch bd.Type {
		case virtv2.ImageDevice:
			vmi, err := helper.FetchObject(ctx, types.NamespacedName{
				Name:      bd.VirtualMachineImage.Name,
				Namespace: state.VM.Name().Namespace,
			}, state.Client, &virtv2.VirtualMachineImage{})
			if err != nil {
				return fmt.Errorf("unable to get VMI %q: %w", bd.VirtualMachineImage.Name, err)
			}
			if vmi == nil {
				continue
			}
			if vmiByName == nil {
				vmiByName = make(map[string]*virtv2.VirtualMachineImage)
			}
			vmiByName[bd.VirtualMachineImage.Name] = vmi

		case virtv2.ClusterImageDevice:
			cvmi, err := helper.FetchObject(ctx, types.NamespacedName{
				Name: bd.ClusterVirtualMachineImage.Name,
			}, state.Client, &virtv2.ClusterVirtualMachineImage{})
			if err != nil {
				return fmt.Errorf("unable to get CVMI %q: %w", bd.ClusterVirtualMachineImage.Name, err)
			}
			if cvmi == nil {
				continue
			}
			if cvmiByName == nil {
				cvmiByName = make(map[string]*virtv2.ClusterVirtualMachineImage)
			}
			cvmiByName[bd.ClusterVirtualMachineImage.Name] = cvmi

		case virtv2.DiskDevice:
			vmd, err := helper.FetchObject(ctx, types.NamespacedName{
				Name:      bd.VirtualMachineDisk.Name,
				Namespace: state.VM.Name().Namespace,
			}, state.Client, &virtv2.VirtualMachineDisk{})
			if err != nil {
				return fmt.Errorf("unable to get VMD %q: %w", bd.VirtualMachineDisk.Name, err)
			}
			if vmd == nil {
				continue
			}
			if vmdByName == nil {
				vmdByName = make(map[string]*virtv2.VirtualMachineDisk)
			}
			vmdByName[bd.VirtualMachineDisk.Name] = vmd

		default:
			panic(fmt.Sprintf("unknown block device type %q", bd.Type))
		}
	}

	state.VMDByName = vmdByName
	state.VMIByName = vmiByName
	state.CVMIByName = cvmiByName
	state.ChangeID = state.VM.Current().Status.ChangeID
	state.StatusMessage = state.VM.Current().Status.Message

	return nil
}

func (state *VMReconcilerState) ShouldReconcile(_ logr.Logger) bool {
	return !state.VM.IsEmpty()
}

func (state *VMReconcilerState) SetStatusMessage(msg string) {
	state.StatusMessage = msg
}

func (state *VMReconcilerState) FindAttachedBlockDevice(spec virtv2.BlockDeviceSpec) *virtv2.BlockDeviceStatus {
	for i := range state.VM.Current().Status.BlockDevicesAttached {
		bda := &state.VM.Current().Status.BlockDevicesAttached[i]
		switch spec.Type {
		case virtv2.DiskDevice:
			if bda.Type == spec.Type && bda.VirtualMachineDisk.Name == spec.VirtualMachineDisk.Name {
				return bda
			}

		case virtv2.ImageDevice:
			if bda.Type == spec.Type && bda.VirtualMachineImage.Name == spec.VirtualMachineImage.Name {
				return bda
			}

		case virtv2.ClusterImageDevice:
			if bda.Type == spec.Type && bda.ClusterVirtualMachineImage.Name == spec.ClusterVirtualMachineImage.Name {
				return bda
			}

		default:
			panic(fmt.Sprintf("unexpected block device type %q", spec.Type))
		}
	}

	return nil
}

func (state *VMReconcilerState) CreateAttachedBlockDevice(spec virtv2.BlockDeviceSpec) *virtv2.BlockDeviceStatus {
	switch spec.Type {
	case virtv2.ImageDevice:
		vs := state.FindVolumeStatus(spec.VirtualMachineImage.Name)
		if vs == nil {
			return nil
		}

		vmi, hasVMI := state.VMIByName[spec.VirtualMachineImage.Name]
		if !hasVMI {
			return nil
		}
		return &virtv2.BlockDeviceStatus{
			Type:                virtv2.ImageDevice,
			VirtualMachineImage: util.CopyByPointer(spec.VirtualMachineImage),
			Target:              vs.Target,
			Size:                vmi.Status.Capacity,
		}

	case virtv2.DiskDevice:
		vs := state.FindVolumeStatus(spec.VirtualMachineDisk.Name)
		if vs == nil {
			return nil
		}

		vmd, hasVmd := state.VMDByName[spec.VirtualMachineDisk.Name]
		if !hasVmd {
			return nil
		}
		return &virtv2.BlockDeviceStatus{
			Type:               virtv2.DiskDevice,
			VirtualMachineDisk: util.CopyByPointer(spec.VirtualMachineDisk),
			Target:             vs.Target,
			Size:               vmd.Status.Capacity,
		}

	case virtv2.ClusterImageDevice:
		vs := state.FindVolumeStatus(spec.ClusterVirtualMachineImage.Name)
		if vs == nil {
			return nil
		}

		cvmi, hasCvmi := state.CVMIByName[spec.ClusterVirtualMachineImage.Name]
		if !hasCvmi {
			return nil
		}
		return &virtv2.BlockDeviceStatus{
			Type:                       virtv2.ClusterImageDevice,
			ClusterVirtualMachineImage: util.CopyByPointer(spec.ClusterVirtualMachineImage),
			Target:                     vs.Target,
			Size:                       cvmi.Status.Size.Unpacked,
		}

	default:
		panic(fmt.Sprintf("unexpected block device type %q", spec.Type))
	}
}

func (state *VMReconcilerState) FindVolumeStatus(volumeName string) *virtv1.VolumeStatus {
	for i := range state.KVVMI.Status.VolumeStatus {
		vs := state.KVVMI.Status.VolumeStatus[i]
		if vs.Name == volumeName {
			return &vs
		}
	}
	return nil
}

func (state *VMReconcilerState) SetVMLabelsWithAttachedBlockDevices() bool {
	// Exclude attach related labels.
	newLabels := RemoveAttachRelatedLabels(state.VM.Current().Labels)

	// Regenerate attach related labels.
	for _, bd := range state.VM.Current().Spec.BlockDevices {
		switch bd.Type {
		case virtv2.ImageDevice:
			vmiAttachedLabel := vmattachee.MakeAttachedResourceLabelKeyFormat("vmi", bd.VirtualMachineImage.Name)
			newLabels[vmiAttachedLabel] = vmattachee.AttachedLabelValue
		case virtv2.ClusterImageDevice:
			cvmiAttachedLabel := vmattachee.MakeAttachedResourceLabelKeyFormat("cvmi", bd.ClusterVirtualMachineImage.Name)
			newLabels[cvmiAttachedLabel] = vmattachee.AttachedLabelValue
		case virtv2.DiskDevice:
			vmdAttachedLabel := vmattachee.MakeAttachedResourceLabelKeyFormat("vmd", bd.VirtualMachineDisk.Name)
			newLabels[vmdAttachedLabel] = vmattachee.AttachedLabelValue
		default:
			panic(fmt.Sprintf("unknown block device type %q", bd.Type))
		}
	}

	if !reflect.DeepEqual(state.VM.Current().Labels, newLabels) {
		state.VM.Changed().SetLabels(newLabels)
		return true
	}
	return false
}

// SetFinalizersOnBlockDevices sets protection finalizers on CVMI and VMD attached to the VM.
func (state *VMReconcilerState) SetFinalizersOnBlockDevices(ctx context.Context) error {
	for _, bd := range state.VM.Current().Spec.BlockDevices {
		switch bd.Type {
		case virtv2.ImageDevice:
			if vmi, hasKey := state.VMIByName[bd.VirtualMachineImage.Name]; hasKey {
				if controllerutil.AddFinalizer(vmi, virtv2.FinalizerVMIProtection) {
					if err := state.Client.Update(ctx, vmi); err != nil {
						return fmt.Errorf("error setting finalizer on a VMI %q: %w", vmi.Name, err)
					}
				}
			}
		case virtv2.ClusterImageDevice:
			if cvmi, hasKey := state.CVMIByName[bd.ClusterVirtualMachineImage.Name]; hasKey {
				if controllerutil.AddFinalizer(cvmi, virtv2.FinalizerCVMIProtection) {
					if err := state.Client.Update(ctx, cvmi); err != nil {
						return fmt.Errorf("error setting finalizer on a CVMI %q: %w", cvmi.Name, err)
					}
				}
			}
		case virtv2.DiskDevice:
			if vmd, hasKey := state.VMDByName[bd.VirtualMachineDisk.Name]; hasKey {
				if controllerutil.AddFinalizer(vmd, virtv2.FinalizerVMDProtection) {
					if err := state.Client.Update(ctx, vmd); err != nil {
						return fmt.Errorf("error setting finalizer on a VMD %q: %w", vmd.Name, err)
					}
				}
			}
		default:
			panic(fmt.Sprintf("unknown block device type %q", bd.Type))
		}
	}

	return nil
}

// BlockDevicesReady check if all attached images and disks are ready to use by the VM.
func (state *VMReconcilerState) BlockDevicesReady() bool {
	for _, bd := range state.VM.Current().Spec.BlockDevices {
		switch bd.Type {
		case virtv2.ImageDevice:
			if vmi, hasKey := state.VMIByName[bd.VirtualMachineImage.Name]; hasKey {
				if vmi.Status.Phase != virtv2.ImageReady {
					return false
				}
			} else {
				return false
			}

		case virtv2.ClusterImageDevice:
			if cvmi, hasKey := state.CVMIByName[bd.ClusterVirtualMachineImage.Name]; hasKey {
				if cvmi.Status.Phase != virtv2.ImageReady {
					return false
				}
			} else {
				return false
			}

		case virtv2.DiskDevice:
			if vmd, hasKey := state.VMDByName[bd.VirtualMachineDisk.Name]; hasKey {
				if vmd.Status.Phase != virtv2.DiskReady {
					return false
				}
			} else {
				return false
			}

		default:
			panic(fmt.Sprintf("unknown block device type %q", bd.Type))
		}
	}

	return true
}

func (state *VMReconcilerState) GetKVVMErrors() (res []error) {
	if state.KVVM.Status.PrintableStatus == virtv1.VirtualMachineStatusUnschedulable {
		res = append(res, fmt.Errorf("%s", virtv1.VirtualMachineStatusUnschedulable))
	}
	return
}

func (state *VMReconcilerState) isDeletion() bool {
	return !state.VM.Current().ObjectMeta.DeletionTimestamp.IsZero()
}

// RemoveAttachRelatedLabels filters out attach related labels from the input map.
// E.g. virtualization.deckhouse.io/cvmi.ubuntu-iso.attached
func RemoveAttachRelatedLabels(labels map[string]string) map[string]string {
	result := make(map[string]string)

	// Copy labels into result map excluding attach related labels.
	for k, v := range labels {
		if _, isCvmi := vmattachee.ExtractAttachedResourceName("cvmi", k); isCvmi {
			continue
		}
		if _, isVmi := vmattachee.ExtractAttachedResourceName("vmi", k); isVmi {
			continue
		}
		if _, isVmd := vmattachee.ExtractAttachedResourceName("vmd", k); isVmd {
			continue
		}
		result[k] = v
	}
	return result
}

// RemoveNonPropagatableAnnotations removes well known annotations that are dangerous to propagate.
func RemoveNonPropagatableAnnotations(anno map[string]string) map[string]string {
	res := make(map[string]string)

	for k, v := range anno {
		if k == common.LastPropagatedVMAnnotationsAnnotation || k == common.LastPropagatedVMLabelsAnnotation {
			continue
		}

		if strings.HasPrefix(k, "kubectl.kubernetes.io") {
			continue
		}

		res[k] = v
	}

	return res
}

// PropagateVMMetadata merges labels and annotations from the input VM into destination object.
// Attach related labels and some dangerous annotations are not copied.
// Return true if destination object was changed.
func PropagateVMMetadata(vm *virtv2.VirtualMachine, destObj client.Object) (bool, error) {
	// No changes if dest is nil.
	if destObj == nil {
		return false, nil
	}

	// 1. Propagate labels.
	lastPropagatedLabels, err := GetLastPropagatedLabels(vm)
	if err != nil {
		return false, err
	}

	// Attach related labels are not needed on kubevirt resources.
	curLabels := RemoveAttachRelatedLabels(vm.GetLabels())

	newLabels, labelsChanged := merger.ApplyMapChanges(destObj.GetLabels(), lastPropagatedLabels, curLabels)
	if labelsChanged {
		destObj.SetLabels(newLabels)
	}

	// 1. Propagate annotations.
	lastPropagatedAnno, err := GetLastPropagatedAnnotations(vm)
	if err != nil {
		return false, err
	}

	// Remove dangerous annotations.
	curAnno := RemoveNonPropagatableAnnotations(vm.GetAnnotations())

	newAnno, annoChanged := merger.ApplyMapChanges(destObj.GetAnnotations(), lastPropagatedAnno, curAnno)
	if annoChanged {
		destObj.SetAnnotations(newAnno)
	}

	return labelsChanged || annoChanged, nil
}

func GetLastPropagatedLabels(vm *virtv2.VirtualMachine) (map[string]string, error) {
	var lastPropagatedLabels map[string]string

	if vm.Annotations[common.LastPropagatedVMLabelsAnnotation] != "" {
		err := json.Unmarshal([]byte(vm.Annotations[common.LastPropagatedVMLabelsAnnotation]), &lastPropagatedLabels)
		if err != nil {
			return nil, err
		}
	}

	return lastPropagatedLabels, nil
}

func SetLastPropagatedLabels(vm *virtv2.VirtualMachine) error {
	data, err := json.Marshal(RemoveAttachRelatedLabels(vm.GetLabels()))
	if err != nil {
		return err
	}

	anno := vm.GetAnnotations()
	if anno == nil {
		anno = make(map[string]string)
	}
	anno[common.LastPropagatedVMLabelsAnnotation] = string(data)
	vm.SetAnnotations(anno)

	return nil
}

func GetLastPropagatedAnnotations(vm *virtv2.VirtualMachine) (map[string]string, error) {
	var lastPropagatedAnno map[string]string

	if vm.Annotations[common.LastPropagatedVMAnnotationsAnnotation] != "" {
		err := json.Unmarshal([]byte(vm.Annotations[common.LastPropagatedVMAnnotationsAnnotation]), &lastPropagatedAnno)
		if err != nil {
			return nil, err
		}
	}

	return lastPropagatedAnno, nil
}

func SetLastPropagatedAnnotations(vm *virtv2.VirtualMachine) error {
	data, err := json.Marshal(RemoveNonPropagatableAnnotations(vm.GetAnnotations()))
	if err != nil {
		return err
	}

	anno := vm.GetAnnotations()
	if anno == nil {
		anno = make(map[string]string)
	}
	anno[common.LastPropagatedVMAnnotationsAnnotation] = string(data)
	vm.SetAnnotations(anno)

	return nil
}
