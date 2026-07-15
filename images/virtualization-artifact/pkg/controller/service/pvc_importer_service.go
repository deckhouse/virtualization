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
	"strconv"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	networkpolicy "github.com/deckhouse/virtualization-controller/pkg/common/network_policy"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements/copier"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr/registrytoken"
)

const (
	pvcImporterDataVolName     = "pvc-importer-data-vol"
	pvcImporterScratchVolName  = "pvc-importer-scratch-vol"
	pvcImporterSourceVolName   = "pvc-importer-source-vol"
	pvcImporterDataDir         = "/data"
	pvcImporterScratchDataDir  = "/scratch"
	pvcImporterWriteBlockPath  = "/dev/pvc-importer-block-volume"
	pvcImporterSourceBlockPath = "/dev/source-block-volume"
	pvcImporterNBDPort         = 10809
	sourceRegistry             = "registry"
)

// PVCImporterService drives the pvc-importer pod that fills a target PVC with
// data fetched from a registry (DVCR) or another PVC. It owns the scratch PVC
// and the pvc-importer pod and is intentionally agnostic of VirtualDisk and
// VirtualImage; callers pass the target PVC, a PVCImportSource, an owner
// client.Object (used for OwnerReferences on the supplemental secret/configmap
// copies) and a supplements.Generator that yields stable names for the helper
// resources.
//
// Callers create the target PVC themselves (with their own owner reference and
// finalizer) and then ask PVCImporterService to perform the rest of the
// import: copy DVCR auth/cert supplements, ensure the scratch PVC, ensure the
// pvc-importer pod, and clean up the helper resources when the import has
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
// pvc-importer pod settings (image, resources, pull policy, verbosity) and
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

// Import starts the import: it makes sure the helper
// secret/configmap copies exist, the scratch PVC exists, and the pvc-importer
// pod has been created.
//
// The caller is responsible for creating the target PVC up front; Import does
// not validate ownership or finalizers on it.
func (s *PVCImporterService) Import(ctx context.Context, target *corev1.PersistentVolumeClaim, source *PVCImportSource, owner client.Object, sup supplements.Generator, nodePlacement *provisioner.NodePlacement) error {
	if target.Annotations[annotations.AnnPVCPopulationDone] == "true" {
		return nil
	}

	if source != nil && source.PVC != nil {
		return s.importFromPVC(ctx, target, source, owner, sup, nodePlacement)
	}

	if err := s.ensureSupplements(ctx, target, source, sup); err != nil {
		return err
	}

	// The importer fills a dedicated prime PVC instead of the target. This keeps the
	// target PVC untouched until the import has finished and its populated volume is
	// rebound to the target (see Wait/Rebind), so the importer pod and the consuming
	// VirtualMachine never contend for the same ReadWriteOnce volume.
	prime, err := s.ensurePrime(ctx, target, nodePlacement)
	if err != nil {
		return err
	}

	scratch, err := s.ensureScratch(ctx, prime)
	if err != nil {
		return err
	}

	// The pvc-importer pod must be allowed egress (notably to DVCR) even inside
	// network-isolated namespaces (e.g. Deckhouse Projects with networkPolicy: Isolated).
	if err := s.ensureNetworkPolicy(ctx, target, sup); err != nil {
		return err
	}

	var sourceClaim *corev1.PersistentVolumeClaim
	if source != nil && source.PVC != nil {
		sourceClaim, err = object.FetchObject(ctx, types.NamespacedName{Name: source.PVC.Name, Namespace: source.PVC.Namespace}, s.client, &corev1.PersistentVolumeClaim{})
		if err != nil {
			return fmt.Errorf("fetch source pvc: %w", err)
		}
		if sourceClaim == nil {
			return fmt.Errorf("source pvc %s/%s not found", source.PVC.Namespace, source.PVC.Name)
		}
	}

	// The importer pod CPU limit and qemu-img convert parallelism are sized to the
	// consuming VM's CPU count for WaitForFirstConsumer volumes, and to 1 for
	// Immediate ones.
	wffc, err := s.isWaitForFirstConsumer(ctx, target)
	if err != nil {
		return fmt.Errorf("determine volume binding mode: %w", err)
	}

	podKey := sup.PVCImporterPod()
	pod, err := object.FetchObject(ctx, podKey, s.client, &corev1.Pod{})
	if err != nil {
		return fmt.Errorf("fetch importer pod: %w", err)
	}
	if pod == nil {
		pod = s.makeImporterPod(podKey.Name, target, prime, owner.GetUID(), source, sourceClaim, scratch.Name, nodePlacement, wffc)
		if err := s.client.Create(ctx, pod); err != nil && !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("create importer pod: %w", err)
		}
	}
	return nil
}

