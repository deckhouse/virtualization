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

package common

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common"
)

const (
	CVIShortName = "cvi"
	VDShortName  = "vd"
	VIShortName  = "vi"

	// AnnAPIGroup is the APIGroup for virtualization-controller.
	AnnAPIGroup = "virt.deckhouse.io"

	// AnnCreatedBy is a pod annotation indicating if the pod was created by the PVC
	AnnCreatedBy = AnnAPIGroup + "/storage.createdByController"
	// AnnPodPhase is a PVC annotation indicating the related pod progress (phase)
	AnnPodPhase = AnnAPIGroup + "/storage.pod.phase"
	// AnnPodReady tells whether the pod is ready
	AnnPodReady = AnnAPIGroup + "/storage.pod.ready"
	// AnnPodRestarts is a PVC annotation that tells how many times a related pod was restarted
	AnnPodRestarts = AnnAPIGroup + "/storage.pod.restarts"
	// AnnPopulatedFor is a PVC annotation telling the datavolume controller that the PVC is already populated
	AnnPopulatedFor = AnnAPIGroup + "/storage.populatedFor"
	// AnnPrePopulated is a PVC annotation telling the datavolume controller that the PVC is already populated
	AnnPrePopulated = AnnAPIGroup + "/storage.prePopulated"
	// AnnPriorityClassName is PVC annotation to indicate the priority class name for importer, cloner and uploader pod
	AnnPriorityClassName = AnnAPIGroup + "/storage.pod.priorityclassname"
	// AnnExternalPopulation annotation marks a PVC as "externally populated", allowing the import-controller to skip it
	AnnExternalPopulation = AnnAPIGroup + "/externalPopulation"

	// AnnDeleteAfterCompletion is PVC annotation for deleting DV after completion
	AnnDeleteAfterCompletion = AnnAPIGroup + "/storage.deleteAfterCompletion"
	// AnnPodRetainAfterCompletion is PVC annotation for retaining transfer pods after completion
	AnnPodRetainAfterCompletion = AnnAPIGroup + "/storage.pod.retainAfterCompletion"

	// AnnPreviousCheckpoint provides a const to indicate the previous snapshot for a multistage import
	AnnPreviousCheckpoint = AnnAPIGroup + "/storage.checkpoint.previous"
	// AnnCurrentCheckpoint provides a const to indicate the current snapshot for a multistage import
	AnnCurrentCheckpoint = AnnAPIGroup + "/storage.checkpoint.current"
	// AnnFinalCheckpoint provides a const to indicate whether the current checkpoint is the last one
	AnnFinalCheckpoint = AnnAPIGroup + "/storage.checkpoint.final"
	// AnnCheckpointsCopied is a prefix for recording which checkpoints have already been copied
	AnnCheckpointsCopied = AnnAPIGroup + "/storage.checkpoint.copied"

	// AnnCurrentPodID keeps track of the latest pod servicing this PVC
	AnnCurrentPodID = AnnAPIGroup + "/storage.checkpoint.pod.id"

	// AnnRunningCondition provides a const for the running condition
	AnnRunningCondition = AnnAPIGroup + "/storage.condition.running"
	// AnnRunningConditionMessage provides a const for the running condition
	AnnRunningConditionMessage = AnnAPIGroup + "/storage.condition.running.message"
	// AnnRunningConditionReason provides a const for the running condition
	AnnRunningConditionReason = AnnAPIGroup + "/storage.condition.running.reason"

	// AnnSourceRunningCondition provides a const for the running condition
	AnnSourceRunningCondition = AnnAPIGroup + "/storage.condition.source.running"
	// AnnSourceRunningConditionMessage provides a const for the running condition
	AnnSourceRunningConditionMessage = AnnAPIGroup + "/storage.condition.source.running.message"
	// AnnSourceRunningConditionReason provides a const for the running condition
	AnnSourceRunningConditionReason = AnnAPIGroup + "/storage.condition.source.running.reason"

	// AnnSource provide a const for our PVC import source annotation
	AnnSource = AnnAPIGroup + "/storage.import.source"
	// AnnEndpoint provides a const for our PVC endpoint annotation
	AnnEndpoint = AnnAPIGroup + "/storage.import.endpoint"

	// AnnRegistryImportMethod provides a const for registry import method annotation
	AnnRegistryImportMethod = AnnAPIGroup + "/storage.import.registryImportMethod"
	// AnnRegistryImageStream provides a const for registry image stream annotation
	AnnRegistryImageStream = AnnAPIGroup + "/storage.import.registryImageStream"
	// AnnUploadURL provides a const for CVMI/VMI/VMD uploadURL annotation
	AnnUploadURL = AnnAPIGroup + "/upload.url"
	// AnnDiskID provides a const for our PVC diskId annotation
	AnnDiskID = AnnAPIGroup + "/storage.import.diskId"
	// AnnUUID provides a const for our PVC uuid annotation
	AnnUUID = AnnAPIGroup + "/storage.import.uuid"
	// AnnBackingFile provides a const for our PVC backing file annotation
	AnnBackingFile = AnnAPIGroup + "/storage.import.backingFile"
	// AnnThumbprint provides a const for our PVC backing thumbprint annotation
	AnnThumbprint = AnnAPIGroup + "/storage.import.vddk.thumbprint"
	// AnnExtraHeaders provides a const for our PVC extraHeaders annotation
	AnnExtraHeaders = AnnAPIGroup + "/storage.import.extraHeaders"
	// AnnSecretExtraHeaders provides a const for our PVC secretExtraHeaders annotation
	AnnSecretExtraHeaders = AnnAPIGroup + "/storage.import.secretExtraHeaders"
	AnnCreatedByImporter  = AnnAPIGroup + "/storage.createdByImporter"

	AnnImportAvgSpeedBytes     = AnnAPIGroup + "/storage.import.speed.avg"
	AnnImportCurrentSpeedBytes = AnnAPIGroup + "/storage.import.speed.current"
	AnnImportStoredSizeBytes   = AnnAPIGroup + "/storage.import.size.stored"
	AnnImportUnpackedSizeBytes = AnnAPIGroup + "/storage.import.size.unpacked"

	// AnnCloneToken is the annotation containing the clone token
	AnnCloneToken = AnnAPIGroup + "/storage.clone.token"
	// AnnExtendedCloneToken is the annotation containing the long term clone token
	AnnExtendedCloneToken = AnnAPIGroup + "/storage.extended.clone.token"
	// AnnPermissiveClone annotation allows the clone-controller to skip the clone size validation
	AnnPermissiveClone = AnnAPIGroup + "/permissiveClone"
	// AnnOwnerUID annotation has the owner UID
	AnnOwnerUID = AnnAPIGroup + "/ownerUID"
	// AnnCloneType is the comuuted/requested clone type
	AnnCloneType = AnnAPIGroup + "/cloneType"
	// AnnCloneSourcePod name of the source clone pod
	AnnCloneSourcePod = "cdi.kubevirt.io/storage.sourceClonePodName"

	// AnnUploadRequest marks that a PVC should be made available for upload
	AnnUploadRequest = AnnAPIGroup + "/storage.upload.target"

	// AnnCheckStaticVolume checks if a statically allocated PV exists before creating the target PVC.
	// If so, PVC is still created but population is skipped
	AnnCheckStaticVolume = AnnAPIGroup + "/storage.checkStaticVolume"

	// AnnPersistentVolumeList is an annotation storing a list of PV names
	AnnPersistentVolumeList = AnnAPIGroup + "/storage.persistentVolumeList"

	// AnnPopulatorKind annotation is added to a PVC' to specify the population kind, so it's later
	// checked by the common populator watches.
	AnnPopulatorKind = AnnAPIGroup + "/storage.populator.kind"

	// AnnDefaultStorageClass is the annotation indicating that a storage class is the default one.
	AnnDefaultStorageClass = "storageclass.kubernetes.io/is-default-class"

	// AnnOpenShiftImageLookup is the annotation for OpenShift image stream lookup
	AnnOpenShiftImageLookup = "alpha.image.policy.openshift.io/resolve-names"

	// AnnCloneRequest sets our expected annotation for a CloneRequest
	AnnCloneRequest = "k8s.io/CloneRequest"
	// AnnCloneOf is used to indicate that cloning was complete
	AnnCloneOf = "k8s.io/CloneOf"

	// AnnPodNetwork is used for specifying Pod Network
	AnnPodNetwork = "k8s.v1.cni.cncf.io/networks"
	// AnnPodMultusDefaultNetwork is used for specifying default Pod Network
	AnnPodMultusDefaultNetwork = "v1.multus-cni.io/default-network"
	// AnnPodSidecarInjection is used for enabling/disabling Pod istio/AspenMesh sidecar injection
	AnnPodSidecarInjection = "sidecar.istio.io/inject"
	// AnnPodSidecarInjectionDefault is the default value passed for AnnPodSidecarInjection
	AnnPodSidecarInjectionDefault = "false"

	// AnnImmediateBinding provides a const to indicate whether immediate binding should be performed on the PV (overrides global config)
	AnnImmediateBinding = AnnAPIGroup + "/storage.bind.immediate.requested"

	AnnAPIGroupV              = "virtualization.deckhouse.io"
	AnnVirtualDisk            = "virtualdisk." + AnnAPIGroupV
	AnnVirtualDiskVolumeMode  = AnnVirtualDisk + "/volume-mode"
	AnnVirtualDiskAccessMode  = AnnVirtualDisk + "/access-mode"
	AnnVirtualDiskBindingMode = AnnVirtualDisk + "/binding-mode"

	// AnnSelectedNode annotation is added to a PVC that has been triggered by scheduler to
	// be dynamically provisioned. Its value is the name of the selected node.
	AnnSelectedNode = "volume.kubernetes.io/selected-node"

	// AnnBoundVirtualMachineName is an ip address annotation with value of bound vm name.
	AnnBoundVirtualMachineName = AnnAPIGroup + "/bound-virtual-machine-name"

	// AnnVMLastAppliedSpec is an annotation on KVVM. It contains a JSON with VM spec.
	AnnVMLastAppliedSpec = AnnAPIGroup + "/vm.last-applied-spec"

	// LastPropagatedVMAnnotationsAnnotation is a marshalled map of previously applied virtual machine annotations.
	LastPropagatedVMAnnotationsAnnotation = AnnAPIGroup + "/last-propagated-vm-annotations"
	// LastPropagatedVMLabelsAnnotation is a marshalled map of previously applied virtual machine labels.
	LastPropagatedVMLabelsAnnotation = AnnAPIGroup + "/last-propagated-vm-labels"

	// ErrStartingPod provides a const to indicate that a pod wasn't able to start without providing sensitive information (reason)
	ErrStartingPod = "ErrStartingPod"
	// MessageErrStartingPod provides a const to indicate that a pod wasn't able to start without providing sensitive information (message)
	MessageErrStartingPod = "Error starting pod '%s': For more information, request access to cdi-deploy logs from your sysadmin"
	// ErrClaimNotValid provides a const to indicate a claim is not valid
	ErrClaimNotValid = "ErrClaimNotValid"
	// ErrExceededQuota provides a const to indicate the claim has exceeded the quota
	ErrExceededQuota = "ErrExceededQuota"
	// ErrIncompatiblePVC provides a const to indicate a clone is not possible due to an incompatible PVC
	ErrIncompatiblePVC = "ErrIncompatiblePVC"

	// SourceHTTP is the source type HTTP, if unspecified or invalid, it defaults to SourceHTTP
	SourceHTTP = "http"
	// SourceS3 is the source type S3
	SourceS3 = "s3"
	// SourceGCS is the source type GCS
	SourceGCS = "gcs"
	// SourceGlance is the source type of glance
	SourceGlance = "glance"
	// SourceNone means there is no source.
	SourceNone = "none"
	// SourceRegistry is the source type of Registry
	SourceRegistry = "registry"
	// SourceImageio is the source type ovirt-imageio
	SourceImageio = "imageio"
	// SourceVDDK is the source type of VDDK
	SourceVDDK = "vddk"
	// SourceDVCR is the source type of dvcr
	SourceDVCR = "dvcr"
	// SourceBlockDevice is the source type of block device
	SourceBlockDevice = "blockDevice"
	// ClaimLost reason const
	ClaimLost = "ClaimLost"
	// NotFound reason const
	NotFound = "NotFound"

	// LabelDefaultInstancetype provides a default VirtualMachine{ClusterInstancetype,Instancetype} that can be used by a VirtualMachine booting from a given PVC
	LabelDefaultInstancetype = "instancetype.kubevirt.io/default-instancetype"
	// LabelDefaultInstancetypeKind provides a default kind of either VirtualMachineClusterInstancetype or VirtualMachineInstancetype
	LabelDefaultInstancetypeKind = "instancetype.kubevirt.io/default-instancetype-kind"
	// LabelDefaultPreference provides a default VirtualMachine{ClusterPreference,Preference} that can be used by a VirtualMachine booting from a given PVC
	LabelDefaultPreference = "instancetype.kubevirt.io/default-preference"
	// LabelDefaultPreferenceKind provides a default kind of either VirtualMachineClusterPreference or VirtualMachinePreference
	LabelDefaultPreferenceKind = "instancetype.kubevirt.io/default-preference-kind"

	// LabelsPrefix is a prefix for virtualization-controller labels.
	LabelsPrefix = "virtualization.deckhouse.io"

	// LabelVirtualMachineName is a label to link VirtualMachineOperation to VirtualMachine.
	LabelVirtualMachineName = LabelsPrefix + "/virtual-machine-name"

	UploaderServiceLabel = "service"
	// ProgressDone this means we are DONE
	ProgressDone = "100.0%"
	// UploaderWaitDuration is upload waiting timeout
	UploaderWaitDuration = 15 * time.Minute
)

