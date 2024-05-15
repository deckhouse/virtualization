package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	merger "github.com/deckhouse/virtualization-controller/pkg/common"
	kvvmutil "github.com/deckhouse/virtualization-controller/pkg/common/kvvm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/powerstate"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmchange"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMReconcilerState struct {
	Client     client.Client
	VM         *helper.Resource[*virtv2.VirtualMachine, virtv2.VirtualMachineStatus]
	KVVM       *virtv1.VirtualMachine
	KVVMI      *virtv1.VirtualMachineInstance
	KVPods     *corev1.PodList
	VMPod      *corev1.Pod
	VMDByName  map[string]*virtv2.VirtualDisk
	VMIByName  map[string]*virtv2.VirtualImage
	CVMIByName map[string]*virtv2.ClusterVirtualImage

	IPAddressClaim *virtv2.VirtualMachineIPAddressClaim
	CPUModel       *virtv2.VirtualMachineCPUModel

	VMPodCompleted   bool
	VMShutdownReason string

	Result                 *reconcile.Result
	StatusMessage          string
	RestartAwaitingChanges []apiextensionsv1.JSON
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

func (state *VMReconcilerState) Reload(ctx context.Context, req reconcile.Request, log logr.Logger, _ client.Client) error {
	err := state.VM.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}

	if state.VM.IsEmpty() {
		log.Info("Reconcile observe an absent VM: it may be deleted", "VM", req.NamespacedName)
		return nil
	}

	claimName := state.VM.Current().Spec.VirtualMachineIPAddressClaim
	if claimName == "" {
		claimName = state.VM.Name().Name
	}

	claimKey := types.NamespacedName{Name: claimName, Namespace: state.VM.Name().Namespace}
	state.IPAddressClaim, err = helper.FetchObject(ctx, claimKey, state.Client, &virtv2.VirtualMachineIPAddressClaim{})
	if err != nil {
		return fmt.Errorf("unable to get Claim %s: %w", claimKey, err)
	}

	vmcpuKey := types.NamespacedName{Name: state.VM.Current().Spec.CPU.VirtualMachineCPUModel}
	state.CPUModel, err = helper.FetchObject(ctx, vmcpuKey, state.Client, &virtv2.VirtualMachineCPUModel{})
	if err != nil {
		return fmt.Errorf("unable to get cpu model %s: %w", claimKey, err)
	}

	kvvmName := state.VM.Name()
	kvvm, err := helper.FetchObject(ctx, kvvmName, state.Client, &virtv1.VirtualMachine{})
	if err != nil {
		return fmt.Errorf("unable to get KubeVirt VM %q: %w", kvvmName, err)
	}
	state.KVVM = kvvm

	if state.KVVM != nil && state.vmIsCreated() {
		// FIXME(VM): ObservedGeneration & DesiredGeneration only available since KubeVirt 1.0.0 which is only prereleased at the moment
		// FIXME(VM): Uncomment following check when KubeVirt updated to 1.0.0
		kvvmi, err := helper.FetchObject(ctx, kvvmName, state.Client, &virtv1.VirtualMachineInstance{})
		if err != nil {
			return fmt.Errorf("unable to get KubeVirt VMI %q: %w", kvvmName, err)
		}
		state.KVVMI = kvvmi
	}

	// Search for virt-launcher Pods if KubeVirt VMI exists for VM.
	if state.KVVMI != nil {
		podList := new(corev1.PodList)
		selector := labels.SelectorFromSet(map[string]string{"vm.kubevirt.io/name": state.KVVM.GetName()})
		err = state.Client.List(ctx, podList, &client.ListOptions{
			LabelSelector: selector,
			Namespace:     kvvm.Namespace,
		})
		if err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("unable to list virt-launcher Pod for KubeVirt VM %q: %w", kvvmName, err)
		}
		if len(podList.Items) > 0 {
			state.KVPods = podList
			// Find Pod with actual VM.
			state.VMPod = kvvmutil.GetVMPod(state.KVVMI, podList)
		}
	}

	// Get shutdown reason if VM is completed.
	state.VMPodCompleted, state.VMShutdownReason = powerstate.ShutdownReason(state.KVVMI, state.KVPods)

	var vmdByName map[string]*virtv2.VirtualDisk
	var vmiByName map[string]*virtv2.VirtualImage
	var cvmiByName map[string]*virtv2.ClusterVirtualImage

	for _, bd := range state.VM.Current().Spec.BlockDeviceRefs {
		switch bd.Kind {
		case virtv2.ImageDevice:
			vmi, err := helper.FetchObject(ctx, types.NamespacedName{
				Name:      bd.Name,
				Namespace: state.VM.Name().Namespace,
			}, state.Client, &virtv2.VirtualImage{})
			if err != nil {
				return fmt.Errorf("unable to get VI %q: %w", bd.Name, err)
			}
			if vmi == nil {
				continue
			}
			if vmiByName == nil {
				vmiByName = make(map[string]*virtv2.VirtualImage)
			}
			vmiByName[bd.Name] = vmi

		case virtv2.ClusterImageDevice:
			cvmi, err := helper.FetchObject(ctx, types.NamespacedName{
				Name: bd.Name,
			}, state.Client, &virtv2.ClusterVirtualImage{})
			if err != nil {
				return fmt.Errorf("unable to get CVI %q: %w", bd.Name, err)
			}
			if cvmi == nil {
				continue
			}
			if cvmiByName == nil {
				cvmiByName = make(map[string]*virtv2.ClusterVirtualImage)
			}
			cvmiByName[bd.Name] = cvmi

		case virtv2.DiskDevice:
			vmd, err := helper.FetchObject(ctx, types.NamespacedName{
				Name:      bd.Name,
				Namespace: state.VM.Name().Namespace,
			}, state.Client, &virtv2.VirtualDisk{})
			if err != nil {
				return fmt.Errorf("unable to get virtual disk %q: %w", bd.Name, err)
			}
			if vmd == nil {
				continue
			}
			if vmdByName == nil {
				vmdByName = make(map[string]*virtv2.VirtualDisk)
			}
			vmdByName[bd.Name] = vmd

		default:
			return fmt.Errorf("unexpected block device kind %q. %w", bd.Kind, common.ErrUnknownType)
		}
	}

	state.VMDByName = vmdByName
	state.VMIByName = vmiByName
	state.CVMIByName = cvmiByName
	//state.StatusMessage = state.VM.Current().Status.Message
	state.RestartAwaitingChanges = state.VM.Current().Status.RestartAwaitingChanges

	return nil
}

