/*
Copyright 2025 Flant JSC

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

package framework

import (
	"context"
	"fmt"
	"maps"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/test/e2e/internal/config"
)

const (
	NamespaceBasePrefix = "v12n-e2e"
	// A label allows to tag the resources created during e2e testing.
	// In case the resource cleanup at the end of the test does not work properly,
	// the resources created during testing can be manually deleted using this label.
	E2ELabel = "v12n-e2e"
)

type Framework struct {
	Clients

	skipNsCreation  bool
	namespacePrefix string
	namespace       *corev1.Namespace

	objectsToDelete []client.Object
}

func NewFramework(namespacePrefix string) *Framework {
	return &Framework{
		Clients:         GetClients(),
		namespacePrefix: namespacePrefix,
		skipNsCreation:  namespacePrefix == "",
	}
}

func (f *Framework) Before() {
	GinkgoHelper()
	if !f.skipNsCreation {
		ns, err := f.createNamespace(f.namespacePrefix)
		Expect(err).NotTo(HaveOccurred())

		By(fmt.Sprintf("Namespace %q has been created", ns.Name))
		f.namespace = ns
	}
}

func (f *Framework) After() {
	GinkgoHelper()

	if config.IsCleanUpNeeded() {
		defer func() {
			if f.namespace != nil {
				By("Cleanup: delete namespace")
				err := f.Delete(context.Background(), f.namespace)
				Expect(err).NotTo(HaveOccurred(), "Failed to delete namespace %q", f.namespace.Name)
			}
		}()

		defer func() {
			By("Cleanup: process deferred deletions")
			err := f.Delete(context.Background(), f.objectsToDelete...)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete object")
		}()
	}

	if CurrentSpecReport().Failed() {
		if f.namespace != nil {
			By("Failed: save resource dump")
			f.saveTestCaseDump()
		}
	}
}

func (f *Framework) createNamespace(prefix string) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-%s-", NamespaceBasePrefix, prefix),
			Labels: map[string]string{
				E2ELabel: "true",
			},
		},
	}

	ns, err := f.KubeClient().CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}

	return ns, nil
}

func (f *Framework) Namespace() *corev1.Namespace {
	return f.namespace
}

func (f *Framework) DeferDelete(objs ...client.Object) {
	f.objectsToDelete = append(f.objectsToDelete, objs...)
}

func (f *Framework) Delete(ctx context.Context, objs ...client.Object) error {
	// 1. Send deletion request for objects.
	for _, obj := range objs {
		err := f.client.Delete(ctx, obj)
		if err != nil && !k8serrors.IsNotFound(err) {
			return err
		}
	}

	// 2. Wait for the objects to be deleted.
	for _, obj := range objs {
		key := types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
		}

		err := wait.PollUntilContextTimeout(ctx, time.Second, LongTimeout, true, func(ctx context.Context) (bool, error) {
			err := f.client.Get(ctx, key, obj)
			switch {
			case err == nil:
				return false, nil
			case k8serrors.IsNotFound(err):
				return true, nil
			default:
				return false, err
			}
		})
		if err != nil {
			return fmt.Errorf("object %q not deleted in time: %w", key, err)
		}
	}

	return nil
}

// CreateWithDeferredDeletion creates one or more Kubernetes resources and
// adds them to a list for deferred deletion.
//
// Returns an error if the creation of any resource
func (f *Framework) CreateWithDeferredDeletion(ctx context.Context, objs ...client.Object) error {
	for _, obj := range objs {
		labels := obj.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		maps.Copy(labels, map[string]string{E2ELabel: f.namespacePrefix})
		obj.SetLabels(labels)

		err := f.client.Create(ctx, obj)
		if err != nil {
			return err
		}
		f.DeferDelete(obj)
	}

	return nil
}