var (
	apiServerKeyOnce sync.Once
	apiServerKey     *rsa.PrivateKey

	// ErrUnknownValue is a variable of type `error` that represents an error message indicating an unknown value.
	ErrUnknownValue = errors.New("unknown value")
	// ErrUnknownType is a variable of type `error` that represents an error message indicating an unknown type.
	ErrUnknownType = errors.New("unknown type")
)

// GetPriorityClass gets PVC priority class
func GetPriorityClass(pvc *corev1.PersistentVolumeClaim) string {
	anno := pvc.GetAnnotations()
	return anno[AnnPriorityClassName]
}

// ShouldCleanupSubResources returns whether sub resources should be deleted:
// - CVMI, VMI has no annotation to retain pod after import
// - CVMI, VMI is deleted
func ShouldCleanupSubResources(obj metav1.Object) bool {
	return obj.GetAnnotations()[AnnPodRetainAfterCompletion] != "true" || obj.GetDeletionTimestamp() != nil
}

func ShouldDeletePod(obj metav1.Object) bool {
	if len(obj.GetAnnotations()) == 0 {
		return false
	}
	return ShouldCleanupSubResources(obj)
}

// AddFinalizer adds a finalizer to a resource
func AddFinalizer(obj metav1.Object, name string) {
	if HasFinalizer(obj, name) {
		return
	}

	obj.SetFinalizers(append(obj.GetFinalizers(), name))
}