func (state *VMReconcilerState) ShouldReconcile(_ logr.Logger) bool {
	return !state.VM.IsEmpty()
}

func (state *VMReconcilerState) SetStatusMessage(msg string) {
	state.StatusMessage = msg
}

func (state *VMReconcilerState) SetChangesInfo(changes *vmchange.SpecChanges) error {
	statusChanges, err := changes.ConvertPendingChanges()
	if err != nil {
		return fmt.Errorf("convert pending changes for status: %w", err)
	}
	state.RestartAwaitingChanges = statusChanges

	statusMessage := ""
	if changes.ActionType() == vmchange.ActionRestart {
		statusMessage = "VM restart required to apply changes."
	}
	state.StatusMessage = statusMessage
	return nil
}

func (state *VMReconcilerState) ResetChangesInfo() {
	state.RestartAwaitingChanges = nil
	state.StatusMessage = ""
}

func (state *VMReconcilerState) FindAttachedBlockDevice(spec virtv2.BlockDeviceSpecRef) *virtv2.BlockDeviceStatusRef {
	for i := range state.VM.Current().Status.BlockDeviceRefs {
		bda := &state.VM.Current().Status.BlockDeviceRefs[i]
		if bda.Kind == spec.Kind && bda.Name == spec.Name {
			return bda
		}
	}

	return nil
}