func (s *PVCImporterService) importFromPVC(ctx context.Context, target *corev1.PersistentVolumeClaim, source *PVCImportSource, owner client.Object, sup supplements.Generator, nodePlacement *provisioner.NodePlacement) error {
	sourceClaim, err := object.FetchObject(ctx, types.NamespacedName{Name: source.PVC.Name, Namespace: source.PVC.Namespace}, s.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return fmt.Errorf("fetch source pvc: %w", err)
	}
	if sourceClaim == nil {
		return fmt.Errorf("source pvc %s/%s not found", source.PVC.Namespace, source.PVC.Name)
	}

	prime, err := s.ensurePrime(ctx, target, nodePlacement)
	if err != nil {
		return err
	}

	if err := s.ensureNetworkPolicy(ctx, target, sup); err != nil {
		return err
	}
	if err := s.ensureSourceImporterService(ctx, target, sup); err != nil {
		return err
	}

	sourceNBDHost := sup.PVCSourceImporterService().Name

	sourcePodKey := sup.PVCSourceImporterPod()
	sourcePod, err := object.FetchObject(ctx, sourcePodKey, s.client, &corev1.Pod{})
	if err != nil {
		return fmt.Errorf("fetch source importer pod: %w", err)
	}
	if sourcePod == nil {
		sourcePod = s.makeSourceImporterPod(sourcePodKey.Name, target, sourceClaim)
		if err := s.client.Create(ctx, sourcePod); err != nil && !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("create source importer pod: %w", err)
		}
	} else if sourcePod.Status.Phase == corev1.PodFailed {
		return nil
	}

	targetPodKey := sup.PVCTargetImporterPod()
	targetPod, err := object.FetchObject(ctx, targetPodKey, s.client, &corev1.Pod{})
	if err != nil {
		return fmt.Errorf("fetch target importer pod: %w", err)
	}
	if targetPod == nil {
		targetPod = s.makeTargetImporterPod(targetPodKey.Name, target, prime, owner.GetUID(), sourceNBDHost, nodePlacement)
		if err := s.client.Create(ctx, targetPod); err != nil && !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("create target importer pod: %w", err)
		}
	}
	return nil
}

// primePVCName returns the name of the prime PVC that the importer fills for the
// given target PVC.
func primePVCName(target *corev1.PersistentVolumeClaim) string {
	return target.Name + "-prime"
}

// selectedNodeAnnotation is the standard annotation the scheduler sets on a
// WaitForFirstConsumer PVC to pin its provisioning to a node. The prime PVC and
// the importer pod are pinned to the same node as the target so the populated
// volume can later be rebound to the target without a cross-node move.
const selectedNodeAnnotation = "volume.kubernetes.io/selected-node"

// SelectedNodeAnnotation is the scheduler annotation that pins a WFFC PVC to a node.
const SelectedNodeAnnotation = selectedNodeAnnotation