// RemoveFinalizer removes a finalizer from a resource
func RemoveFinalizer(obj metav1.Object, name string) {
	if !HasFinalizer(obj, name) {
		return
	}

	var finalizers []string
	for _, f := range obj.GetFinalizers() {
		if f != name {
			finalizers = append(finalizers, f)
		}
	}

	obj.SetFinalizers(finalizers)
}

// HasFinalizer returns true if a resource has a specific finalizer
func HasFinalizer(object metav1.Object, value string) bool {
	for _, f := range object.GetFinalizers() {
		if f == value {
			return true
		}
	}
	return false
}

// AddAnnotation adds an annotation to an object
func AddAnnotation(obj metav1.Object, key, value string) {
	if obj.GetAnnotations() == nil {
		obj.SetAnnotations(make(map[string]string))
	}
	obj.GetAnnotations()[key] = value
}

// AddLabel adds a label to an object
func AddLabel(obj metav1.Object, key, value string) {
	if obj.GetLabels() == nil {
		obj.SetLabels(make(map[string]string))
	}
	obj.GetLabels()[key] = value
}

// PublishPodErr handles pod-creation errors and updates the CVMI without providing sensitive information.
// TODO make work with VirtualImage object.
func PublishPodErr(err error, podName string, obj client.Object, recorder record.EventRecorder, apiClient client.Client) error {
	// Generic reason and msg to avoid providing sensitive information
	reason := ErrStartingPod
	msg := fmt.Sprintf(MessageErrStartingPod, podName) + ": " + err.Error()

	// Error handling to fine-tune the event with pertinent info
	if ErrQuotaExceeded(err) {
		reason = ErrExceededQuota
	}

	recorder.Event(obj, corev1.EventTypeWarning, reason, msg)

	if isCloneSourcePod := CreateCloneSourcePodName(obj) == podName; isCloneSourcePod {
		AddAnnotation(obj, AnnSourceRunningCondition, "false")
		AddAnnotation(obj, AnnSourceRunningConditionReason, reason)
		AddAnnotation(obj, AnnSourceRunningConditionMessage, msg)
	} else {
		AddAnnotation(obj, AnnRunningCondition, "false")
		AddAnnotation(obj, AnnRunningConditionReason, reason)
		AddAnnotation(obj, AnnRunningConditionMessage, msg)
	}

	AddAnnotation(obj, AnnPodPhase, string(corev1.PodFailed))
	return apiClient.Update(context.TODO(), obj)
}

