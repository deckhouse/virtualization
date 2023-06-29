package common

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/common"
)

const (
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

	// AnnSecret provides a const for our PVC secretName annotation
	AnnSecret = AnnAPIGroup + "/storage.import.secretName"
	// AnnCertConfigMap is the name of a configmap containing tls certs
	AnnCertConfigMap = AnnAPIGroup + "/storage.import.certConfigMap"
	// AnnRegistryImportMethod provides a const for registry import method annotation
	AnnRegistryImportMethod = AnnAPIGroup + "/storage.import.registryImportMethod"
	// AnnRegistryImageStream provides a const for registry image stream annotation
	AnnRegistryImageStream = AnnAPIGroup + "/storage.import.registryImageStream"
	// AnnImportPod provides a const for our PVC importPodName annotation
	AnnImportPod = AnnAPIGroup + "/storage.import.importPodName"
	// ImportDone this means importer Pod is completed successfully.
	AnnImportDone = AnnAPIGroup + "/storage.import.done"
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

	//AnnDefaultStorageClass is the annotation indicating that a storage class is the default one.
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

	// AnnSelectedNode annotation is added to a PVC that has been triggered by scheduler to
	// be dynamically provisioned. Its value is the name of the selected node.
	AnnSelectedNode = "volume.kubernetes.io/selected-node"

	// CloneUniqueID is used as a special label to be used when we search for the pod
	CloneUniqueID = "cdi.kubevirt.io/storage.clone.cloneUniqeId"

	// CloneSourceInUse is reason for event created when clone source pvc is in use
	CloneSourceInUse = "CloneSourceInUse"

	// CloneComplete message
	CloneComplete = "Clone Complete"

	cloneTokenLeeway = 10 * time.Second

	// Default value for preallocation option if not defined in DV or CDIConfig
	defaultPreallocation = false

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

	// ProgressDone this means we are DONE
	ProgressDone = "100.0%"
)

// Size-detection pod error codes
const (
	NoErr int = iota
	ErrBadArguments
	ErrInvalidFile
	ErrInvalidPath
	ErrBadTermFile
	ErrUnknown
)

var (
	apiServerKeyOnce sync.Once
	apiServerKey     *rsa.PrivateKey
)

// GetStorageClassByName looks up the storage class based on the name. If no storage class is found returns nil
func GetStorageClassByName(ctx context.Context, client client.Client, name *string) (*storagev1.StorageClass, error) {
	// look up storage class by name
	if name != nil {
		storageClass := &storagev1.StorageClass{}
		if err := client.Get(ctx, types.NamespacedName{Name: *name}, storageClass); err != nil {
			if k8serrors.IsNotFound(err) {
				return nil, nil
			}
			klog.V(3).Info("Unable to retrieve storage class", "storage class name", *name)
			return nil, errors.Errorf("unable to retrieve storage class %s", *name)
		}
		return storageClass, nil
	}
	// No storage class found, just return nil for storage class and let caller deal with it.
	return GetDefaultStorageClass(ctx, client)
}

// GetDefaultStorageClass returns the default storage class or nil if none found
func GetDefaultStorageClass(ctx context.Context, client client.Client) (*storagev1.StorageClass, error) {
	storageClasses := &storagev1.StorageClassList{}
	if err := client.List(ctx, storageClasses); err != nil {
		klog.V(3).Info("Unable to retrieve available storage classes")
		return nil, errors.New("unable to retrieve storage classes")
	}
	for _, storageClass := range storageClasses.Items {
		if storageClass.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			return &storageClass, nil
		}
	}

	return nil, nil
}

//// GetDefaultPodResourceRequirements gets default pod resource requirements from cdi config status
//func GetDefaultPodResourceRequirements(client client.Client) (*corev1.ResourceRequirements, error) {
//	cdiconfig := &cdiv1.CDIConfig{}
//	if err := client.Get(context.TODO(), types.NamespacedName{Name: common.ConfigName}, cdiconfig); err != nil {
//		klog.Errorf("Unable to find CDI configuration, %v\n", err)
//		return nil, err
//	}
//
//	return cdiconfig.Status.DefaultPodResourceRequirements, nil
//}