// ensurePrime creates the prime PVC the importer writes into. The prime mirrors
// the target's storage spec (storage class, size, access/volume modes) but never
// carries the target's data source: it is filled by the pvc-importer pod and its
// volume is later rebound to the target. The prime is owned by the target so it
// is garbage-collected if the disk is removed before the import finishes, and is
// excluded from namespace quota accounting.
func (s *PVCImporterService) ensurePrime(ctx context.Context, target *corev1.PersistentVolumeClaim, nodePlacement *provisioner.NodePlacement) (*corev1.PersistentVolumeClaim, error) {
	name := primePVCName(target)
	prime, err := object.FetchObject(ctx, types.NamespacedName{Name: name, Namespace: target.Namespace}, s.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return nil, fmt.Errorf("fetch prime pvc: %w", err)
	}
	if prime != nil {
		return prime, nil
	}

	prime = &corev1.PersistentVolumeClaim{
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
		Spec: *target.Spec.DeepCopy(),
	}
	prime.Spec.VolumeName = ""
	prime.Spec.DataSource = nil
	prime.Spec.DataSourceRef = nil

	// Pin the prime to the consuming VirtualMachine's node so the populated volume is
	// local to that node and can later be rebound to the target and attached to the VM
	// without a cross-node move. The VM's node (nodePlacement.Node) is preferred because
	// a VM's disk is hotplug-attached and never sets the target PVC's selected-node
	// annotation itself; the target's selected-node is used only as a fallback.
	selectedNode := target.Annotations[selectedNodeAnnotation]
	if nodePlacement != nil && nodePlacement.Node != "" {
		selectedNode = nodePlacement.Node
	}
	if selectedNode != "" {
		if prime.Annotations == nil {
			prime.Annotations = map[string]string{}
		}
		prime.Annotations[selectedNodeAnnotation] = selectedNode
	}

	if nodePlacement != nil {
		if err := provisioner.KeepNodePlacementTolerations(nodePlacement, prime); err != nil {
			return nil, fmt.Errorf("keep node placement on prime: %w", err)
		}
	}

	if err := s.client.Create(ctx, prime); err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("create prime pvc: %w", err)
	}
	return prime, nil
}

func (s *PVCImporterService) Wait(ctx context.Context, target *corev1.PersistentVolumeClaim, sup supplements.Generator) (corev1.PodPhase, error) {
	if target.Annotations[annotations.AnnPVCPopulationDone] == "true" {
		return corev1.PodSucceeded, nil
	}

	if target.Annotations[annotations.AnnPVCPopulationStrategy] == PopulationStrategyHostAssigned {
		return s.waitHostAssigned(ctx, target, sup)
	}

	pod, err := object.FetchObject(ctx, sup.PVCImporterPod(), s.client, &corev1.Pod{})
	if err != nil {
		return "", fmt.Errorf("fetch importer pod: %w", err)
	}
	if pod == nil {
		return corev1.PodPending, nil
	}
	if pod.Status.Phase == "" {
		return corev1.PodPending, nil
	}
	if pod.Status.Phase == corev1.PodSucceeded {
		// The prime PVC is populated; rebind its volume to the target. Rebind is
		// idempotent and resumable: it returns false until the target is Bound, so
		// the import is only reported Succeeded once the target carries the data.
		primeKey := types.NamespacedName{Name: primePVCName(target), Namespace: target.Namespace}
		done, err := Rebind(ctx, s.client, primeKey, client.ObjectKeyFromObject(target))
		if err != nil {
			return "", fmt.Errorf("rebind prime to target: %w", err)
		}
		if !done {
			return corev1.PodRunning, nil
		}
		_, err = s.CleanUp(ctx, sup, target)
		return corev1.PodSucceeded, err
	}
	return pod.Status.Phase, nil
}