type UIDable interface {
	GetUID() types.UID
}

// CreateCloneSourcePodName creates clone source pod name
func CreateCloneSourcePodName(obj UIDable) string {
	return string(obj.GetUID()) + common.ClonerSourcePodNameSuffix
}

// IsPodRunning returns true if a Pod is in 'Running' phase, false if not.
func IsPodRunning(pod *corev1.Pod) bool {
	return pod != nil && pod.Status.Phase == corev1.PodRunning
}

// IsPodStarted returns true if a Pod is in started state, false if not.
func IsPodStarted(pod *corev1.Pod) bool {
	if pod == nil || pod.Status.StartTime == nil {
		return false
	}

	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Started == nil || !*cs.Started {
			return false
		}
	}

	return true
}

// IsPodComplete returns true if a Pod is in 'Succeeded' phase, false if not.
func IsPodComplete(pod *corev1.Pod) bool {
	return pod != nil && pod.Status.Phase == corev1.PodSucceeded
}

// IsDataVolumeComplete returns true if a DataVolume is in 'Succeeded' phase, false if not.
func IsDataVolumeComplete(dv *cdiv1.DataVolume) bool {
	return dv != nil && dv.Status.Phase == cdiv1.Succeeded
}

// IsPVCBound returns true if a PersistentVolumeClaim is in 'Bound' phase, false if not.
func IsPVCBound(pvc *corev1.PersistentVolumeClaim) bool {
	return pvc != nil && pvc.Status.Phase == corev1.ClaimBound
}

