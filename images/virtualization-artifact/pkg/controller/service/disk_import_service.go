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

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
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
	cdiDataVolName        = "cdi-data-vol"
	cdiScratchVolName     = "cdi-scratch-vol"
	cdiImporterDataDir    = "/data"
	cdiScratchDataDir     = "/scratch"
	cdiWriteBlockPath     = "/dev/cdi-block-volume"
	cdiSourceBlockPath    = "/dev/source-block-volume"
	sourceRegistry        = "registry"
	sourcePVC             = "pvc"
	cloneStrategySnapshot = "snapshot"
	cloneStrategyCSI      = "csi-clone"
	cloneStrategyHost     = "host-assisted"
)

type PVCImportSource struct {
	Registry *PVCImportSourceRegistry
	PVC      *PVCImportSourcePVC
}

type PVCImportSourceRegistry struct {
	URL           string
	Secret        string
	CertConfigMap string
}

type PVCImportSourcePVC struct {
	Name      string
	Namespace string
}

func NewPVCRegistryImportSource(url, secret, certConfigMap string) *PVCImportSource {
	return &PVCImportSource{
		Registry: &PVCImportSourceRegistry{
			URL:           url,
			Secret:        secret,
			CertConfigMap: certConfigMap,
		},
	}
}

func NewPVCPVCImportSource(name, namespace string) *PVCImportSource {
	return &PVCImportSource{
		PVC: &PVCImportSourcePVC{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func (s DiskService) StartPVCImport(ctx context.Context, pvcSize resource.Quantity, sc *storagev1.StorageClass, source *PVCImportSource, vd *v1alpha2.VirtualDisk, nodePlacement *provisioner.NodePlacement) error {
	if sc == nil {
		return fmt.Errorf("cannot create import PVC: StorageClass must not be nil")
	}
	if source != nil && source.PVC != nil {
		return s.startPVCClone(ctx, pvcSize, sc, source.PVC, vd, nodePlacement)
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
			Annotations: s.pvcImportAnnotations(source, pvcSize),
			Finalizers:  s.diskProtectionFinalizers(),
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

func (s DiskService) StartSupplementPVCImport(ctx context.Context, pvcSize resource.Quantity, sc *storagev1.StorageClass, source *PVCImportSource, owner client.Object, sup supplements.Generator, nodePlacement *provisioner.NodePlacement) error {
	if sc == nil {
		return fmt.Errorf("cannot create import PVC: StorageClass must not be nil")
	}

	volumeMode, accessMode, err := s.GetVolumeAndAccessModes(ctx, owner, sc)
	if err != nil {
		return fmt.Errorf("get volume and access modes: %w", err)
	}

	target := &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{Kind: "PersistentVolumeClaim", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:            sup.PersistentVolumeClaim().Name,
			Namespace:       sup.PersistentVolumeClaim().Namespace,
			Annotations:     s.pvcImportAnnotations(source, pvcSize),
			Finalizers:      s.diskProtectionFinalizers(),
			OwnerReferences: []metav1.OwnerReference{ownerReferenceForObject(owner)},
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

func (s DiskService) EnsurePVCImport(ctx context.Context, target *corev1.PersistentVolumeClaim, source *PVCImportSource, vd *v1alpha2.VirtualDisk, nodePlacement *provisioner.NodePlacement) (corev1.PodPhase, error) {
	sup := supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID)
	return s.EnsureSupplementPVCImport(ctx, target, source, vd, sup, nodePlacement)
}

func (s DiskService) EnsureSupplementPVCImport(ctx context.Context, target *corev1.PersistentVolumeClaim, source *PVCImportSource, owner client.Object, sup supplements.Generator, nodePlacement *provisioner.NodePlacement) (corev1.PodPhase, error) {
	if source != nil && source.PVC != nil && isSmartCloneStrategy(target.Annotations[annotations.AnnPVCImportCloneStrategy]) {
		if target.Status.Phase == corev1.ClaimBound {
			if err := s.patchTargetImportPhase(ctx, target, corev1.PodSucceeded); err != nil {
				return "", err
			}
			return corev1.PodSucceeded, s.cleanupPVCImportClone(ctx, target)
		}
		return corev1.PodPending, nil
	}

	phase := corev1.PodPhase(target.Annotations[annotations.AnnPVCImportPhase])
	if phase == corev1.PodSucceeded {
		_, err := s.cleanupPVCImport(ctx, sup, target)
		return phase, err
	}

	if err := s.ensurePVCImportSupplements(ctx, target, sup); err != nil {
		return "", err
	}

	scratch, err := s.ensurePVCImportScratch(ctx, target)
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

	podKey := sup.PVCImporterPod()
	target.Annotations[annotations.AnnPVCImportPod] = podKey.Name
	pod, err := object.FetchObject(ctx, podKey, s.client, &corev1.Pod{})
	if err != nil {
		return "", fmt.Errorf("fetch importer pod: %w", err)
	}
	if pod == nil {
		pod = s.makePVCImporterPod(podKey.Name, target, source, sourceClaim, scratch.Name, nodePlacement)
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
		_, err := s.cleanupPVCImport(ctx, sup, target)
		return pod.Status.Phase, err
	}
	return pod.Status.Phase, nil
}

// pvcImportAnnotations builds the annotations applied on a target PVC to
// describe how its contents should be imported. The source can be a DVCR
// registry image (used by Upload/HTTP/Registry and ObjectRef CVI/VI data
// sources) or another PVC (used when cloning from a VirtualDisk).
// diskProtectionFinalizers returns the finalizer slice applied to PVCs that
// belong to a VirtualDisk/VirtualImage. The finalizer ensures the controller
// has a chance to perform explicit cleanup before garbage collection deletes
// the PVC. It is applied at creation time so no separate Protect call is
// required afterwards.
func (s DiskService) diskProtectionFinalizers() []string {
	if s.protection == nil {
		return nil
	}
	finalizer := s.protection.GetFinalizer()
	if finalizer == "" {
		return nil
	}
	return []string{finalizer}
}

func (s DiskService) pvcImportAnnotations(source *PVCImportSource, size resource.Quantity) map[string]string {
	anno := map[string]string{
		annotations.AnnPVCImportPod:       "",
		annotations.AnnPVCImportPhase:     string(corev1.PodPending),
		annotations.AnnPVCImportImageSize: size.String(),
	}
	if source != nil && source.Registry != nil {
		anno[annotations.AnnPVCImportSource] = sourceRegistry
		if source.Registry.URL != "" {
			anno[annotations.AnnPVCImportEndpoint] = source.Registry.URL
		}
		if source.Registry.Secret != "" {
			anno[annotations.AnnPVCImportSecret] = source.Registry.Secret
		}
		if source.Registry.CertConfigMap != "" {
			anno[annotations.AnnPVCImportCertConfigMap] = source.Registry.CertConfigMap
		}
	}
	if source != nil && source.PVC != nil {
		anno[annotations.AnnPVCImportSource] = sourcePVC
		anno[annotations.AnnPVCImportEndpoint] = source.PVC.Namespace + "/" + source.PVC.Name
	}
	return anno
}

func (s DiskService) ensurePVCImportSupplements(ctx context.Context, target *corev1.PersistentVolumeClaim, supGen supplements.Generator) error {
	if s.dvcrSettings == nil {
		return nil
	}

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

func (s DiskService) ensurePVCImportScratch(ctx context.Context, target *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	name := target.Name + "-scratch"
	scratch, err := object.FetchObject(ctx, types.NamespacedName{Name: name, Namespace: target.Namespace}, s.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return nil, fmt.Errorf("fetch scratch pvc: %w", err)
	}
	if scratch != nil {
		return scratch, nil
	}

	size := scratchPVCSize(target.Spec.Resources.Requests[corev1.ResourceStorage])
	volumeMode := corev1.PersistentVolumeFilesystem
	scratch = &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{Kind: "PersistentVolumeClaim", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: target.Namespace,
			Labels: map[string]string{
				annotations.QuotaExcludeLabel: annotations.QuotaExcludeValue,
			},
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

func scratchPVCSize(targetSize resource.Quantity) resource.Quantity {
	size := targetSize.DeepCopy()
	minOverhead := resource.MustParse("256Mi")
	overhead := *resource.NewQuantity(targetSize.Value()/10, targetSize.Format)
	if overhead.Cmp(minOverhead) < 0 {
		overhead = minOverhead
	}
	size.Add(overhead)
	return size
}

func (s DiskService) makePVCImporterPod(podName string, target *corev1.PersistentVolumeClaim, source *PVCImportSource, sourceClaim *corev1.PersistentVolumeClaim, scratchName string, nodePlacement *provisioner.NodePlacement) *corev1.Pod {
	podAnnotations := map[string]string{annotations.AnnPVCImportCreatedBy: "yes"}
	target.Annotations[annotations.AnnPVCImportPod] = podName

	container := corev1.Container{
		Name:            "d8v-cdi-importer",
		Image:           s.diskImporterImage,
		ImagePullPolicy: corev1.PullPolicy(s.pullPolicy),
		Command:         []string{"/usr/bin/cdi-importer"},
		Args:            []string{"-v=" + s.verbose},
		Env: []corev1.EnvVar{
			{Name: common.ImporterSource, Value: sourceRegistry},
			{Name: common.ImporterEndpoint, Value: target.Annotations[annotations.AnnPVCImportEndpoint]},
			{Name: common.ImporterContentType, Value: "kubevirt"},
			{Name: common.ImporterImageSize, Value: target.Annotations[annotations.AnnPVCImportImageSize]},
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
	if secretName := target.Annotations[annotations.AnnPVCImportSecret]; secretName != "" {
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
	if target.Annotations[annotations.AnnPVCImportCertConfigMap] != "" {
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
	if certConfigMap := target.Annotations[annotations.AnnPVCImportCertConfigMap]; certConfigMap != "" {
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

		container.Command = []string{"/usr/bin/qemu-img"}
		container.Args = []string{"convert", "-p", "-O", "raw", sourcePath, targetPath}
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
			Name:        podName,
			Namespace:   target.Namespace,
			Annotations: podAnnotations,
			Labels: map[string]string{
				annotations.QuotaExcludeLabel: annotations.QuotaExcludeValue,
			},
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
	copy.Annotations[annotations.AnnPVCImportPhase] = string(phase)
	return s.client.Patch(ctx, copy, client.MergeFrom(target))
}

func (s DiskService) cleanupPVCImport(ctx context.Context, sup supplements.Generator, target *corev1.PersistentVolumeClaim) (bool, error) {
	var deleted bool
	podName := target.Annotations[annotations.AnnPVCImportPod]
	if podName == "" {
		podName = sup.PVCImporterPod().Name
	}
	for _, obj := range []client.Object{
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: target.Namespace}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: target.Name + "-scratch", Namespace: target.Namespace}},
	} {
		err := s.client.Delete(ctx, obj)
		switch {
		case err == nil:
			deleted = true
		case !k8serrors.IsNotFound(err):
			return false, err
		}
	}
	return deleted, nil
}

func ownerReferenceForObject(obj client.Object) metav1.OwnerReference {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return metav1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               obj.GetName(),
		UID:                obj.GetUID(),
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}
}

func (s DiskService) startPVCClone(ctx context.Context, pvcSize resource.Quantity, sc *storagev1.StorageClass, source *PVCImportSourcePVC, vd *v1alpha2.VirtualDisk, nodePlacement *provisioner.NodePlacement) error {
	sourceClaim, err := object.FetchObject(ctx, types.NamespacedName{Name: source.Name, Namespace: source.Namespace}, s.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return fmt.Errorf("fetch source pvc: %w", err)
	}
	if sourceClaim == nil {
		return fmt.Errorf("source pvc %s/%s not found", source.Namespace, source.Name)
	}

	volumeMode, accessMode, err := s.GetVolumeAndAccessModes(ctx, vd, sc)
	if err != nil {
		return fmt.Errorf("get volume and access modes: %w", err)
	}

	strategy := s.choosePVCCloneStrategy(ctx, sourceClaim, sc, volumeMode)
	target := s.makePVCCloneTarget(pvcSize, sc, accessMode, volumeMode, sourceClaim, vd, strategy)
	if nodePlacement != nil {
		if err := provisioner.KeepNodePlacementTolerations(nodePlacement, target); err != nil {
			return fmt.Errorf("keep node placement: %w", err)
		}
	}

	if strategy == cloneStrategySnapshot {
		if err := s.ensureCloneSnapshot(ctx, sourceClaim, target, vd); err != nil {
			return err
		}
	}

	err = s.client.Create(ctx, target)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (s DiskService) choosePVCCloneStrategy(ctx context.Context, sourceClaim *corev1.PersistentVolumeClaim, targetSC *storagev1.StorageClass, targetVolumeMode corev1.PersistentVolumeMode) string {
	sourceSC, err := s.getPVCStorageClass(ctx, sourceClaim)
	if err != nil || sourceSC == nil {
		return cloneStrategyHost
	}

	preferred := cloneStrategySnapshot
	if sp, err := object.FetchObject(ctx, types.NamespacedName{Name: targetSC.Name}, s.client, &cdiv1.StorageProfile{}); err == nil && sp != nil && sp.Status.CloneStrategy != nil {
		switch *sp.Status.CloneStrategy {
		case cdiv1.CloneStrategyCsiClone:
			preferred = cloneStrategyCSI
		case cdiv1.CloneStrategyHostAssisted:
			preferred = cloneStrategyHost
		case cdiv1.CloneStrategySnapshot:
			preferred = cloneStrategySnapshot
		}
	}

	if preferred == cloneStrategySnapshot && s.canSnapshotClone(ctx, sourceClaim, sourceSC, targetSC, targetVolumeMode) {
		return cloneStrategySnapshot
	}
	if preferred != cloneStrategyHost && s.canCSIClone(sourceClaim, sourceSC, targetSC, targetVolumeMode) {
		return cloneStrategyCSI
	}
	if preferred == cloneStrategyCSI && s.canSnapshotClone(ctx, sourceClaim, sourceSC, targetSC, targetVolumeMode) {
		return cloneStrategySnapshot
	}
	return cloneStrategyHost
}

func (s DiskService) makePVCCloneTarget(pvcSize resource.Quantity, sc *storagev1.StorageClass, accessMode corev1.PersistentVolumeAccessMode, volumeMode corev1.PersistentVolumeMode, sourceClaim *corev1.PersistentVolumeClaim, vd *v1alpha2.VirtualDisk, strategy string) *corev1.PersistentVolumeClaim {
	pvcSize = pvcCloneTargetSize(pvcSize, sourceClaim)
	pvcAnnotations := map[string]string{
		annotations.AnnPVCImportSource:        sourcePVC,
		annotations.AnnPVCImportEndpoint:      sourceClaim.Namespace + "/" + sourceClaim.Name,
		annotations.AnnPVCImportCloneStrategy: strategy,
		annotations.AnnPVCImportImageSize:     pvcSize.String(),
		annotations.AnnPVCImportPhase:         string(corev1.PodPending),
		annotations.AnnPVCImportPod:           "",
	}

	target := &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{Kind: "PersistentVolumeClaim", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        vd.Status.Target.PersistentVolumeClaim,
			Namespace:   vd.Namespace,
			Annotations: pvcAnnotations,
			Finalizers:  s.diskProtectionFinalizers(),
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

	switch strategy {
	case cloneStrategySnapshot:
		snapshotName := target.Name + "-clone-snapshot"
		target.Annotations[annotations.AnnPVCImportCloneSnapshot] = snapshotName
		target.Spec.DataSource = &corev1.TypedLocalObjectReference{
			APIGroup: ptr.To("snapshot.storage.k8s.io"),
			Kind:     "VolumeSnapshot",
			Name:     snapshotName,
		}
		target.Spec.DataSourceRef = &corev1.TypedObjectReference{
			APIGroup: ptr.To("snapshot.storage.k8s.io"),
			Kind:     "VolumeSnapshot",
			Name:     snapshotName,
		}
	case cloneStrategyCSI:
		target.Spec.DataSource = &corev1.TypedLocalObjectReference{
			Kind: "PersistentVolumeClaim",
			Name: sourceClaim.Name,
		}
		target.Spec.DataSourceRef = &corev1.TypedObjectReference{
			Kind: "PersistentVolumeClaim",
			Name: sourceClaim.Name,
		}
	}
	return target
}

func pvcCloneTargetSize(requested resource.Quantity, sourceClaim *corev1.PersistentVolumeClaim) resource.Quantity {
	size := requested.DeepCopy()
	for _, candidate := range []resource.Quantity{
		sourceClaim.Spec.Resources.Requests[corev1.ResourceStorage],
		sourceClaim.Status.Capacity[corev1.ResourceStorage],
	} {
		if !candidate.IsZero() && size.Cmp(candidate) < 0 {
			size = candidate.DeepCopy()
		}
	}
	return size
}

func (s DiskService) ensureCloneSnapshot(ctx context.Context, sourceClaim, target *corev1.PersistentVolumeClaim, vd *v1alpha2.VirtualDisk) error {
	snapshotName := target.Annotations[annotations.AnnPVCImportCloneSnapshot]
	if snapshotName == "" {
		return fmt.Errorf("clone snapshot annotation is empty")
	}
	existing, err := object.FetchObject(ctx, types.NamespacedName{Name: snapshotName, Namespace: target.Namespace}, s.client, &vsv1.VolumeSnapshot{})
	if err != nil {
		return fmt.Errorf("fetch clone snapshot: %w", err)
	}
	if existing != nil {
		return nil
	}

	sourceSC, err := s.getPVCStorageClass(ctx, sourceClaim)
	if err != nil {
		return err
	}
	snapshotClass := s.snapshotClassForProvisioner(ctx, sourceSC.Provisioner)
	if snapshotClass == "" {
		return fmt.Errorf("no compatible VolumeSnapshotClass found for provisioner %q", sourceSC.Provisioner)
	}

	vs := &vsv1.VolumeSnapshot{
		TypeMeta: metav1.TypeMeta{Kind: "VolumeSnapshot", APIVersion: "snapshot.storage.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      snapshotName,
			Namespace: target.Namespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         v1alpha2.SchemeGroupVersion.String(),
				Kind:               v1alpha2.VirtualDiskKind,
				Name:               vd.Name,
				UID:                vd.UID,
				Controller:         ptr.To(false),
				BlockOwnerDeletion: ptr.To(true),
			}},
		},
		Spec: vsv1.VolumeSnapshotSpec{
			Source: vsv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: ptr.To(sourceClaim.Name),
			},
			VolumeSnapshotClassName: ptr.To(snapshotClass),
		},
	}
	if err := s.client.Create(ctx, vs); err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create clone snapshot: %w", err)
	}
	return nil
}

func (s DiskService) canSnapshotClone(ctx context.Context, sourceClaim *corev1.PersistentVolumeClaim, sourceSC, targetSC *storagev1.StorageClass, targetVolumeMode corev1.PersistentVolumeMode) bool {
	return sourceSC.Provisioner == targetSC.Provisioner &&
		volumeModesEqual(sourceClaim, targetVolumeMode) &&
		s.snapshotClassForProvisioner(ctx, sourceSC.Provisioner) != ""
}

func (s DiskService) canCSIClone(sourceClaim *corev1.PersistentVolumeClaim, sourceSC, targetSC *storagev1.StorageClass, targetVolumeMode corev1.PersistentVolumeMode) bool {
	return sourceClaim.Namespace != "" &&
		sourceSC.Provisioner == targetSC.Provisioner &&
		volumeModesEqual(sourceClaim, targetVolumeMode)
}

func (s DiskService) getPVCStorageClass(ctx context.Context, claim *corev1.PersistentVolumeClaim) (*storagev1.StorageClass, error) {
	if claim.Spec.StorageClassName == nil || *claim.Spec.StorageClassName == "" {
		return nil, fmt.Errorf("source pvc %s/%s has no storageClassName", claim.Namespace, claim.Name)
	}
	sc, err := object.FetchObject(ctx, types.NamespacedName{Name: *claim.Spec.StorageClassName}, s.client, &storagev1.StorageClass{})
	if err != nil {
		return nil, fmt.Errorf("fetch source storage class: %w", err)
	}
	if sc == nil {
		return nil, fmt.Errorf("source storage class %q not found", *claim.Spec.StorageClassName)
	}
	return sc, nil
}

func (s DiskService) snapshotClassForProvisioner(ctx context.Context, provisioner string) string {
	var list vsv1.VolumeSnapshotClassList
	if err := s.client.List(ctx, &list); err != nil {
		return ""
	}
	for _, item := range list.Items {
		if item.Driver == provisioner {
			return item.Name
		}
	}
	return ""
}

func volumeModesEqual(sourceClaim *corev1.PersistentVolumeClaim, targetVolumeMode corev1.PersistentVolumeMode) bool {
	sourceMode := corev1.PersistentVolumeFilesystem
	if sourceClaim.Spec.VolumeMode != nil {
		sourceMode = *sourceClaim.Spec.VolumeMode
	}
	return sourceMode == targetVolumeMode
}

func isSmartCloneStrategy(strategy string) bool {
	return strategy == cloneStrategySnapshot || strategy == cloneStrategyCSI
}

func (s DiskService) cleanupPVCImportClone(ctx context.Context, target *corev1.PersistentVolumeClaim) error {
	if target.Annotations[annotations.AnnPVCImportCloneStrategy] != cloneStrategySnapshot {
		return nil
	}
	snapshotName := target.Annotations[annotations.AnnPVCImportCloneSnapshot]
	if snapshotName == "" {
		return nil
	}
	err := s.client.Delete(ctx, &vsv1.VolumeSnapshot{ObjectMeta: metav1.ObjectMeta{Name: snapshotName, Namespace: target.Namespace}})
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}
	return nil
}