func (s *PVCImporterService) waitHostAssigned(ctx context.Context, target *corev1.PersistentVolumeClaim, sup supplements.Generator) (corev1.PodPhase, error) {
	sourcePod, err := object.FetchObject(ctx, sup.PVCSourceImporterPod(), s.client, &corev1.Pod{})
	if err != nil {
		return "", fmt.Errorf("fetch source importer pod: %w", err)
	}
	if sourcePod != nil && sourcePod.Status.Phase == corev1.PodFailed {
		return corev1.PodFailed, nil
	}

	targetPod, err := object.FetchObject(ctx, sup.PVCTargetImporterPod(), s.client, &corev1.Pod{})
	if err != nil {
		return "", fmt.Errorf("fetch target importer pod: %w", err)
	}
	if targetPod == nil || targetPod.Status.Phase == "" {
		return corev1.PodPending, nil
	}
	if targetPod.Status.Phase != corev1.PodSucceeded {
		return targetPod.Status.Phase, nil
	}

	primeKey := types.NamespacedName{Name: primePVCName(target), Namespace: target.Namespace}
	done, err := Rebind(ctx, s.client, primeKey, client.ObjectKeyFromObject(target))
	if err != nil {
		return "", fmt.Errorf("rebind prime to target: %w", err)
	}
	if !done {
		return corev1.PodRunning, nil
	}
	_, err = s.CleanUp(ctx, sup, target)
	return corev1.PodSucceeded, err
}

// CleanUp removes the pvc-importer pod and the helper PVCs (prime and its
// scratch) associated with the target PVC. The prime PVC is normally deleted by
// Rebind once its volume has been transferred to the target; deleting it here is
// a safe, idempotent fallback for imports that were abandoned before completion.
// CleanUp is idempotent: missing resources are ignored.
func (s *PVCImporterService) CleanUp(ctx context.Context, sup supplements.Generator, target *corev1.PersistentVolumeClaim) (bool, error) {
	var deleted bool
	for _, obj := range []client.Object{
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: sup.PVCSourceImporterService().Name, Namespace: target.Namespace}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: sup.PVCImporterPod().Name, Namespace: target.Namespace}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: sup.PVCSourceImporterPod().Name, Namespace: target.Namespace}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: sup.PVCTargetImporterPod().Name, Namespace: target.Namespace}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: primePVCName(target) + "-scratch", Namespace: target.Namespace}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: primePVCName(target), Namespace: target.Namespace}},
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

// ensureSupplements mints a scoped DVCR auth secret and copies the CA bundle
// into the target's namespace under stable supplemental names, owned by target
// so they get garbage-collected together with it.
func (s *PVCImporterService) ensureSupplements(ctx context.Context, target *corev1.PersistentVolumeClaim, source *PVCImportSource, supGen supplements.Generator) error {
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

	// The pvc-importer only reads from DVCR (it writes to a PVC), so the token
	// is scoped to pull-only access on the source repository.
	if source != nil && source.Registry != nil && source.Registry.URL != "" {
		authCopier := copier.AuthSecret{
			Secret: copier.Secret{
				Destination:    supGen.DVCRAuthSecretForPVCImporter(),
				OwnerReference: ownerRef,
			},
		}
		scope := []registrytoken.Access{repoAccess(s.dvcrSettings.RepoPath(source.Registry.URL), "pull")}
		if err := authCopier.CreateScopedTokenCDI(ctx, s.client, s.dvcrSettings.TokenSigner, scope); err != nil {
			return fmt.Errorf("create scoped dvcr auth secret: %w", err)
		}
	}

	if s.dvcrSettings.CertsSecret != "" {
		caBundleCopier := copier.CABundleConfigMap{
			SourceSecret: types.NamespacedName{
				Name:      s.dvcrSettings.CertsSecret,
				Namespace: s.dvcrSettings.CertsSecretNamespace,
			},
			Destination:    supGen.DVCRCABundleConfigMapForPVCImporter(),
			OwnerReference: ownerRef,
		}
		if err := caBundleCopier.Copy(ctx, s.client); err != nil {
			return fmt.Errorf("copy dvcr ca bundle: %w", err)
		}
	}

	return nil
}