func IsTerminating(obj client.Object) bool {
	return !reflect.ValueOf(obj).IsNil() && obj.GetDeletionTimestamp() != nil
}

func AnyTerminating(objs ...client.Object) bool {
	for _, obj := range objs {
		if IsTerminating(obj) {
			return true
		}
	}

	return false
}

// SetRestrictedSecurityContext sets the pod security params to be compatible with restricted PSA
func SetRestrictedSecurityContext(podSpec *corev1.PodSpec) {
	hasVolumeMounts := false
	for _, containers := range [][]corev1.Container{podSpec.InitContainers, podSpec.Containers} {
		for i := range containers {
			container := &containers[i]
			if container.SecurityContext == nil {
				container.SecurityContext = &corev1.SecurityContext{}
			}
			container.SecurityContext.Capabilities = &corev1.Capabilities{
				Drop: []corev1.Capability{
					"ALL",
				},
			}
			container.SecurityContext.SeccompProfile = &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			}
			container.SecurityContext.AllowPrivilegeEscalation = ptr.To(false)
			container.SecurityContext.RunAsNonRoot = ptr.To(true)
			container.SecurityContext.RunAsUser = ptr.To(common.QemuSubGid)
			if len(container.VolumeMounts) > 0 {
				hasVolumeMounts = true
			}
		}
	}

	if hasVolumeMounts {
		if podSpec.SecurityContext == nil {
			podSpec.SecurityContext = &corev1.PodSecurityContext{}
		}
		podSpec.SecurityContext.FSGroup = ptr.To(common.QemuSubGid)
	}
}

// SetNodeNameIfPopulator sets NodeName in a pod spec when the PVC is being handled by a CDI volume populator
func SetNodeNameIfPopulator(pvc *corev1.PersistentVolumeClaim, podSpec *corev1.PodSpec) {
	_, isPopulator := pvc.Annotations[AnnPopulatorKind]
	nodeName := pvc.Annotations[AnnSelectedNode]
	if isPopulator && nodeName != "" {
		podSpec.NodeName = nodeName
	}
}

// CreatePvc creates PVC
func CreatePvc(name, ns string, annotations, labels map[string]string) *corev1.PersistentVolumeClaim {
	return CreatePvcInStorageClass(name, ns, nil, annotations, labels, corev1.ClaimBound)
}

