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
	"errors"
	"reflect"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

	// AnnPodRetainAfterCompletion is PVC annotation for retaining transfer pods after completion
	AnnPodRetainAfterCompletion = AnnAPIGroup + "/storage.pod.retainAfterCompletion"

	// AnnUploadURL provides a const for CVMI/VMI/VMD uploadURL annotation
	AnnUploadURL = AnnAPIGroup + "/upload.url"

	// AnnDefaultStorageClass is the annotation indicating that a storage class is the default one.
	AnnDefaultStorageClass = "storageclass.kubernetes.io/is-default-class"

	AnnAPIGroupV              = "virtualization.deckhouse.io"
	AnnVirtualDisk            = "virtualdisk." + AnnAPIGroupV
	AnnVirtualDiskVolumeMode  = AnnVirtualDisk + "/volume-mode"
	AnnVirtualDiskAccessMode  = AnnVirtualDisk + "/access-mode"
	AnnVirtualDiskBindingMode = AnnVirtualDisk + "/binding-mode"

	// AnnVMLastAppliedSpec is an annotation on KVVM. It contains a JSON with VM spec.
	AnnVMLastAppliedSpec = AnnAPIGroup + "/vm.last-applied-spec"

	// LastPropagatedVMAnnotationsAnnotation is a marshalled map of previously applied virtual machine annotations.
	LastPropagatedVMAnnotationsAnnotation = AnnAPIGroup + "/last-propagated-vm-annotations"
	// LastPropagatedVMLabelsAnnotation is a marshalled map of previously applied virtual machine labels.
	LastPropagatedVMLabelsAnnotation = AnnAPIGroup + "/last-propagated-vm-labels"

	// LabelsPrefix is a prefix for virtualization-controller labels.
	LabelsPrefix = "virtualization.deckhouse.io"

	// LabelVirtualMachineUID is a label to link VirtualMachineIPAddress to VirtualMachine.
	LabelVirtualMachineUID = LabelsPrefix + "/virtual-machine-uid"

	UploaderServiceLabel = "service"

	// AppKubernetesManagedByLabel is the Kubernetes recommended managed-by label
	AppKubernetesManagedByLabel = "app.kubernetes.io/managed-by"
	// AppKubernetesComponentLabel is the Kubernetes recommended component label
	AppKubernetesComponentLabel = "app.kubernetes.io/component"

	// QemuSubGid is the gid used as the qemu group in fsGroup
	QemuSubGid = int64(107)
)

var (
	// ErrUnknownValue is a variable of type `error` that represents an error message indicating an unknown value.
	ErrUnknownValue = errors.New("unknown value")
	// ErrUnknownType is a variable of type `error` that represents an error message indicating an unknown type.
	ErrUnknownType = errors.New("unknown type")
)

// ShouldCleanupSubResources returns whether sub resources should be deleted:
// - CVMI, VMI has no annotation to retain pod after import
// - CVMI, VMI is deleted
func ShouldCleanupSubResources(obj metav1.Object) bool {
	return obj.GetAnnotations()[AnnPodRetainAfterCompletion] != "true" || obj.GetDeletionTimestamp() != nil
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

type UIDable interface {
	GetUID() types.UID
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
			container.SecurityContext.RunAsUser = ptr.To(QemuSubGid)
			if len(container.VolumeMounts) > 0 {
				hasVolumeMounts = true
			}
		}
	}

	if hasVolumeMounts {
		if podSpec.SecurityContext == nil {
			podSpec.SecurityContext = &corev1.PodSecurityContext{}
		}
		podSpec.SecurityContext.FSGroup = ptr.To(QemuSubGid)
	}
}

// ErrQuotaExceeded checked is the error is of exceeded quota
func ErrQuotaExceeded(err error) bool {
	return strings.Contains(err.Error(), "exceeded quota:")
}

// IsBound returns if the pvc is bound
// SetRecommendedLabels sets the recommended labels on CDI resources (does not get rid of existing ones)
func SetRecommendedLabels(obj metav1.Object, installerLabels map[string]string, controllerName string) {
	staticLabels := map[string]string{
		AppKubernetesManagedByLabel: controllerName,
		AppKubernetesComponentLabel: "storage",
	}

	// Merge existing labels with static labels and add installer dynamic labels as well (/version, /part-of).
	mergedLabels := common.MergeLabels(obj.GetLabels(), staticLabels, installerLabels)

	obj.SetLabels(mergedLabels)
}

func NamespacedName(obj client.Object) types.NamespacedName {
	return types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
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