// ensureScratch makes sure the scratch PVC exists; it is created sized as the
// base PVC (the prime) plus a small overhead and owned by it so it is
// garbage-collected once the import finishes.
func (s *PVCImporterService) ensureScratch(ctx context.Context, base *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	name := base.Name + "-scratch"
	scratch, err := object.FetchObject(ctx, types.NamespacedName{Name: name, Namespace: base.Namespace}, s.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return nil, fmt.Errorf("fetch scratch pvc: %w", err)
	}
	if scratch != nil {
		return scratch, nil
	}

	size := scratchPVCSize(base.Spec.Resources.Requests[corev1.ResourceStorage])
	volumeMode := corev1.PersistentVolumeFilesystem
	scratch = &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{Kind: "PersistentVolumeClaim", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: base.Namespace,
			Labels: map[string]string{
				annotations.QuotaExcludeLabel: annotations.QuotaExcludeValue,
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         "v1",
				Kind:               "PersistentVolumeClaim",
				Name:               base.Name,
				UID:                base.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			}},
		},
		Spec: *base.Spec.DeepCopy(),
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

// ensureNetworkPolicy creates a NetworkPolicy that allows the pvc-importer pod egress
// (notably to DVCR) and ingress from sibling importer pods and from the
// virtualization-controller namespace (progress metrics scrape). It is required in
// network-isolated namespaces where a default-deny policy would otherwise block the
// import. The policy selects the importer pods by their app label, allows all egress,
// and is owned by the target PVC so it is garbage-collected with the disk. It is
// idempotent and coexists with the NetworkPolicy that ImporterService creates for
// DVCR-fed imports.
func (s *PVCImporterService) ensureNetworkPolicy(ctx context.Context, target *corev1.PersistentVolumeClaim, sup supplements.Generator) error {
	controllerNamespace := ""
	if s.dvcrSettings != nil {
		controllerNamespace = s.dvcrSettings.ControllerNamespace
	}
	npName := sup.NetworkPolicy()
	np := &netv1.NetworkPolicy{
		TypeMeta: metav1.TypeMeta{Kind: "NetworkPolicy", APIVersion: "networking.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      npName.Name,
			Namespace: npName.Namespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         "v1",
				Kind:               "PersistentVolumeClaim",
				Name:               target.Name,
				UID:                target.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			}},
		},
		Spec: netv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      annotations.AppLabel,
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{annotations.CDILabelValue, annotations.DVCRLabelValue},
				}},
			},
			Ingress: []netv1.NetworkPolicyIngressRule{{
				From: networkpolicy.PVCImporterIngressPeers(controllerNamespace),
			}},
			Egress:      []netv1.NetworkPolicyEgressRule{{}},
			PolicyTypes: []netv1.PolicyType{netv1.PolicyTypeIngress, netv1.PolicyTypeEgress},
		},
	}

	return client.IgnoreAlreadyExists(s.client.Create(ctx, np))
}

