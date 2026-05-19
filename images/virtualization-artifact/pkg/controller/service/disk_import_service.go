/*
Copyright 2026 Flant JSC

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

package service

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization-controller/pkg/common/pvc"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements/copier"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	AnnObjectRefImportPhase         = "virtualization.deckhouse.io/object-ref-import.phase"
	AnnObjectRefImportPod           = "virtualization.deckhouse.io/object-ref-import.pod"
	AnnObjectRefImportSource        = "virtualization.deckhouse.io/object-ref-import.source"
	AnnObjectRefImportEndpoint      = "virtualization.deckhouse.io/object-ref-import.endpoint"
	AnnObjectRefImportSecret        = "virtualization.deckhouse.io/object-ref-import.secret"
	AnnObjectRefImportCertConfigMap = "virtualization.deckhouse.io/object-ref-import.cert-config-map"
	AnnObjectRefImportImageSize     = "virtualization.deckhouse.io/object-ref-import.image-size"
	AnnObjectRefImportCreatedBy     = "virtualization.deckhouse.io/object-ref-import.created-by"

	cdiDataVolName     = "cdi-data-vol"
	cdiScratchVolName  = "cdi-scratch-vol"
	cdiImporterDataDir = "/data"
	cdiScratchDataDir  = "/scratch"
	cdiWriteBlockPath  = "/dev/cdi-block-volume"
	cdiSourceBlockPath = "/dev/source-block-volume"
	sourceRegistry     = "registry"
	sourcePVC          = "pvc"
)

func (s DiskService) StartObjectRefDiskImport(ctx context.Context, pvcSize resource.Quantity, sc *storagev1.StorageClass, source *cdiv1.DataVolumeSource, vd *v1alpha2.VirtualDisk, nodePlacement *provisioner.NodePlacement) error {
	if sc == nil {
		return fmt.Errorf("cannot create import PVC: StorageClass must not be nil")
	}

	volumeMode, accessMode, err := s.GetVolumeAndAccessModes(ctx, vd, sc)
	if err != nil {
		return fmt.Errorf("get volume and access modes: %w", err)
	}

	target := &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{Kind: "PersistentVolumeClaim", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        vd.Status.Target.PersistentVolumeClaim,
			Namespace:   vd.Namespace,
			Annotations: s.objectRefImportAnnotations(source, pvcSize),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         v1alpha2.SchemeGroupVersion.String(),
				Kind:               v1alpha2.VirtualDiskKind,
				Name:               vd.Name,
				UID:                vd.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			}},
		},
		Spec: *pvc.CreateSpec(&sc.Name, pvcSize, accessMode, volumeMode),
	}

	if nodePlacement != nil {
		if err := provisioner.KeepNodePlacementTolerations(nodePlacement, target); err != nil {
			return fmt.Errorf("keep node placement: %w", err)
		}
	}

	err = s.client.Create(ctx, target)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func (s DiskService) EnsureObjectRefDiskImport(ctx context.Context, target *corev1.PersistentVolumeClaim, source *cdiv1.DataVolumeSource, vd *v1alpha2.VirtualDisk, nodePlacement *provisioner.NodePlacement) (corev1.PodPhase, error) {
	phase := corev1.PodPhase(target.Annotations[AnnObjectRefImportPhase])
	if phase == corev1.PodSucceeded {
		return phase, s.cleanupObjectRefDiskImport(ctx, target)
	}

	if err := s.ensureObjectRefImportSupplements(ctx, target, vd); err != nil {
		return "", err
	}

	scratch, err := s.ensureObjectRefScratchPVC(ctx, target)
	if err != nil {
		return "", err
	}

	var sourceClaim *corev1.PersistentVolumeClaim
	if source != nil && source.PVC != nil {
		sourceClaim, err = object.FetchObject(ctx, types.NamespacedName{Name: source.PVC.Name, Namespace: source.PVC.Namespace}, s.client, &corev1.PersistentVolumeClaim{})
		if err != nil {
			return "", fmt.Errorf("fetch source pvc: %w", err)
		}
		if sourceClaim == nil {
			return "", fmt.Errorf("source pvc %s/%s not found", source.PVC.Namespace, source.PVC.Name)
		}
	}

	pod, err := object.FetchObject(ctx, types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, s.client, &corev1.Pod{})
	if err != nil {
		return "", fmt.Errorf("fetch importer pod: %w", err)
	}
	if pod == nil {
		pod = s.makeObjectRefImporterPod(target, source, sourceClaim, scratch.Name, vd, nodePlacement)
		if err := s.client.Create(ctx, pod); err != nil && !k8serrors.IsAlreadyExists(err) {
			return "", fmt.Errorf("create importer pod: %w", err)
		}
		return corev1.PodPending, s.patchTargetImportPhase(ctx, target, corev1.PodPending)
	}

	if pod.Status.Phase != "" && pod.Status.Phase != phase {
		if err := s.patchTargetImportPhase(ctx, target, pod.Status.Phase); err != nil {
			return "", err
		}
	}
	if pod.Status.Phase == corev1.PodSucceeded {
		return pod.Status.Phase, s.cleanupObjectRefDiskImport(ctx, target)
	}
	return pod.Status.Phase, nil
}

func (s DiskService) objectRefImportAnnotations(source *cdiv1.DataVolumeSource, size resource.Quantity) map[string]string {
	anno := map[string]string{
		AnnObjectRefImportPod:       "",
		AnnObjectRefImportPhase:     string(corev1.PodPending),
		AnnObjectRefImportImageSize: size.String(),
	}
	if source != nil && source.Registry != nil {
		anno[AnnObjectRefImportSource] = sourceRegistry
		if source.Registry.URL != nil {
			anno[AnnObjectRefImportEndpoint] = *source.Registry.URL
		}
		if source.Registry.SecretRef != nil {
			anno[AnnObjectRefImportSecret] = *source.Registry.SecretRef
		}
		if source.Registry.CertConfigMap != nil {
			anno[AnnObjectRefImportCertConfigMap] = *source.Registry.CertConfigMap
		}
	}
	if source != nil && source.PVC != nil {
		anno[AnnObjectRefImportSource] = sourcePVC
		anno[AnnObjectRefImportEndpoint] = source.PVC.Namespace + "/" + source.PVC.Name
	}
	return anno
}

func (s DiskService) ensureObjectRefImportSupplements(ctx context.Context, target *corev1.PersistentVolumeClaim, vd *v1alpha2.VirtualDisk) error {
	if s.dvcrSettings == nil {
		return nil
	}

	supGen := supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID)
	ownerRef := metav1.OwnerReference{
		APIVersion:         "v1",
		Kind:               "PersistentVolumeClaim",
		Name:               target.Name,
		UID:                target.UID,
		Controller:         ptr.To(false),
		BlockOwnerDeletion: ptr.To(true),
	}

	if s.dvcrSettings.AuthSecret != "" {
		authCopier := copier.AuthSecret{
			Secret: copier.Secret{
				Source: types.NamespacedName{
					Name:      s.dvcrSettings.AuthSecret,
					Namespace: s.dvcrSettings.AuthSecretNamespace,
				},
				Destination:    supGen.DVCRAuthSecretForDV(),
				OwnerReference: ownerRef,
			},
		}
		if err := authCopier.CopyCDICompatible(ctx, s.client, s.dvcrSettings.RegistryURL); err != nil {
			return fmt.Errorf("copy dvcr auth secret: %w", err)
		}
	}

	if s.dvcrSettings.CertsSecret != "" {
		caBundleCopier := copier.CABundleConfigMap{
			SourceSecret: types.NamespacedName{
				Name:      s.dvcrSettings.CertsSecret,
				Namespace: s.dvcrSettings.CertsSecretNamespace,
			},
			Destination:    supGen.DVCRCABundleConfigMapForDV(),
			OwnerReference: ownerRef,
		}
		if err := caBundleCopier.Copy(ctx, s.client); err != nil {
			return fmt.Errorf("copy dvcr ca bundle: %w", err)
		}
	}

	return nil
}

func (s DiskService) ensureObjectRefScratchPVC(ctx context.Context, target *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	name := target.Name + "-scratch"
	scratch, err := object.FetchObject(ctx, types.NamespacedName{Name: name, Namespace: target.Namespace}, s.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return nil, fmt.Errorf("fetch scratch pvc: %w", err)
	}
	if scratch != nil {
		return scratch, nil
	}

	size := target.Spec.Resources.Requests[corev1.ResourceStorage]
	volumeMode := corev1.PersistentVolumeFilesystem
	scratch = &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{Kind: "PersistentVolumeClaim", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: target.Namespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         "v1",
				Kind:               "PersistentVolumeClaim",
				Name:               target.Name,
				UID:                target.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			}},
		},
		Spec: target.Spec,
	}
	scratch.Spec.VolumeName = ""
	scratch.Spec.VolumeMode = &volumeMode
	scratch.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	scratch.Spec.Resources.Requests[corev1.ResourceStorage] = size
	if err := s.client.Create(ctx, scratch); err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("create scratch pvc: %w", err)
	}
	return scratch, nil
}

func (s DiskService) makeObjectRefImporterPod(target *corev1.PersistentVolumeClaim, source *cdiv1.DataVolumeSource, sourceClaim *corev1.PersistentVolumeClaim, scratchName string, vd *v1alpha2.VirtualDisk, nodePlacement *provisioner.NodePlacement) *corev1.Pod {
	annotations := map[string]string{AnnObjectRefImportCreatedBy: "yes"}
	target.Annotations[AnnObjectRefImportPod] = target.Name

	container := corev1.Container{
		Name:            "d8v-cdi-importer",
		Image:           s.diskImporterImage,
		ImagePullPolicy: corev1.PullPolicy(s.pullPolicy),
		Command:         []string{"/usr/bin/cdi-importer"},
		Args:            []string{"-v=" + s.verbose},
		Env: []corev1.EnvVar{
			{Name: common.ImporterSource, Value: sourceRegistry},
			{Name: common.ImporterEndpoint, Value: target.Annotations[AnnObjectRefImportEndpoint]},
			{Name: common.ImporterContentType, Value: string(cdiv1.DataVolumeKubeVirt)},
			{Name: common.ImporterImageSize, Value: target.Annotations[AnnObjectRefImportImageSize]},
			{Name: common.OwnerUID, Value: string(target.UID)},
			{Name: common.FilesystemOverheadVar, Value: "0"},
			{Name: common.InsecureTLSVar, Value: "false"},
			{Name: "PREALLOCATION", Value: "false"},
		},
		VolumeMounts: []corev1.VolumeMount{{Name: cdiScratchVolName, MountPath: cdiScratchDataDir}, {Name: "tmp", MountPath: "/tmp"}},
		Ports:        []corev1.ContainerPort{{Name: "metrics", ContainerPort: 8443, Protocol: corev1.ProtocolTCP}},
	}
	if s.resourceRequirements.Requests != nil || s.resourceRequirements.Limits != nil {
		container.Resources = s.resourceRequirements
	}
	if secretName := target.Annotations[AnnObjectRefImportSecret]; secretName != "" {
		container.Env = append(container.Env, corev1.EnvVar{
			Name: common.ImporterAccessKeyID,
			ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Key:                  importer.KeyAccess,
			}},
		}, corev1.EnvVar{
			Name: common.ImporterSecretKey,
			ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Key:                  importer.KeySecret,
			}},
		})
	}
	if target.Annotations[AnnObjectRefImportCertConfigMap] != "" {
		container.Env = append(container.Env, corev1.EnvVar{Name: common.ImporterCertDirVar, Value: common.ImporterCertDir})
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: "cert-vol", MountPath: common.ImporterCertDir})
	}
	if target.Spec.VolumeMode != nil && *target.Spec.VolumeMode == corev1.PersistentVolumeBlock {
		container.VolumeDevices = []corev1.VolumeDevice{{Name: cdiDataVolName, DevicePath: cdiWriteBlockPath}}
	} else {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: cdiDataVolName, MountPath: cdiImporterDataDir})
	}

	volumes := []corev1.Volume{
		{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		{Name: cdiDataVolName, VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: target.Name}}},
		{Name: cdiScratchVolName, VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: scratchName}}},
	}
	if certConfigMap := target.Annotations[AnnObjectRefImportCertConfigMap]; certConfigMap != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "cert-vol",
			VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: certConfigMap},
			}},
		})
	}
	if source != nil && source.PVC != nil && sourceClaim != nil {
		sourcePath := "/source/disk.img"
		if sourceClaim.Spec.VolumeMode != nil && *sourceClaim.Spec.VolumeMode == corev1.PersistentVolumeBlock {
			sourcePath = cdiSourceBlockPath
			container.VolumeDevices = append(container.VolumeDevices, corev1.VolumeDevice{Name: "source-vol", DevicePath: cdiSourceBlockPath})
		} else {
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: "source-vol", MountPath: "/source", ReadOnly: true})
		}

		targetPath := cdiImporterDataDir + "/disk.img"
		if target.Spec.VolumeMode != nil && *target.Spec.VolumeMode == corev1.PersistentVolumeBlock {
			targetPath = cdiWriteBlockPath
		}

		container.Command = []string{"/bin/sh", "-c"}
		container.Args = []string{fmt.Sprintf("qemu-img convert -p -O raw %q %q", sourcePath, targetPath)}
		container.Env = nil
		container.Ports = nil
		volumes = append(volumes, corev1.Volume{
			Name: "source-vol",
			VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: source.PVC.Name,
				ReadOnly:  true,
			}},
		})
	}

	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        target.Name,
			Namespace:   target.Namespace,
			Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         "v1",
				Kind:               "PersistentVolumeClaim",
				Name:               target.Name,
				UID:                target.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			}},
		},
		Spec: corev1.PodSpec{
			Containers:    []corev1.Container{container},
			RestartPolicy: corev1.RestartPolicyOnFailure,
			Volumes:       volumes,
		},
	}
	podutil.SetRestrictedSecurityContext(&pod.Spec)
	if nodePlacement != nil {
		pod.Spec.Tolerations = nodePlacement.Tolerations
		_ = provisioner.KeepNodePlacementTolerations(nodePlacement, pod)
	}
	return pod
}

func (s DiskService) patchTargetImportPhase(ctx context.Context, target *corev1.PersistentVolumeClaim, phase corev1.PodPhase) error {
	copy := target.DeepCopy()
	if copy.Annotations == nil {
		copy.Annotations = map[string]string{}
	}
	copy.Annotations[AnnObjectRefImportPhase] = string(phase)
	copy.Annotations[AnnObjectRefImportPod] = target.Name
	return s.client.Patch(ctx, copy, client.MergeFrom(target))
}

func (s DiskService) cleanupObjectRefDiskImport(ctx context.Context, target *corev1.PersistentVolumeClaim) error {
	for _, obj := range []client.Object{
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: target.Name, Namespace: target.Namespace}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: target.Name + "-scratch", Namespace: target.Namespace}},
	} {
		err := s.client.Delete(ctx, obj)
		if err != nil && !k8serrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}
