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

package populator

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	podobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/pod"
	pvcobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/pvc"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

const (
	populatorPVCSize       = "64Mi"
	populatorDVCRPVCSize   = "256Mi"
	populatorWaitTimeout   = 3 * time.Minute
	populatorPollInterval  = 2 * time.Second
	snapshotStorageAPI     = "snapshot.storage.k8s.io"
	populatorSourcePVCName = "source"

	populationStrategyCSIClone     = "csi-clone"
	populationStrategySnapshot     = "snapshot"
	populationStrategyHostAssigned = "host-assigned"
	populationStrategyDVCR         = "dvcr"
)

var _ = Describe("Populator", Label(precheck.PrecheckDefaultStorageClass, precheck.PrecheckSnapshot), func() {
	var (
		f  *framework.Framework
		sc string
	)

	BeforeEach(func() {
		f = framework.NewFramework("populator")
		f.Before()
		DeferCleanup(f.After)

		defaultSC := framework.GetConfig().StorageClass.DefaultStorageClass
		if defaultSC == nil {
			Skip("StorageClass is not configured")
		}
		sc = defaultSC.Name
	})

	It("creates target PVC from PVC using CSI clone", func(ctx SpecContext) {
		source := newPopulatorPVC(populatorSourcePVCName, f.Namespace().Name, sc, nil)
		target := newPopulatorPVC("target-csi-clone", f.Namespace().Name, sc, map[string]string{
			annotations.AnnPVCPopulationStrategy:  populationStrategyCSIClone,
			annotations.AnnPVCPopulationSourcePVC: source.Name,
		})
		target.Spec.DataSourceRef = &corev1.TypedObjectReference{Kind: "PersistentVolumeClaim", Name: source.Name}

		sourceObs := startPVCObserver(ctx, f, source)
		targetObs := startPVCObserver(ctx, f, target)
		Expect(f.CreateWithDeferredDeletion(ctx, source)).To(Succeed())
		bindSourcePVC(ctx, f, sourceObs, source.Name)
		Expect(f.CreateWithDeferredDeletion(ctx, target)).To(Succeed())
		// The CSI clone is performed by the provisioner, so on a WFFC StorageClass
		// it does not start until the target PVC gets its first consumer.
		bindTargetPVC(ctx, f, targetObs, target.Name)

		waitPVCBoundAndDone(targetObs)
		waitPopulatorCleanup(ctx, f, target.Name)
	}, SpecTimeout(populatorWaitTimeout))

	It("creates target PVC from PVC using snapshot", func(ctx SpecContext) {
		source := newPopulatorPVC(populatorSourcePVCName, f.Namespace().Name, sc, nil)
		snapshotName := "target-snapshot-" + rand.String(5)
		target := newPopulatorPVC("target-snapshot", f.Namespace().Name, sc, map[string]string{
			annotations.AnnPVCPopulationStrategy:  populationStrategySnapshot,
			annotations.AnnPVCPopulationSourcePVC: source.Name,
		})
		target.Spec.DataSource = &corev1.TypedLocalObjectReference{APIGroup: ptr.To(snapshotStorageAPI), Kind: "VolumeSnapshot", Name: snapshotName}
		target.Spec.DataSourceRef = &corev1.TypedObjectReference{APIGroup: ptr.To(snapshotStorageAPI), Kind: "VolumeSnapshot", Name: snapshotName}

		sourceObs := startPVCObserver(ctx, f, source)
		targetObs := startPVCObserver(ctx, f, target)
		Expect(f.CreateWithDeferredDeletion(ctx, source)).To(Succeed())
		bindSourcePVC(ctx, f, sourceObs, source.Name)
		Expect(f.CreateWithDeferredDeletion(ctx, target)).To(Succeed())
		// Restoring from the VolumeSnapshot is performed by the provisioner, so on
		// a WFFC StorageClass it does not start until the target PVC gets its
		// first consumer.
		bindTargetPVC(ctx, f, targetObs, target.Name)

		waitPVCBoundAndDone(targetObs)
		waitPopulatorCleanup(ctx, f, target.Name)
	}, SpecTimeout(populatorWaitTimeout))

	It("creates target PVC from PVC using host assigned population", func(ctx SpecContext) {
		source := newPopulatorPVC(populatorSourcePVCName, f.Namespace().Name, sc, nil)
		target := newPopulatorPVC("target-host-assigned", f.Namespace().Name, sc, map[string]string{
			annotations.AnnPVCPopulationStrategy:  populationStrategyHostAssigned,
			annotations.AnnPVCPopulationSourcePVC: source.Name,
		})
		target.Spec.DataSourceRef = &corev1.TypedObjectReference{
			APIGroup: ptr.To("virtualization.deckhouse.io"),
			Kind:     "PersistentVolumeClaim",
			Name:     source.Name,
		}

		sourceObs := startPVCObserver(ctx, f, source)
		targetObs := startPVCObserver(ctx, f, target)
		Expect(f.CreateWithDeferredDeletion(ctx, source)).To(Succeed())
		// The writer pod is the source's first consumer, so it also lets the
		// PVC bind on a WaitForFirstConsumer StorageClass.
		writeRawDiskImage(ctx, f, source.Name)
		waitPVCBound(sourceObs)
		Expect(f.CreateWithDeferredDeletion(ctx, target)).To(Succeed())

		waitPVCBoundAndDone(targetObs)
		waitPopulatorCleanup(ctx, f, target.Name)
	}, SpecTimeout(populatorWaitTimeout))

	It("creates target PVC from DVCR", func(ctx SpecContext) {
		cvi := &v1alpha2.ClusterVirtualImage{}
		Expect(f.GenericClient().Get(ctx, crclient.ObjectKey{Name: object.PrecreatedCVIAlpineBIOS}, cvi)).To(Succeed())
		Expect(cvi.Status.Target.RegistryURL).NotTo(BeEmpty())

		target := newPopulatorPVCWithSize("target-dvcr", f.Namespace().Name, sc, populatorDVCRPVCSize, map[string]string{
			annotations.AnnPVCPopulationStrategy:   populationStrategyDVCR,
			annotations.AnnPVCPopulationSourceDVCR: "docker://" + cvi.Status.Target.RegistryURL,
		})
		target.Spec.DataSourceRef = &corev1.TypedObjectReference{
			APIGroup: ptr.To("virtualization.deckhouse.io"),
			Kind:     "ClusterVirtualImage",
			Name:     cvi.Name,
		}
		targetObs := startPVCObserver(ctx, f, target)
		Expect(f.CreateWithDeferredDeletion(ctx, target)).To(Succeed())

		waitPVCBoundAndDone(targetObs)
		waitPopulatorCleanup(ctx, f, target.Name)
	}, SpecTimeout(populatorWaitTimeout))
})