func (s *PVCImporterService) ensureSourceImporterService(ctx context.Context, target *corev1.PersistentVolumeClaim, sup supplements.Generator) error {
	svcKey := sup.PVCSourceImporterService()
	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcKey.Name,
			Namespace: svcKey.Namespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         "v1",
				Kind:               "PersistentVolumeClaim",
				Name:               target.Name,
				UID:                target.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			}},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				annotations.AppLabel:           annotations.CDILabelValue,
				annotations.PVCImportRoleLabel: annotations.PVCImportRoleSource,
			},
			Ports: []corev1.ServicePort{{
				Name:       "nbd",
				Port:       pvcImporterNBDPort,
				TargetPort: intstr.FromInt32(pvcImporterNBDPort),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
	return client.IgnoreAlreadyExists(s.client.Create(ctx, svc))
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

// isWaitForFirstConsumer reports whether the target PVC's storage class uses the
// WaitForFirstConsumer volume binding mode.
func (s *PVCImporterService) isWaitForFirstConsumer(ctx context.Context, pvc *corev1.PersistentVolumeClaim) (bool, error) {
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName == "" {
		return false, nil
	}
	sc, err := object.FetchObject(ctx, types.NamespacedName{Name: *pvc.Spec.StorageClassName}, s.client, &storagev1.StorageClass{})
	if err != nil {
		return false, fmt.Errorf("fetch storage class %q: %w", *pvc.Spec.StorageClassName, err)
	}
	if sc == nil || sc.VolumeBindingMode == nil {
		return false, nil
	}
	return *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer, nil
}

// importerConvertThreads sizes the importer pod CPU limit and the qemu-img convert
// parallelism (-m): for WaitForFirstConsumer volumes it is the consuming VM's CPU
// count capped at 16; for Immediate volumes (or when no single VM is attached) it is 1.
func importerConvertThreads(wffc bool, nodePlacement *provisioner.NodePlacement) int {
	if wffc && nodePlacement != nil && nodePlacement.CPUCores > 0 {
		if nodePlacement.CPUCores > 16 {
			return 16
		}
		return nodePlacement.CPUCores
	}
	return 1
}

// importerResources returns the pod resource requirements with the CPU limit set
// to cpuLimit cores, keeping the configured memory limit and requests.
func (s *PVCImporterService) importerResources(cpuLimit int) corev1.ResourceRequirements {
	resources := s.resourceRequirements.DeepCopy()
	if resources.Limits == nil {
		resources.Limits = corev1.ResourceList{}
	}
	resources.Limits[corev1.ResourceCPU] = *resource.NewQuantity(int64(cpuLimit), resource.DecimalSI)
	return *resources
}

// makeImporterPod builds the pvc-importer pod descriptor. It fills dataPVC (the
// prime PVC) with the imported data, while ownership/UID are taken from target
// (the VirtualDisk's PVC) so the pod is garbage-collected with the disk and
// labelled to be excluded from namespace quota accounting.
func (s *PVCImporterService) makeImporterPod(podName string, target, dataPVC *corev1.PersistentVolumeClaim, ownerUID types.UID, source *PVCImportSource, sourceClaim *corev1.PersistentVolumeClaim, scratchName string, nodePlacement *provisioner.NodePlacement, wffc bool) *corev1.Pod {
	registryEndpoint := ""
	if source != nil && source.Registry != nil {
		registryEndpoint = source.Registry.URL
	}
	imageSize := dataPVC.Spec.Resources.Requests[corev1.ResourceStorage]

	// The importer pod's CPU limit and the qemu-img convert parallelism (-m) are
	// sized to the consuming VirtualMachine's CPU count (capped at 16) for
	// WaitForFirstConsumer volumes, and pinned to 1 for Immediate ones (no VM
	// context is available at import time).
	convertThreads := importerConvertThreads(wffc, nodePlacement)

	container := corev1.Container{
		Name:            "d8v-pvc-importer",
		Image:           s.image,
		ImagePullPolicy: corev1.PullPolicy(s.pullPolicy),
		Command:         []string{"/usr/bin/pvc-importer"},
		Args:            []string{"-v=" + s.verbose},
		Env: []corev1.EnvVar{
			{Name: common.ImporterSource, Value: sourceRegistry},
			{Name: common.ImporterEndpoint, Value: registryEndpoint},
			{Name: common.ImporterContentType, Value: "kubevirt"},
			{Name: common.ImporterImageSize, Value: imageSize.String()},
			// The progress metric (kubevirt_cdi_import_progress_total) is labelled with
			// this ownerUID; it must be the VirtualDisk's UID so the controller's progress
			// scraper (which queries by vd.GetUID()) can match it. Using the target PVC UID
			// here would make progress appear stuck (jumping 0->100 / 50->100).
			{Name: common.OwnerUID, Value: string(ownerUID)},
			{Name: common.FilesystemOverheadVar, Value: "0"},
			{Name: common.InsecureTLSVar, Value: "false"},
			{Name: common.ImporterQemuConvertThreads, Value: strconv.Itoa(convertThreads)},
		},
		VolumeMounts: []corev1.VolumeMount{{Name: pvcImporterScratchVolName, MountPath: pvcImporterScratchDataDir}, {Name: "tmp", MountPath: "/tmp"}},
		Ports:        []corev1.ContainerPort{{Name: "metrics", ContainerPort: 8443, Protocol: corev1.ProtocolTCP}},
	}
	container.Resources = s.importerResources(convertThreads)
	if source != nil && source.Registry != nil && source.Registry.Secret != "" {
		secretName := source.Registry.Secret
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
	if source != nil && source.Registry != nil && source.Registry.CertConfigMap != "" {
		container.Env = append(container.Env, corev1.EnvVar{Name: common.ImporterCertDirVar, Value: common.ImporterCertDir})
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: "cert-vol", MountPath: common.ImporterCertDir})
	}
	if dataPVC.Spec.VolumeMode != nil && *dataPVC.Spec.VolumeMode == corev1.PersistentVolumeBlock {
		container.VolumeDevices = []corev1.VolumeDevice{{Name: pvcImporterDataVolName, DevicePath: pvcImporterWriteBlockPath}}
	} else {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: pvcImporterDataVolName, MountPath: pvcImporterDataDir})
	}

	volumes := []corev1.Volume{
		{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		{Name: pvcImporterDataVolName, VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: dataPVC.Name}}},
		{Name: pvcImporterScratchVolName, VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: scratchName}}},
	}
	if source != nil && source.Registry != nil && source.Registry.CertConfigMap != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "cert-vol",
			VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: source.Registry.CertConfigMap},
			}},
		})
	}
	if source != nil && source.PVC != nil && sourceClaim != nil {
		sourcePath := "/source/disk.img"
		if sourceClaim.Spec.VolumeMode != nil && *sourceClaim.Spec.VolumeMode == corev1.PersistentVolumeBlock {
			sourcePath = pvcImporterSourceBlockPath
			container.VolumeDevices = append(container.VolumeDevices, corev1.VolumeDevice{Name: "source-vol", DevicePath: pvcImporterSourceBlockPath})
		} else {
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: "source-vol", MountPath: "/source", ReadOnly: true})
		}

		targetPath := pvcImporterDataDir + "/disk.img"
		if dataPVC.Spec.VolumeMode != nil && *dataPVC.Spec.VolumeMode == corev1.PersistentVolumeBlock {
			targetPath = pvcImporterWriteBlockPath
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
			Name:      podName,
			Namespace: target.Namespace,
			Labels: map[string]string{
				annotations.QuotaExcludeLabel: annotations.QuotaExcludeValue,
				// Matches the importer NetworkPolicy selector so the pod is allowed
				// egress (to DVCR) in network-isolated namespaces.
				annotations.AppLabel: annotations.CDILabelValue,
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

	// Pin the importer to the node the prime volume is provisioned on so the pod can
	// mount it (ReadWriteOnce) and so the populated volume stays local to the
	// consuming VirtualMachine's node.
	if selectedNode := dataPVC.Annotations[selectedNodeAnnotation]; selectedNode != "" {
		pod.Spec.Affinity = &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{{
						MatchExpressions: []corev1.NodeSelectorRequirement{{
							Key:      corev1.LabelHostname,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{selectedNode},
						}},
					}},
				},
			},
		}
	}
	return pod
}