//// GetImagePullSecrets gets the imagePullSecrets needed to pull images from the cdi config
//func GetImagePullSecrets(client client.Client) ([]corev1.LocalObjectReference, error) {
//	cdiconfig := &cdiv1.CDIConfig{}
//	if err := client.Get(context.TODO(), types.NamespacedName{Name: common.ConfigName}, cdiconfig); err != nil {
//		klog.Errorf("Unable to find CDI configuration, %v\n", err)
//		return nil, err
//	}
//
//	return cdiconfig.Status.ImagePullSecrets, nil
//}

//// AddVolumeDevices returns VolumeDevice slice with one block device for pods using PV with block volume mode
//func AddVolumeDevices() []corev1.VolumeDevice {
//	volumeDevices := []corev1.VolumeDevice{
//		{
//			Name:       DataVolName,
//			DevicePath: common.WriteBlockPath,
//		},
//	}
//	return volumeDevices
//}

//// GetWorkloadNodePlacement extracts the workload-specific nodeplacement values from the CDI CR
//func GetWorkloadNodePlacement(ctx context.Context, c client.Client) (*sdkapi.NodePlacement, error) {
//	cr, err := GetActiveCDI(ctx, c)
//	if err != nil {
//		return nil, err
//	}
//
//	if cr == nil {
//		return nil, fmt.Errorf("no active CDI")
//	}
//
//	return &cr.Spec.Workloads, nil
//}
//
//// GetActiveCDI returns the active CDI CR
//func GetActiveCDI(ctx context.Context, c client.Client) (*cdiv1.CDI, error) {
//	crList := &cdiv1.CDIList{}
//	if err := c.List(ctx, crList, &client.ListOptions{}); err != nil {
//		return nil, err
//	}
//
//	var activeResources []cdiv1.CDI
//	for _, cr := range crList.Items {
//		if cr.Status.Phase != sdkapi.PhaseError {
//			activeResources = append(activeResources, cr)
//		}
//	}
//
//	if len(activeResources) == 0 {
//		return nil, nil
//	}
//
//	if len(activeResources) > 1 {
//		return nil, fmt.Errorf("number of active CDI CRs > 1")
//	}
//
//	return &activeResources[0], nil
//}

// GetPriorityClass gets PVC priority class
func GetPriorityClass(pvc *corev1.PersistentVolumeClaim) string {
	anno := pvc.GetAnnotations()
	return anno[AnnPriorityClassName]
}