// CreatePvcInStorageClass creates PVC with storgae class
func CreatePvcInStorageClass(name, ns string, storageClassName *string, annotations, labels map[string]string, phase corev1.PersistentVolumeClaimPhase) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   ns,
			Annotations: annotations,
			Labels:      labels,
			UID:         types.UID(ns + "-" + name),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany, corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1G"),
				},
			},
			StorageClassName: storageClassName,
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: phase,
		},
	}
	pvc.Status.Capacity = pvc.Spec.Resources.Requests.DeepCopy()
	return pvc
}

// GetAPIServerKey returns API server RSA key
func GetAPIServerKey() *rsa.PrivateKey {
	apiServerKeyOnce.Do(func() {
		apiServerKey, _ = rsa.GenerateKey(rand.Reader, 2048)
	})
	return apiServerKey
}

// CreateStorageClass creates storage class CR
func CreateStorageClass(name string, annotations map[string]string) *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: annotations,
		},
	}
}

// CreateStorageClassWithProvisioner creates CR of storage class with provisioner
func CreateStorageClassWithProvisioner(name string, annotations, labels map[string]string, provisioner string) *storagev1.StorageClass {
	return &storagev1.StorageClass{
		Provisioner: provisioner,
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: annotations,
			Labels:      labels,
		},
	}
}

// ErrQuotaExceeded checked is the error is of exceeded quota
func ErrQuotaExceeded(err error) bool {
	return strings.Contains(err.Error(), "exceeded quota:")
}

// GetNamespace returns the given namespace if not empty, otherwise the default namespace
func GetNamespace(namespace, defaultNamespace string) string {
	if namespace == "" {
		return defaultNamespace
	}
	return namespace
}

// IsBound returns if the pvc is bound
func IsBound(pvc *corev1.PersistentVolumeClaim) bool {
	return pvc.Spec.VolumeName != ""
}

// IsUnbound returns if the pvc is not bound yet
func IsUnbound(pvc *corev1.PersistentVolumeClaim) bool {
	return !IsBound(pvc)
}

// IsImageStream returns true if registry source is ImageStream
func IsImageStream(pvc *corev1.PersistentVolumeClaim) bool {
	return pvc.Annotations[AnnRegistryImageStream] == "true"
}

// ShouldIgnorePod checks if a pod should be ignored.
// If this is a completed pod that was used for one checkpoint of a multi-stage import, it
// should be ignored by pod lookups as long as the retainAfterCompletion annotation is set.
func ShouldIgnorePod(pod *corev1.Pod, pvc *corev1.PersistentVolumeClaim) bool {
	retain := pvc.ObjectMeta.Annotations[AnnPodRetainAfterCompletion]
	checkpoint := pvc.ObjectMeta.Annotations[AnnCurrentCheckpoint]
	if checkpoint != "" && pod.Status.Phase == corev1.PodSucceeded {
		return retain == "true"
	}
	return false
}

// SetRecommendedLabels sets the recommended labels on CDI resources (does not get rid of existing ones)
func SetRecommendedLabels(obj metav1.Object, installerLabels map[string]string, controllerName string) {
	staticLabels := map[string]string{
		common.AppKubernetesManagedByLabel: controllerName,
		common.AppKubernetesComponentLabel: "storage",
	}

	// Merge existing labels with static labels and add installer dynamic labels as well (/version, /part-of).
	mergedLabels := common.MergeLabels(obj.GetLabels(), staticLabels, installerLabels)

	obj.SetLabels(mergedLabels)
}

func NamespacedName(obj client.Object) types.NamespacedName {
	return types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
}

func MatchLabelSelector(labels map[string]string, selector metav1.LabelSelector) bool {
	return MatchLabels(labels, selector.MatchLabels) && MatchExpressions(labels, selector.MatchExpressions)
}

func MatchLabels(labels, matchLabels map[string]string) bool {
	for key, value := range matchLabels {
		if labels[key] != value {
			return false
		}
	}
	return true
}

func MatchExpressions(labels map[string]string, expressions []metav1.LabelSelectorRequirement) bool {
	for _, expr := range expressions {
		switch expr.Operator {
		case metav1.LabelSelectorOpIn:
			if !slices.Contains(expr.Values, labels[expr.Key]) {
				return false
			}
		case metav1.LabelSelectorOpNotIn:
			if slices.Contains(expr.Values, labels[expr.Key]) {
				return false
			}
		case metav1.LabelSelectorOpExists:
			if _, ok := labels[expr.Key]; !ok {
				return false
			}
		case metav1.LabelSelectorOpDoesNotExist:
			if _, ok := labels[expr.Key]; ok {
				return false
			}
		}
	}
	return true
}