func (s *PVCImporterService) makeSourceImporterPod(podName string, target, sourceClaim *corev1.PersistentVolumeClaim) *corev1.Pod {
	sourcePath := "/source/disk.img"
	var volumeDevices []corev1.VolumeDevice
	volumeMounts := []corev1.VolumeMount{{Name: "tmp", MountPath: "/tmp"}}
	if sourceClaim.Spec.VolumeMode != nil && *sourceClaim.Spec.VolumeMode == corev1.PersistentVolumeBlock {
		sourcePath = pvcImporterSourceBlockPath
		volumeDevices = append(volumeDevices, corev1.VolumeDevice{Name: pvcImporterSourceVolName, DevicePath: pvcImporterSourceBlockPath})
	} else {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{Name: pvcImporterSourceVolName, MountPath: "/source", ReadOnly: true})
	}

	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: target.Namespace,
			Labels: map[string]string{
				annotations.QuotaExcludeLabel:  annotations.QuotaExcludeValue,
				annotations.AppLabel:           annotations.CDILabelValue,
				annotations.PVCImportRoleLabel: annotations.PVCImportRoleSource,
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
			Containers: []corev1.Container{{
				Name:            "d8v-pvc-source-importer",
				Image:           s.image,
				ImagePullPolicy: corev1.PullPolicy(s.pullPolicy),
				Command:         []string{"/usr/sbin/nbdkit"},
				Args:            []string{"-f", "-r", "-p", fmt.Sprintf("%d", pvcImporterNBDPort), "file", sourcePath},
				Ports:           []corev1.ContainerPort{{Name: "nbd", ContainerPort: pvcImporterNBDPort, Protocol: corev1.ProtocolTCP}},
				VolumeMounts:    volumeMounts,
				VolumeDevices:   volumeDevices,
			}},
			RestartPolicy: corev1.RestartPolicyOnFailure,
			Volumes: []corev1.Volume{
				{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
				{Name: pvcImporterSourceVolName, VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: sourceClaim.Name,
					ReadOnly:  true,
				}}},
			},
		},
	}
	if s.resourceRequirements.Requests != nil || s.resourceRequirements.Limits != nil {
		pod.Spec.Containers[0].Resources = s.resourceRequirements
	}
	podutil.SetRestrictedSecurityContext(&pod.Spec)
	return pod
}

