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

package store

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
)

func TestMigrationStoreSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Migration Store Suite")
}

var _ = Describe("Migration store", func() {
	It("should create shared ConfigMap and mark migration as completed", func() {
		fakeClient, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		store := NewConfigMapStore(fakeClient)
		completed, err := store.IsCompleted(context.Background(), "test-migration")
		Expect(err).NotTo(HaveOccurred())
		Expect(completed).To(BeFalse())

		Expect(store.MarkCompleted(context.Background(), "test-migration")).To(Succeed())

		completed, err = store.IsCompleted(context.Background(), "test-migration")
		Expect(err).NotTo(HaveOccurred())
		Expect(completed).To(BeTrue())

		cm := &corev1.ConfigMap{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: ConfigMapName, Namespace: Namespace}, cm)).To(Succeed())
		completedAt, ok := cm.Data["test-migration"]
		Expect(ok).To(BeTrue())
		_, err = time.Parse(time.RFC3339, completedAt)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should not overwrite existing completion time", func() {
		completedAt := "2026-04-29T10:25:06Z"
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: ConfigMapName, Namespace: Namespace},
			Data:       map[string]string{"test-migration": completedAt},
		}
		fakeClient, err := testutil.NewFakeClientWithObjects(cm)
		Expect(err).NotTo(HaveOccurred())

		store := NewConfigMapStore(fakeClient)
		Expect(store.MarkCompleted(context.Background(), "test-migration")).To(Succeed())

		updatedCM := &corev1.ConfigMap{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: ConfigMapName, Namespace: Namespace}, updatedCM)).To(Succeed())
		Expect(updatedCM.Data).To(HaveKeyWithValue("test-migration", completedAt))
	})

	It("should return false when shared ConfigMap exists without migration key", func() {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: ConfigMapName, Namespace: Namespace},
			Data:       map[string]string{"other-migration": "2026-04-29T10:25:06Z"},
		}
		fakeClient, err := testutil.NewFakeClientWithObjects(cm)
		Expect(err).NotTo(HaveOccurred())

		store := NewConfigMapStore(fakeClient)
		completed, err := store.IsCompleted(context.Background(), "test-migration")
		Expect(err).NotTo(HaveOccurred())
		Expect(completed).To(BeFalse())
	})

	It("should preserve existing migration records", func() {
		otherCompletedAt := "2026-04-29T10:25:06Z"
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: ConfigMapName, Namespace: Namespace},
			Data:       map[string]string{"other-migration": otherCompletedAt},
		}
		fakeClient, err := testutil.NewFakeClientWithObjects(cm)
		Expect(err).NotTo(HaveOccurred())

		store := NewConfigMapStore(fakeClient)
		Expect(store.MarkCompleted(context.Background(), "test-migration")).To(Succeed())

		updatedCM := &corev1.ConfigMap{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: ConfigMapName, Namespace: Namespace}, updatedCM)).To(Succeed())
		Expect(updatedCM.Data).To(HaveKeyWithValue("other-migration", otherCompletedAt))
		Expect(updatedCM.Data).To(HaveKey("test-migration"))
	})

	It("should initialize empty data in existing ConfigMap", func() {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: ConfigMapName, Namespace: Namespace},
		}
		fakeClient, err := testutil.NewFakeClientWithObjects(cm)
		Expect(err).NotTo(HaveOccurred())

		store := NewConfigMapStore(fakeClient)
		Expect(store.MarkCompleted(context.Background(), "test-migration")).To(Succeed())

		updatedCM := &corev1.ConfigMap{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: ConfigMapName, Namespace: Namespace}, updatedCM)).To(Succeed())
		Expect(updatedCM.Data).To(HaveKey("test-migration"))
	})
})