func newPopulatorPVC(name, namespace, storageClass string, anns map[string]string) *corev1.PersistentVolumeClaim {
	return newPopulatorPVCWithSize(name, namespace, storageClass, populatorPVCSize, anns)
}

func newPopulatorPVCWithSize(name, namespace, storageClass, size string, anns map[string]string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{Kind: "PersistentVolumeClaim", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: anns,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: ptr.To(storageClass),
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			VolumeMode:       ptr.To(corev1.PersistentVolumeFilesystem),
			Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(size),
			}},
		},
	}
}

func startPVCObserver(ctx context.Context, f *framework.Framework, pvc *corev1.PersistentVolumeClaim) pvcobs.Observer {
	GinkgoHelper()
	obs := pvcobs.StartObserver(ctx, f, pvc)
	obs.Never(pvcobs.BeLost())
	return obs
}

func waitPVCBoundAndDone(obs pvcobs.Observer) {
	GinkgoHelper()
	err := obs.WaitFor(pvcobs.BeBoundAndPopulated(), populatorWaitTimeout)
	Expect(err).NotTo(HaveOccurred())
}

func waitPVCBound(obs pvcobs.Observer) {
	GinkgoHelper()
	err := obs.WaitFor(pvcobs.BeBound(), populatorWaitTimeout)
	Expect(err).NotTo(HaveOccurred())
}

