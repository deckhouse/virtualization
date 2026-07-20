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

package blockdevice

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
	scobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/storageclass"
	spobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/storageprofile"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/rewrite"
)

// The virtualization-controller storageprofile controller must create a StorageProfile
// for every StorageClass added to the cluster and remove it once the StorageClass is
// deleted. A plain StorageClass is used so the test controls the full lifecycle:
// SDS-managed StorageClasses cannot be deleted directly and are not removed when the
// owning ReplicatedStorageClass is deleted.
var _ = Describe("StorageProfileCreation", Label(label.SIGStorage, precheck.NoPrecheck), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("")
		f.Before()
		DeferCleanup(f.After)
	})

	It("creates a StorageProfile when a StorageClass is added and deletes it when the StorageClass is removed", func() {
		// Cluster-scoped name with a random suffix to stay unique across parallel runs.
		name := fmt.Sprintf("v12n-e2e-storageprofile-%s", rand.String(6))

		scObs := scobs.StartObserver(ctx, f, name)
		spObs := spobs.StartObserver(ctx, f, name)

		By("Creating a test StorageClass", func() {
			_, err := f.KubeClient().StorageV1().StorageClasses().Create(ctx, newTestStorageClass(name), metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "failed to create StorageClass %q", name)

			DeferCleanup(func() {
				err := f.KubeClient().StorageV1().StorageClasses().Delete(context.Background(), name, metav1.DeleteOptions{})
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue(),
					"failed to delete StorageClass %q: %v", name, err)
			})
		})

		By("Waiting for the StorageProfile to be created for the new StorageClass", func() {
			err := scObs.WaitFor(scobs.BeAvailable(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred(), "StorageClass %q was not available", name)

			err = spObs.WaitFor(spobs.BeReady(name), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred(), "StorageProfile %q was not created for the new StorageClass", name)
		})

		By("Deleting the test StorageClass", func() {
			err := f.KubeClient().StorageV1().StorageClasses().Delete(ctx, name, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred(), "failed to delete StorageClass %q", name)
		})

		By("Waiting for the StorageClass to be deleted", func() {
			err := waitForStorageClassDeleted(ctx, f, name, framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred(), "StorageClass %q was not deleted", name)
		})

		By("Waiting for the StorageProfile to be deleted for the removed StorageClass", func() {
			err := waitForStorageProfileDeleted(ctx, f, name, framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred(), "StorageProfile %q was not deleted after its StorageClass was removed", name)
		})
	})
})

func newTestStorageClass(name string) *storagev1.StorageClass {
	reclaimPolicy := corev1.PersistentVolumeReclaimDelete
	volumeBindingMode := storagev1.VolumeBindingWaitForFirstConsumer
	return &storagev1.StorageClass{
		ObjectMeta:        metav1.ObjectMeta{Name: name},
		Provisioner:       "kubernetes.io/no-provisioner",
		ReclaimPolicy:     &reclaimPolicy,
		VolumeBindingMode: &volumeBindingMode,
	}
}

func waitForStorageClassDeleted(ctx context.Context, f *framework.Framework, name string, timeout time.Duration) error {
	return observer.WaitForDeleted(
		ctx,
		f.KubeClient().StorageV1().StorageClasses(),
		name,
		"",
		timeout,
		func(ctx context.Context) (bool, error) {
			_, err := f.KubeClient().StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		},
	)
}

func waitForStorageProfileDeleted(ctx context.Context, f *framework.Framework, name string, timeout time.Duration) error {
	spGVR := rewrite.StorageProfile{}.GVR()
	return observer.WaitForDeleted(
		ctx,
		observer.DynamicWatcher(f.DynamicClient(), spGVR, ""),
		name,
		"",
		timeout,
		func(ctx context.Context) (bool, error) {
			_, err := f.DynamicClient().Resource(spGVR).Get(ctx, name, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		},
	)
}
