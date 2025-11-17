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

package annotations

import (
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/merger"
)

const (
	CVIShortName = "cvi"
	VDShortName  = "vd"
	VIShortName  = "vi"

	// AnnIntegrityGroup is the Integrity for virtualization-contrller.
	AnnIntegrityGroup = "integrity.virtualization.deckhouse.io/"

	// AnnIntegrityCoreChecksumis is the integrity check VMI checksum.
	AnnIntegrityCoreChecksum = AnnIntegrityGroup + "core-spec-checksum"

	// AnnIntegrityCoreChecksumisApplied is the integrity check VMI applied checksum.
	AnnIntegrityCoreChecksumApplied = AnnIntegrityGroup + "core-spec-checksum-applied"

	// AnnAPIGroup is the APIGroup for virtualization-controller.
	AnnAPIGroup = "virt.deckhouse.io"

	// AnnCreatedBy is a pod annotation indicating if the pod was created by the PVC.
	AnnCreatedBy = AnnAPIGroup + "/storage.createdByController"

	// AnnPodRetainAfterCompletion is PVC annotation for retaining transfer pods after completion
	AnnPodRetainAfterCompletion = AnnAPIGroup + "/storage.pod.retainAfterCompletion"

	// AnnUploadURLDeprecated provides a const for CVMI/VMI/VMD uploadURL annotation.
	// TODO remove annotation and its usages after version 1.0 becomes Stable.
	AnnUploadURLDeprecated = AnnAPIGroup + "/upload.url"

	// AnnTolerationsHash provides a const for annotation with hash of applied tolerations.
	AnnTolerationsHash = AnnAPIGroup + "/tolerations-hash"
	// AnnProvisionerTolerations provides a const for tolerations to use for provisioners.
	AnnProvisionerTolerations = AnnAPIGroup + "/provisioner-tolerations"
	// AnnProvisionerName provides a name of data volume provisioner.
	AnnProvisionerName = AnnAPIGroup + "/provisioner-name"

	// AnnDefaultStorageClass is the annotation indicating that a storage class is the default one.
	AnnDefaultStorageClass = "storageclass.kubernetes.io/is-default-class"

	AnnAPIGroupV              = "virtualization.deckhouse.io"
	AnnVirtualDisk            = "virtualdisk." + AnnAPIGroupV
	AnnVirtualDiskVolumeMode  = AnnVirtualDisk + "/volume-mode"
	AnnVirtualDiskAccessMode  = AnnVirtualDisk + "/access-mode"
	AnnVirtualDiskBindingMode = AnnVirtualDisk + "/binding-mode"

	AnnVirtualMachineClass        = "virtualmachineclass." + AnnAPIGroupV
	AnnVirtualMachineClassDefault = AnnVirtualMachineClass + "/is-default-class"

	// AnnVMLastAppliedSpec is an annotation on KVVM. It contains a JSON with VM spec.
	AnnVMLastAppliedSpec = AnnAPIGroup + "/vm.last-applied-spec"

	// AnnVMClassLastAppliedSpec is an annotation on KVVM. It contains a JSON with VM spec.
	AnnVMClassLastAppliedSpec = AnnAPIGroup + "/vmclass.last-applied-spec"

	// LastPropagatedVMAnnotationsAnnotation is a marshalled map of previously applied virtual machine annotations.
	LastPropagatedVMAnnotationsAnnotation = AnnAPIGroup + "/last-propagated-vm-annotations"
	// LastPropagatedVMLabelsAnnotation is a marshalled map of previously applied virtual machine labels.
	LastPropagatedVMLabelsAnnotation = AnnAPIGroup + "/last-propagated-vm-labels"

	AnnOsType = AnnAPIGroupV + "/os-type"

	// AnnVMStartRequested is an annotation on KVVM that represents a request to start a virtual machine.
	AnnVMStartRequested = AnnAPIGroupV + "/vm-start-requested"

	// AnnVMRestartRequested is an annotation on KVVM that represents a request to restart a virtual machine.
	AnnVMRestartRequested = AnnAPIGroupV + "/vm-restart-requested"

	// AnnVMOPWorkloadUpdate is an annotation on vmop that represents a vmop created by workload-updater controller.
	AnnVMOPWorkloadUpdate                 = AnnAPIGroupV + "/workload-update"
	AnnVMOPWorkloadUpdateImage            = AnnAPIGroupV + "/workload-update-image"
	AnnVMOPWorkloadUpdateNodePlacementSum = AnnAPIGroupV + "/workload-update-node-placement-sum"
	// AnnVMRestore is an annotation on a resource that indicates it was created by the vmrestore controller; the value is the UID of the `VirtualMachineRestore` resource.
	AnnVMRestore = AnnAPIGroupV + "/vmrestore"
	// AnnVMOPEvacuation is an annotation on vmop that represents a vmop created by evacuation controller
	AnnVMOPEvacuation = AnnAPIGroupV + "/evacuation"
	// AnnVMOPVolumeMigration is an annotation on vmop that represents a vmop created by volume-migration controller
	AnnVMOPVolumeMigration = AnnAPIGroupV + "/volume-migration"

	// AnnVMOPRestore is an annotation on a resource that indicates it was created by the vmop snapshot controller; the value is the UID of the `VirtualMachineOperation` resource.
	AnnVMOPRestore = AnnAPIGroupV + "/vmoprestore"
	// AnnVMRestoreDeleted is an annotation on a resource that indicates it was deleted by the vmop snapshot controller; the value is the UID of the `VirtualMachineOperation` resource.
	AnnVMOPRestoreDeleted = AnnAPIGroupV + "/vmoprestoredeleted"

	// AnnUploadURL is an annotation on Ingress with full URL to upload image from outside the cluster.
	AnnUploadURL = AnnAPIGroupV + "/upload.url"
	// AnnUploadPath is an annotation on Ingress with the URL path to upload image.
	AnnUploadPath = AnnAPIGroupV + "/upload.path"

	// LabelsPrefix is a prefix for virtualization-controller labels.
	LabelsPrefix = "virtualization.deckhouse.io"

	// LabelVirtualMachineUID is a label to link VirtualMachineIPAddress to VirtualMachine.
	LabelVirtualMachineUID = LabelsPrefix + "/virtual-machine-uid"

	// LabelVirtualMachineIPAddressUID is a label to link VirtualMachineIPAddressLease to VirtualMachineIPAddress.
	LabelVirtualMachineIPAddressUID = LabelsPrefix + "/virtual-machine-ip-address-uid"

	// SkipPodSecurityStandardsCheckLabel is a label to skip the check for PodSecurityStandards to avoid irrelevant alerts related to a privileged virtual machine pod.
	SkipPodSecurityStandardsCheckLabel = "security.deckhouse.io/skip-pss-check"

	// LabelVirtualMachineMACAddressUID is a label to link VirtualMachineMACAddressLease to VirtualMachineMACAddress.
	LabelVirtualMachineMACAddressUID = LabelsPrefix + "/virtual-machine-mac-address-uid"

	UploaderServiceLabel = "service"

	// AppKubernetesManagedByLabel is the Kubernetes recommended managed-by label.
	AppKubernetesManagedByLabel = "app.kubernetes.io/managed-by"
	// AppKubernetesComponentLabel is the Kubernetes recommended component label.
	AppKubernetesComponentLabel = "app.kubernetes.io/component"

	// AnnVersionsGroup is the internal APIGroup for virtualization-controller.
	AnnVersionsGroup = "versions." + AnnAPIGroupV
	// AnnQemuVersion is a pod annotation indicating qemu version.
	AnnQemuVersion = AnnVersionsGroup + "/qemu-version"
	// AnnLibvirtVersion is a pod annotation indicating libvirt version.
	AnnLibvirtVersion = AnnVersionsGroup + "/libvirt-version"

	// AnnAuditDecision is the Kubernetes annotation for audit event decision.
	AnnAuditDecision = "authorization.k8s.io/decision"
	// AnnAuditReason is the Kubernetes annotation for audit event reason.
	AnnAuditReason = "authorization.k8s.io/reason"

	// AnnNodeCpuFeature is the Kubevirt annotation for CPU feature.
	AnnNodeCPUFeature = "cpu-feature.node.virtualization.deckhouse.io/"

	// AnnDataExportRequest is the annotation for indicating that export requested.
	AnnDataExportRequest = "storage.deckhouse.io/data-export-request"

	// TODO: remove deprecated annotations in the v1 version.
	// AnnStorageClassName is the annotation for indicating that storage class name. (USED IN STORAGE sds controllers)
	AnnStorageClassName           = AnnAPIGroupV + "/storage-class-name"
	AnnStorageClassNameDeprecated = "storageClass"
	// AnnVolumeMode is the annotation for indicating that volume mode.  (USED IN STORAGE sds controllers)
	AnnVolumeMode           = AnnAPIGroupV + "/volume-mode"
	AnnVolumeModeDeprecated = "volumeMode"
	// AnnAccessMode is the annotation for indicating that access mode.  (USED IN STORAGE sds controllers)
	AnnAccessModes           = AnnAPIGroupV + "/access-mode"
	AnnAccessModesDeprecated = "accessModes"
	// AnnStorageProvisioner is the annotation for indicating storage provisioner
	AnnStorageProvisioner           = "volume.kubernetes.io/storage-provisioner"
	AnnStorageProvisionerDeprecated = "volume.beta.kubernetes.io/storage-provisioner"

	// AppLabel is the app name label.
	AppLabel = "app"
	// CDILabelValue provides a constant  for CDI Pod label values.
	CDILabelValue = "containerized-data-importer"
	// DVCRLabelValue provides a constant  for DVCR Pod label values.
	DVCRLabelValue = "dvcr-data-importer"

	// InhibitNodeShutdownLabel is a label to prevent node shutdown is Pod with label is present.
	InhibitNodeShutdownLabel = "pod.deckhouse.io/inhibit-node-shutdown"

	// AnnNetworksSpec is the annotation for request network configuration into Pod.
	AnnNetworksSpec = "network.deckhouse.io/networks-spec"
	// AnnNetworksStatus is the annotation for view current network configuration into Pod.
	AnnNetworksStatus = "network.deckhouse.io/networks-status"

	// AnnVirtualDiskOriginalAnnotations is the annotation for storing original VirtualDisk annotations.
	AnnVirtualDiskOriginalAnnotations = AnnAPIGroupV + "/vd-original-annotations"
	// AnnVirtualDiskOriginalLabels is the annotation for storing original VirtualDisk labels.
	AnnVirtualDiskOriginalLabels = AnnAPIGroupV + "/vd-original-labels"
	// AnnVMOPUID is an annotation on vmop that represents name of VMOP.
	AnnVMOPUID = AnnAPIGroupV + "/vmop-uid"
	// AnnVMOPSnapshotName is an annotation on vmop that represents name a snapshot created for VMOP.
	AnnVMOPSnapshotName = AnnAPIGroupV + "/vmop-snapshot-name"

	// AnnVMFilesystemRequest is an annotation on a virtual machine that indicates a request to freeze or unfreeze the filesystem has been sent.
	AnnVMFilesystemRequest = AnnAPIGroup + "/virtual-machine-filesystem-request"
)

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

// SetRecommendedLabels sets the recommended labels on CDI resources (does not get rid of existing ones)
func SetRecommendedLabels(obj metav1.Object, installerLabels map[string]string, controllerName string) {
	staticLabels := map[string]string{
		AppKubernetesManagedByLabel: controllerName,
		AppKubernetesComponentLabel: "storage",
	}

	// Merge existing labels with static labels and add installer dynamic labels as well (/version, /part-of).
	mergedLabels := merger.MergeLabels(obj.GetLabels(), staticLabels, installerLabels)

	obj.SetLabels(mergedLabels)
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
