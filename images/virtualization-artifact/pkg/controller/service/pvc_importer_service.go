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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements/copier"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
)

const (
	cdiDataVolName     = "cdi-data-vol"
	cdiScratchVolName  = "cdi-scratch-vol"
	cdiImporterDataDir = "/data"
	cdiScratchDataDir  = "/scratch"
	cdiWriteBlockPath  = "/dev/cdi-block-volume"
	cdiSourceBlockPath = "/dev/source-block-volume"
	sourceRegistry     = "registry"
	sourcePVC          = "pvc"
)

// PVCImporterService drives the cdi-importer pod that fills a target PVC with
// data fetched from a registry (DVCR) or another PVC. It owns the scratch PVC
// and the cdi-importer pod and is intentionally agnostic of VirtualDisk and
// VirtualImage; callers pass the target PVC, a PVCImportSource, an owner
// client.Object (used for OwnerReferences on the supplemental secret/configmap
// copies) and a supplements.Generator that yields stable names for the helper
// resources.
//
// Callers create the target PVC themselves (with their own owner reference and
// finalizer) and then ask PVCImporterService to perform the rest of the
// import: copy DVCR auth/cert supplements, ensure the scratch PVC, ensure the
// cdi-importer pod, and clean up the helper resources when the import has
// finished.
type PVCImporterService struct {
	client               client.Client
	dvcrSettings         *dvcr.Settings
	image                string
	resourceRequirements corev1.ResourceRequirements
	pullPolicy           string
	verbose              string
}

// NewPVCImporterService returns a PVCImporterService configured with the
// cdi-importer pod settings (image, resources, pull policy, verbosity) and
// the DVCR settings used to derive auth/CA supplements.
func NewPVCImporterService(
	c client.Client,
	dvcrSettings *dvcr.Settings,
	image string,
	resourceRequirements corev1.ResourceRequirements,
	pullPolicy string,
	verbose string,
) *PVCImporterService {
	return &PVCImporterService{
		client:               c,
		dvcrSettings:         dvcrSettings,
		image:                image,
		resourceRequirements: resourceRequirements,
		pullPolicy:           pullPolicy,
		verbose:              verbose,
	}
}

// Ensure runs one reconciliation step of the import: it makes sure the helper
// secret/configmap copies exist, the scratch PVC exists, and the cdi-importer
// pod has been created. The returned phase mirrors the current cdi-importer
// pod phase; once it is corev1.PodSucceeded the helper resources are torn
// down automatically.
//
// The caller is responsible for creating the target PVC up front; Ensure does
// not validate ownership or finalizers on it.
func (s *PVCImporterService) Ensure(ctx context.Context, target *corev1.PersistentVolumeClaim, source *PVCImportSource, owner client.Object, sup supplements.Generator, nodePlacement *provisioner.NodePlacement) (corev1.PodPhase, error) {
	phase := corev1.PodPhase(target.Annotations[annotations.AnnPVCImportPhase])
	if phase == corev1.PodSucceeded {
		_, err := s.CleanUp(ctx, sup, target)
		return phase, err
	}

	if err := s.ensureSupplements(ctx, target, owner, sup); err != nil {
		return "", err
	}

	scratch, err := s.ensureScratch(ctx, target)
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
		pod = s.makeImporterPod(podKey.Name, target, source, sourceClaim, scratch.Name, nodePlacement)
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
		_, err := s.CleanUp(ctx, sup, target)
		return pod.Status.Phase, err
	}
	return pod.Status.Phase, nil
}

// CleanUp removes the cdi-importer pod and the scratch PVC associated with
// the target PVC. The pod name is taken from target's annotation when
// available and falls back to the generator-issued name. CleanUp is
// idempotent: missing resources are ignored.
func (s *PVCImporterService) CleanUp(ctx context.Context, sup supplements.Generator, target *corev1.PersistentVolumeClaim) (bool, error) {
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

// PVCImportAnnotations builds the annotation map used at target PVC creation
// time to describe how the cdi-importer pod should populate it. The source
// can be a DVCR registry image (Upload/HTTP/Registry/ObjectRef CVI/VI) or
// another PVC (clone from a VirtualDisk).
func (s *PVCImporterService) PVCImportAnnotations(source *PVCImportSource, size resource.Quantity) map[string]string {
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

// ensureSupplements copies the DVCR auth secret and CA bundle into the
// target's namespace under stable supplemental names, owned by target so
// they get garbage-collected together with it.
func (s *PVCImporterService) ensureSupplements(ctx context.Context, target *corev1.PersistentVolumeClaim, _ client.Object, supGen supplements.Generator) error {
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

// ensureScratch makes sure the scratch PVC exists; it is created sized as the
// target plus a small overhead and owned by target so it is garbage-collected
// once the import finishes.
func (s *PVCImporterService) ensureScratch(ctx context.Context, target *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
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

// makeImporterPod builds the cdi-importer pod descriptor. The pod is owned by
// the target PVC and labelled to be excluded from namespace quota accounting.
func (s *PVCImporterService) makeImporterPod(podName string, target *corev1.PersistentVolumeClaim, source *PVCImportSource, sourceClaim *corev1.PersistentVolumeClaim, scratchName string, nodePlacement *provisioner.NodePlacement) *corev1.Pod {
	podAnnotations := map[string]string{annotations.AnnPVCImportCreatedBy: "yes"}
	target.Annotations[annotations.AnnPVCImportPod] = podName

	container := corev1.Container{
		Name:            "d8v-cdi-importer",
		Image:           s.image,
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

// patchTargetImportPhase mirrors the cdi-importer pod phase onto the target
// PVC's annotation so external observers and other reconciliations can read
// the import progress without having to look up the helper pod.
func (s *PVCImporterService) patchTargetImportPhase(ctx context.Context, target *corev1.PersistentVolumeClaim, phase corev1.PodPhase) error {
	copy := target.DeepCopy()
	if copy.Annotations == nil {
		copy.Annotations = map[string]string{}
	}
	copy.Annotations[annotations.AnnPVCImportPhase] = string(phase)
	return s.client.Patch(ctx, copy, client.MergeFrom(target))
}