// ShouldDeletePod returns whether the importer pod should be deleted:
// - CVMI has no annotation to retain pod after import
// - CVMI is deleted
func ShouldDeletePod(cvmi *virtv2alpha1.ClusterVirtualMachineImage) bool {
	return cvmi.GetAnnotations()[AnnPodRetainAfterCompletion] != "true" || cvmi.DeletionTimestamp != nil
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

// HandleFailedPod handles pod-creation errors and updates the CVMI without providing sensitive information.
// TODO make work with VirtualMachineImage object.
func HandleFailedPod(err error, podName string, cvmi *virtv2alpha1.ClusterVirtualMachineImage, recorder record.EventRecorder, c client.Client) error {
	if err == nil {
		return nil
	}
	// Generic reason and msg to avoid providing sensitive information
	reason := ErrStartingPod
	msg := fmt.Sprintf(MessageErrStartingPod, podName)

	// Error handling to fine-tune the event with pertinent info
	if ErrQuotaExceeded(err) {
		reason = ErrExceededQuota
	}

	recorder.Event(cvmi, corev1.EventTypeWarning, reason, msg)

	if isCloneSourcePod := CreateCloneSourcePodName(cvmi) == podName; isCloneSourcePod {
		AddAnnotation(cvmi, AnnSourceRunningCondition, "false")
		AddAnnotation(cvmi, AnnSourceRunningConditionReason, reason)
		AddAnnotation(cvmi, AnnSourceRunningConditionMessage, msg)
	} else {
		AddAnnotation(cvmi, AnnRunningCondition, "false")
		AddAnnotation(cvmi, AnnRunningConditionReason, reason)
		AddAnnotation(cvmi, AnnRunningConditionMessage, msg)
	}

	AddAnnotation(cvmi, AnnPodPhase, string(corev1.PodFailed))
	if err := c.Update(context.TODO(), cvmi); err != nil {
		return err
	}

	return err
}

type getSource interface {
	GetDataSource() virtv2alpha1.DataSource
}

// GetSource returns the source string which determines the type of source. If no source or invalid source found, default to http
func GetSource(obj getSource) string {
	srcType := string(obj.GetDataSource().Type)
	switch srcType {
	case
		SourceHTTP,
		SourceS3,
		SourceGCS,
		SourceGlance,
		SourceNone,
		SourceRegistry,
		SourceImageio,
		SourceVDDK:
	default:
		srcType = SourceHTTP
	}
	return srcType
}

type UIDable interface {
	GetUID() types.UID
}

// CreateCloneSourcePodName creates clone source pod name
func CreateCloneSourcePodName(obj UIDable) string {
	//return ""
	return string(obj.GetUID()) + common.ClonerSourcePodNameSuffix
}

// IsPVCComplete returns true if a PVC is in 'Succeeded' phase, false if not
func IsPVCComplete(pvc *corev1.PersistentVolumeClaim) bool {
	if pvc != nil {
		phase, exists := pvc.ObjectMeta.Annotations[AnnPodPhase]
		return exists && (phase == string(corev1.PodSucceeded))
	}
	return false
}

// IsPodComplete returns true if a Pod is in 'Completed' phase, false if not.
func IsPodComplete(pod *corev1.Pod) bool {
	if pod != nil {
		return pod.Status.Phase == corev1.PodSucceeded
		//phase, exists := pvc.ObjectMeta.Annotations[AnnPodPhase]
		//return exists && (phase == string(corev1.PodSucceeded))
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
			container.SecurityContext.AllowPrivilegeEscalation = pointer.Bool(false)
			container.SecurityContext.RunAsNonRoot = pointer.Bool(true)
			container.SecurityContext.RunAsUser = pointer.Int64(common.QemuSubGid)
			if len(container.VolumeMounts) > 0 {
				hasVolumeMounts = true
			}
		}
	}

	if hasVolumeMounts {
		if podSpec.SecurityContext == nil {
			podSpec.SecurityContext = &corev1.PodSecurityContext{}
		}
		podSpec.SecurityContext.FSGroup = pointer.Int64(common.QemuSubGid)
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
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("1G"),
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

// IsErrCacheNotStarted checked is the error is of cache not started
func IsErrCacheNotStarted(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*cache.ErrCacheNotStarted)
	return ok
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

// BuildHTTPClient generates an http client that accepts any certificate, since we are using
// it to get prometheus data it doesn't matter if someone can intercept the data. Once we have
// a mechanism to properly sign the server, we can update this method to get a proper client.
func BuildHTTPClient(httpClient *http.Client) *http.Client {
	if httpClient == nil {
		defaultTransport := http.DefaultTransport.(*http.Transport)
		// Create new Transport that ignores self-signed SSL
		tr := &http.Transport{
			Proxy:                 defaultTransport.Proxy,
			DialContext:           defaultTransport.DialContext,
			MaxIdleConns:          defaultTransport.MaxIdleConns,
			IdleConnTimeout:       defaultTransport.IdleConnTimeout,
			ExpectContinueTimeout: defaultTransport.ExpectContinueTimeout,
			TLSHandshakeTimeout:   defaultTransport.TLSHandshakeTimeout,
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		}
		httpClient = &http.Client{
			Transport: tr,
		}
	}
	return httpClient
}

// ErrConnectionRefused checks for connection refused errors
func ErrConnectionRefused(err error) bool {
	return strings.Contains(err.Error(), "connection refused")
}

// GetPodMetricsPort returns, if exists, the metrics port from the passed pod
func GetPodMetricsPort(pod *corev1.Pod) (int, error) {
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			if port.Name == "metrics" {
				return int(port.ContainerPort), nil
			}
		}
	}
	return 0, errors.New("Metrics port not found in pod")
}

// GetMetricsURL builds the metrics URL according to the specified pod
func GetMetricsURL(pod *corev1.Pod) (string, error) {
	if pod == nil {
		return "", nil
	}
	port, err := GetPodMetricsPort(pod)
	if err != nil || pod.Status.PodIP == "" {
		return "", err
	}
	url := fmt.Sprintf("https://%s:%d/metrics", pod.Status.PodIP, port)
	return url, nil
}

// GetProgressReportFromURL fetches the progress report from the passed URL according to a specific regular expression
func GetProgressReportFromURL(url string, regExp *regexp.Regexp, httpClient *http.Client) (string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		if ErrConnectionRefused(err) {
			return "", nil
		}
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	// Parse the progress from the body
	progressReport := ""
	match := regExp.FindStringSubmatch(string(body))
	if match != nil {
		progressReport = match[1]
	}
	return progressReport, nil
}