func (state *VMReconcilerState) CreateAttachedBlockDevice(spec virtv2.BlockDeviceSpecRef) *virtv2.BlockDeviceStatusRef {
	switch spec.Kind {
	case virtv2.ImageDevice:
		vs := state.FindVolumeStatus(kvbuilder.GenerateVMIDiskName(spec.Name))
		if vs == nil {
			return nil
		}

		vmi, hasVMI := state.VMIByName[spec.Name]
		if !hasVMI {
			return nil
		}

		return &virtv2.BlockDeviceStatusRef{
			Kind:   virtv2.ImageDevice,
			Name:   spec.Name,
			Target: vs.Target,
			Size:   vmi.Status.Capacity,
		}

	case virtv2.DiskDevice:
		vs := state.FindVolumeStatus(kvbuilder.GenerateVMDDiskName(spec.Name))
		if vs == nil {
			return nil
		}

		vmd, hasVmd := state.VMDByName[spec.Name]
		if !hasVmd {
			return nil
		}

		return &virtv2.BlockDeviceStatusRef{
			Kind:   virtv2.DiskDevice,
			Name:   spec.Name,
			Target: vs.Target,
			Size:   vmd.Status.Capacity,
		}

	case virtv2.ClusterImageDevice:
		vs := state.FindVolumeStatus(kvbuilder.GenerateCVMIDiskName(spec.Name))
		if vs == nil {
			return nil
		}

		cvmi, hasCvmi := state.CVMIByName[spec.Name]
		if !hasCvmi {
			return nil
		}

		return &virtv2.BlockDeviceStatusRef{
			Kind:   virtv2.ClusterImageDevice,
			Name:   spec.Name,
			Target: vs.Target,
			Size:   cvmi.Status.Size.Unpacked,
		}
	}
	return nil
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

// SetFinalizersOnBlockDevices sets protection finalizers on CVMI and VMD attached to the VM.
func (state *VMReconcilerState) SetFinalizersOnBlockDevices(ctx context.Context) error {
	for _, bd := range state.VM.Current().Spec.BlockDeviceRefs {
		switch bd.Kind {
		case virtv2.ImageDevice:
			if vmi, hasKey := state.VMIByName[bd.Name]; hasKey {
				if controllerutil.AddFinalizer(vmi, virtv2.FinalizerVMIProtection) {
					if err := state.Client.Update(ctx, vmi); err != nil {
						return fmt.Errorf("error setting finalizer on a VI %q: %w", vmi.Name, err)
					}
				}
			}
		case virtv2.ClusterImageDevice:
			if cvmi, hasKey := state.CVMIByName[bd.Name]; hasKey {
				if controllerutil.AddFinalizer(cvmi, virtv2.FinalizerCVMIProtection) {
					if err := state.Client.Update(ctx, cvmi); err != nil {
						return fmt.Errorf("error setting finalizer on a CVI %q: %w", cvmi.Name, err)
					}
				}
			}
		case virtv2.DiskDevice:
			if vmd, hasKey := state.VMDByName[bd.Name]; hasKey {
				if controllerutil.AddFinalizer(vmd, virtv2.FinalizerVMDProtection) {
					if err := state.Client.Update(ctx, vmd); err != nil {
						return fmt.Errorf("error setting finalizer on a virtual disk %q: %w", vmd.Name, err)
					}
				}
			}
		default:
			return fmt.Errorf("unexpected block device kind %q. %w", bd.Kind, common.ErrUnknownType)
		}
	}

	return nil
}

// BlockDevicesReady check if all attached images and disks are ready to use by the VM.
func (state *VMReconcilerState) BlockDevicesReady() bool {
	for _, bd := range state.VM.Current().Spec.BlockDeviceRefs {
		switch bd.Kind {
		case virtv2.ImageDevice:
			if vmi, hasKey := state.VMIByName[bd.Name]; hasKey {
				if vmi.Status.Phase != virtv2.ImageReady {
					return false
				}
			} else {
				return false
			}

		case virtv2.ClusterImageDevice:
			if cvmi, hasKey := state.CVMIByName[bd.Name]; hasKey {
				if cvmi.Status.Phase != virtv2.ImageReady {
					return false
				}
			} else {
				return false
			}

		case virtv2.DiskDevice:
			if vmd, hasKey := state.VMDByName[bd.Name]; hasKey {
				if vmd.Status.Phase != virtv2.DiskReady {
					return false
				}
			} else {
				return false
			}
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

func (state *VMReconcilerState) EnsureRunStrategy(ctx context.Context, desiredRunStrategy virtv1.VirtualMachineRunStrategy) error {
	kvvmRunStrategy := kvvmutil.GetRunStrategy(state.KVVM)

	if kvvmRunStrategy == desiredRunStrategy {
		return nil
	}
	patch := kvvmutil.PatchRunStrategy(desiredRunStrategy)
	err := state.Client.Patch(ctx, state.KVVM, patch)
	if err != nil {
		return fmt.Errorf("patch KVVM with runStrategy %s: %w", desiredRunStrategy, err)
	}

	return nil
}

func (state *VMReconcilerState) isDeletion() bool {
	return !state.VM.Current().ObjectMeta.DeletionTimestamp.IsZero()
}

func (state *VMReconcilerState) vmIsCreated() bool {
	return state.KVVM != nil && state.KVVM.Status.Created
}

func (state *VMReconcilerState) vmIsStopped() bool {
	return state.KVVM != nil && state.KVVM.Status.PrintableStatus == virtv1.VirtualMachineStatusStopped
}

func (state *VMReconcilerState) vmIsStopping() bool {
	return state.KVVM != nil && state.KVVM.Status.PrintableStatus == virtv1.VirtualMachineStatusStopping
}

func (state *VMReconcilerState) vmIsScheduling() bool {
	return state.KVVM != nil && state.KVVM.Status.PrintableStatus == virtv1.VirtualMachineStatusProvisioning
}

func (state *VMReconcilerState) vmIsStarting() bool {
	return state.KVVM != nil && state.KVVM.Status.PrintableStatus == virtv1.VirtualMachineStatusStarting
}

func (state *VMReconcilerState) vmIsRunning() bool {
	return state.KVVMI != nil && state.KVVMI.Status.Phase == virtv1.Running
}

func (state *VMReconcilerState) vmIsMigrating() bool {
	return state.KVVM != nil && state.KVVM.Status.PrintableStatus == virtv1.VirtualMachineStatusMigrating
}

func (state *VMReconcilerState) vmIsPaused() bool {
	return state.KVVM != nil && state.KVVM.Status.PrintableStatus == virtv1.VirtualMachineStatusPaused
}

func (state *VMReconcilerState) vmIsFailed() bool {
	return state.KVVM != nil &&
		(state.KVVM.Status.PrintableStatus == virtv1.VirtualMachineStatusCrashLoopBackOff ||
			state.KVVM.Status.PrintableStatus == virtv1.VirtualMachineStatusUnschedulable ||
			state.KVVM.Status.PrintableStatus == virtv1.VirtualMachineStatusErrImagePull ||
			state.KVVM.Status.PrintableStatus == virtv1.VirtualMachineStatusImagePullBackOff ||
			state.KVVM.Status.PrintableStatus == virtv1.VirtualMachineStatusPvcNotFound ||
			state.KVVM.Status.PrintableStatus == virtv1.VirtualMachineStatusDataVolumeError)
}

func (state *VMReconcilerState) vmIsPending() bool {
	return state.KVVM != nil &&
		(state.KVVM.Status.PrintableStatus == "" || state.KVVM.Status.PrintableStatus == virtv1.VirtualMachineStatusWaitingForVolumeBinding)
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
func PropagateVMMetadata(vm *virtv2.VirtualMachine, kvvm *virtv1.VirtualMachine, destObj client.Object) (bool, error) {
	// No changes if dest is nil.
	if destObj == nil {
		return false, nil
	}

	// 1. Propagate labels.
	lastPropagatedLabels, err := GetLastPropagatedLabels(kvvm)
	if err != nil {
		return false, err
	}

	newLabels, labelsChanged := merger.ApplyMapChanges(destObj.GetLabels(), lastPropagatedLabels, vm.GetLabels())
	if labelsChanged {
		destObj.SetLabels(newLabels)
	}

	// 1. Propagate annotations.
	lastPropagatedAnno, err := GetLastPropagatedAnnotations(kvvm)
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

func GetLastPropagatedLabels(kvvm *virtv1.VirtualMachine) (map[string]string, error) {
	var lastPropagatedLabels map[string]string

	if kvvm.Annotations[common.LastPropagatedVMLabelsAnnotation] != "" {
		err := json.Unmarshal([]byte(kvvm.Annotations[common.LastPropagatedVMLabelsAnnotation]), &lastPropagatedLabels)
		if err != nil {
			return nil, err
		}
	}

	return lastPropagatedLabels, nil
}

func SetLastPropagatedLabels(kvvm *virtv1.VirtualMachine, vm *virtv2.VirtualMachine) (bool, error) {
	data, err := json.Marshal(vm.GetLabels())
	if err != nil {
		return false, err
	}

	newAnnoValue := string(data)

	if kvvm.Annotations[common.LastPropagatedVMLabelsAnnotation] == newAnnoValue {
		return false, nil
	}

	common.AddAnnotation(kvvm, common.LastPropagatedVMLabelsAnnotation, newAnnoValue)
	return true, nil
}

func GetLastPropagatedAnnotations(kvvm *virtv1.VirtualMachine) (map[string]string, error) {
	var lastPropagatedAnno map[string]string

	if kvvm.Annotations[common.LastPropagatedVMAnnotationsAnnotation] != "" {
		err := json.Unmarshal([]byte(kvvm.Annotations[common.LastPropagatedVMAnnotationsAnnotation]), &lastPropagatedAnno)
		if err != nil {
			return nil, err
		}
	}

	return lastPropagatedAnno, nil
}

func SetLastPropagatedAnnotations(kvvm *virtv1.VirtualMachine, vm *virtv2.VirtualMachine) (bool, error) {
	data, err := json.Marshal(RemoveNonPropagatableAnnotations(vm.GetAnnotations()))
	if err != nil {
		return false, err
	}

	newAnnoValue := string(data)

	if kvvm.Annotations[common.LastPropagatedVMAnnotationsAnnotation] == newAnnoValue {
		return false, nil
	}

	common.AddAnnotation(kvvm, common.LastPropagatedVMAnnotationsAnnotation, newAnnoValue)
	return true, nil
}