func (s *PVCImporterService) makeTargetImporterPod(podName string, target, dataPVC *corev1.PersistentVolumeClaim, ownerUID types.UID, sourceNBDHost string, nodePlacement *provisioner.NodePlacement) *corev1.Pod {
	volumeMounts := []corev1.VolumeMount{{Name: "tmp", MountPath: "/tmp"}}
	var volumeDevices []corev1.VolumeDevice
	if dataPVC.Spec.VolumeMode != nil && *dataPVC.Spec.VolumeMode == corev1.PersistentVolumeBlock {
		volumeDevices = append(volumeDevices, corev1.VolumeDevice{Name: pvcImporterDataVolName, DevicePath: pvcImporterWriteBlockPath})
	} else {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{Name: pvcImporterDataVolName, MountPath: pvcImporterDataDir})
	}

	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: target.Namespace,
			Labels: map[string]string{
				annotations.QuotaExcludeLabel: annotations.QuotaExcludeValue,
				annotations.AppLabel:          annotations.CDILabelValue,
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
			Containers: []corev1.Container{{
				Name:            "d8v-pvc-target-importer",
				Image:           s.image,
				ImagePullPolicy: corev1.PullPolicy(s.pullPolicy),
				Command:         []string{"/usr/bin/pvc-target-importer"},
				Args:            []string{"-v=" + s.verbose},
				Env: []corev1.EnvVar{
					{Name: common.OwnerUID, Value: string(ownerUID)},
					{Name: common.ImporterNBDEndpoint, Value: fmt.Sprintf("nbd://%s:%d", sourceNBDHost, pvcImporterNBDPort)},
				},
				Ports:         []corev1.ContainerPort{{Name: "metrics", ContainerPort: 8443, Protocol: corev1.ProtocolTCP}},
				VolumeMounts:  volumeMounts,
				VolumeDevices: volumeDevices,
			}},
			RestartPolicy: corev1.RestartPolicyOnFailure,
			Volumes: []corev1.Volume{
				{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
				{Name: pvcImporterDataVolName, VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: dataPVC.Name}}},
			},
		},
	}
	if s.resourceRequirements.Requests != nil || s.resourceRequirements.Limits != nil {
		pod.Spec.Containers[0].Resources = s.resourceRequirements
	}
	if nodePlacement != nil {
		pod.Spec.Tolerations = nodePlacement.Tolerations
		_ = provisioner.KeepNodePlacementTolerations(nodePlacement, pod)
	}
	if selectedNode := dataPVC.Annotations[selectedNodeAnnotation]; selectedNode != "" {
		pod.Spec.Affinity = &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{{
						MatchExpressions: []corev1.NodeSelectorRequirement{{
							Key:      corev1.LabelHostname,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{selectedNode},
						}},
					}},
				},
			},
		}
	}
	podutil.SetRestrictedSecurityContext(&pod.Spec)
	return pod
}