func waitPopulatorCleanup(ctx context.Context, f *framework.Framework, targetName string) {
	GinkgoHelper()
	target := &corev1.PersistentVolumeClaim{}
	Expect(f.GenericClient().Get(ctx, crclient.ObjectKey{Name: targetName, Namespace: f.Namespace().Name}, target)).To(Succeed())
	podNames := []string{
		"d8v-pvc-pvc-importer-" + string(target.UID),
		"d8v-pvc-pvc-source-importer-" + string(target.UID),
		"d8v-pvc-pvc-target-importer-" + string(target.UID),
	}

	err := wait.PollUntilContextTimeout(ctx, populatorPollInterval, populatorWaitTimeout, true, func(ctx context.Context) (bool, error) {
		for _, key := range []types.NamespacedName{
			{Name: targetName + "-prime", Namespace: f.Namespace().Name},
			{Name: targetName + "-prime-scratch", Namespace: f.Namespace().Name},
		} {
			pvc := &corev1.PersistentVolumeClaim{}
			err := f.GenericClient().Get(ctx, key, pvc)
			if err == nil {
				return false, nil
			}
			if !k8serrors.IsNotFound(err) {
				return false, err
			}
		}
		for _, podName := range podNames {
			pod := &corev1.Pod{}
			err := f.GenericClient().Get(ctx, crclient.ObjectKey{Name: podName, Namespace: f.Namespace().Name}, pod)
			if err == nil {
				return false, nil
			}
			if !k8serrors.IsNotFound(err) {
				return false, err
			}
		}
		return true, nil
	})
	Expect(err).NotTo(HaveOccurred())
}

// bindPVC waits for a freshly created PVC to become Bound. On a
// WaitForFirstConsumer StorageClass a bare PVC never binds on its own — this
// also holds for the csi-clone and snapshot-restore targets, whose provisioning
// (and hence the cloning itself) starts only at the first consumer — so run a
// short-lived consumer pod first to trigger provisioning.
func bindPVC(ctx context.Context, f *framework.Framework, obs pvcobs.Observer, podName, pvcName string) {
	GinkgoHelper()
	sc := framework.GetConfig().StorageClass.DefaultStorageClass
	if sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer {
		runConsumerPod(ctx, f, podName, pvcName, "true")
	}
	waitPVCBound(obs)
}

func bindSourcePVC(ctx context.Context, f *framework.Framework, obs pvcobs.Observer, sourcePVC string) {
	GinkgoHelper()
	bindPVC(ctx, f, obs, "bind-source-pvc", sourcePVC)
}

func bindTargetPVC(ctx context.Context, f *framework.Framework, obs pvcobs.Observer, targetPVC string) {
	GinkgoHelper()
	bindPVC(ctx, f, obs, "bind-target-pvc", targetPVC)
}

func writeRawDiskImage(ctx context.Context, f *framework.Framework, sourcePVC string) {
	GinkgoHelper()
	runConsumerPod(ctx, f, "write-source-disk", sourcePVC, "dd if=/dev/zero of=/data/disk.img bs=1M count=1")
}

// runConsumerPod runs a short-lived pod that mounts pvcName and executes
// script, waiting until the pod succeeds.
func runConsumerPod(ctx context.Context, f *framework.Framework, podName, pvcName, script string) {
	GinkgoHelper()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: f.Namespace().Name,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			SecurityContext: &corev1.PodSecurityContext{
				FSGroup: ptr.To[int64](65532),
			},
			Containers: []corev1.Container{{
				Name:    "consumer",
				Image:   framework.GetConfig().HelperImages.CurlImage,
				Command: []string{"/bin/sh", "-c"},
				Args:    []string{script},
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "data",
					MountPath: "/data",
				}},
			}},
			Volumes: []corev1.Volume{{
				Name: "data",
				VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				}},
			}},
		},
	}
	obs := podobs.StartObserver(ctx, f, pod)
	obs.Never(podobs.BeFailed())
	Expect(f.CreateWithDeferredDeletion(ctx, pod)).To(Succeed())
	err := obs.WaitFor(podobs.BeSucceeded(), populatorWaitTimeout)
	Expect(err).NotTo(HaveOccurred())
}
