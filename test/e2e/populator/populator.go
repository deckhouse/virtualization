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
	populatorWaitTimeout   = 10 * time.Minute
	populatorPollInterval  = 2 * time.Second
	snapshotStorageAPI     = "snapshot.storage.k8s.io"
	populatorSourcePVCName = "source"

	populationStrategyCSIClone     = "csi-clone"
	populationStrategySnapshot     = "snapshot"
	populationStrategyHostAssigned = "host-assigned"
	populationStrategyDVCR         = "dvcr"
)

var _ = Describe("Populator", Label(precheck.PrecheckImmediateStorageClass, precheck.PrecheckSnapshot), func() {
	var (
		ctx context.Context
		f   *framework.Framework
		sc  string
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("populator")
		f.Before()
		DeferCleanup(f.After)

		immediateSC := framework.GetConfig().StorageClass.ImmediateStorageClass
		if immediateSC == nil {
			Skip("Immediate StorageClass is not configured")
		}
		sc = immediateSC.Name
	})

	It("creates target PVC from PVC using CSI clone", func() {
		source := newPopulatorPVC(populatorSourcePVCName, f.Namespace().Name, sc, nil)
		target := newPopulatorPVC("target-csi-clone", f.Namespace().Name, sc, map[string]string{
			annotations.AnnPVCPopulationStrategy:           populationStrategyCSIClone,
			annotations.AnnPVCPopulationSourcePVC:          source.Name,
			annotations.AnnPVCPopulationSourcePVCNamespace: source.Namespace,
		})
		target.Spec.DataSourceRef = &corev1.TypedObjectReference{Kind: "PersistentVolumeClaim", Name: source.Name}

		sourceObs := startPVCObserver(ctx, f, source)
		targetObs := startPVCObserver(ctx, f, target)
		Expect(f.CreateWithDeferredDeletion(ctx, source)).To(Succeed())
		waitPVCBound(sourceObs)
		Expect(f.CreateWithDeferredDeletion(ctx, target)).To(Succeed())

		waitPVCBoundAndDone(targetObs)
		waitPopulatorCleanup(ctx, f, target.Name)
	})

	It("creates target PVC from PVC using snapshot", func() {
		source := newPopulatorPVC(populatorSourcePVCName, f.Namespace().Name, sc, nil)
		snapshotName := "target-snapshot-" + rand.String(5)
		target := newPopulatorPVC("target-snapshot", f.Namespace().Name, sc, map[string]string{
			annotations.AnnPVCPopulationStrategy:           populationStrategySnapshot,
			annotations.AnnPVCPopulationSourcePVC:          source.Name,
			annotations.AnnPVCPopulationSourcePVCNamespace: source.Namespace,
			annotations.AnnPVCImportCloneSnapshot:          snapshotName,
		})
		target.Spec.DataSource = &corev1.TypedLocalObjectReference{APIGroup: ptr.To(snapshotStorageAPI), Kind: "VolumeSnapshot", Name: snapshotName}
		target.Spec.DataSourceRef = &corev1.TypedObjectReference{APIGroup: ptr.To(snapshotStorageAPI), Kind: "VolumeSnapshot", Name: snapshotName}

		sourceObs := startPVCObserver(ctx, f, source)
		targetObs := startPVCObserver(ctx, f, target)
		Expect(f.CreateWithDeferredDeletion(ctx, source)).To(Succeed())
		waitPVCBound(sourceObs)
		Expect(f.CreateWithDeferredDeletion(ctx, target)).To(Succeed())

		waitPVCBoundAndDone(targetObs)
		waitPopulatorCleanup(ctx, f, target.Name)
	})

	It("creates target PVC from PVC using host assigned population", func() {
		source := newPopulatorPVC(populatorSourcePVCName, f.Namespace().Name, sc, nil)
		target := newPopulatorPVC("target-host-assigned", f.Namespace().Name, sc, map[string]string{
			annotations.AnnPVCPopulationStrategy:           populationStrategyHostAssigned,
			annotations.AnnPVCPopulationSourcePVC:          source.Name,
			annotations.AnnPVCPopulationSourcePVCNamespace: source.Namespace,
		})
		target.Spec.DataSourceRef = &corev1.TypedObjectReference{
			APIGroup: ptr.To("virtualization.deckhouse.io"),
			Kind:     "PersistentVolumeClaim",
			Name:     source.Name,
		}

		sourceObs := startPVCObserver(ctx, f, source)
		targetObs := startPVCObserver(ctx, f, target)
		Expect(f.CreateWithDeferredDeletion(ctx, source)).To(Succeed())
		waitPVCBound(sourceObs)
		writeRawDiskImage(ctx, f, source.Name)
		Expect(f.CreateWithDeferredDeletion(ctx, target)).To(Succeed())

		waitPVCBoundAndDone(targetObs)
		waitPopulatorCleanup(ctx, f, target.Name)
	})

	It("creates target PVC from DVCR", func() {
		cvi := &v1alpha2.ClusterVirtualImage{}
		Expect(f.GenericClient().Get(ctx, crclient.ObjectKey{Name: object.PrecreatedCVIAlpineBIOS}, cvi)).To(Succeed())
		Expect(cvi.Status.Target.RegistryURL).NotTo(BeEmpty())

		target := newPopulatorPVC("target-dvcr", f.Namespace().Name, sc, map[string]string{
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
	})
})

func newPopulatorPVC(name, namespace, storageClass string, anns map[string]string) *corev1.PersistentVolumeClaim {
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
				corev1.ResourceStorage: resource.MustParse(populatorPVCSize),
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
	Expect(obs.WaitFor(pvcobs.BeBoundAndPopulated(), populatorWaitTimeout)).To(Succeed())
}

func waitPVCBound(obs pvcobs.Observer) {
	GinkgoHelper()
	Expect(obs.WaitFor(pvcobs.BeBound(), populatorWaitTimeout)).To(Succeed())
}

func waitPopulatorCleanup(ctx context.Context, f *framework.Framework, targetName string) {
	GinkgoHelper()
	target := &corev1.PersistentVolumeClaim{}
	Expect(f.GenericClient().Get(ctx, crclient.ObjectKey{Name: targetName, Namespace: f.Namespace().Name}, target)).To(Succeed())
	importerPodName := "d8v-pvc-pvc-importer-" + string(target.UID)

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
		pod := &corev1.Pod{}
		err := f.GenericClient().Get(ctx, crclient.ObjectKey{Name: importerPodName, Namespace: f.Namespace().Name}, pod)
		if err == nil {
			return false, nil
		}
		if !k8serrors.IsNotFound(err) {
			return false, err
		}
		return true, nil
	})
	Expect(err).NotTo(HaveOccurred())
}

func writeRawDiskImage(ctx context.Context, f *framework.Framework, sourcePVC string) {
	GinkgoHelper()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "write-source-disk",
			Namespace: f.Namespace().Name,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			SecurityContext: &corev1.PodSecurityContext{
				FSGroup: ptr.To[int64](65532),
			},
			Containers: []corev1.Container{{
				Name:    "writer",
				Image:   framework.GetConfig().HelperImages.CurlImage,
				Command: []string{"/bin/sh", "-c"},
				Args:    []string{"dd if=/dev/zero of=/data/disk.img bs=1M count=1"},
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "data",
					MountPath: "/data",
				}},
			}},
			Volumes: []corev1.Volume{{
				Name: "data",
				VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: sourcePVC,
				}},
			}},
		},
	}
	obs := podobs.StartObserver(ctx, f, pod)
	obs.Never(podobs.BeFailed())
	Expect(f.CreateWithDeferredDeletion(ctx, pod)).To(Succeed())
	Expect(obs.WaitFor(podobs.BeSucceeded(), populatorWaitTimeout)).To(Succeed())
}
