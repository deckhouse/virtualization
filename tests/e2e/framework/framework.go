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
	"sync"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	NamespaceBasePrefix = "virtualization-e2e"
	NamespaceLabel      = "virtualization-e2e"
)

type Framework struct {
	Clients

	namespacePrefix string
	skipNsCreation  bool

	namespace          *corev1.Namespace
	namespacesToDelete map[string]struct{}

	objectsToDelete map[string]client.Object

	mu sync.Mutex
}

func NewFramework(namespacePrefix string) *Framework {
	return &Framework{
		Clients:         GetClients(),
		namespacePrefix: namespacePrefix,
		skipNsCreation:  namespacePrefix == "",

		namespacesToDelete: make(map[string]struct{}),
		objectsToDelete:    make(map[string]client.Object),
	}
}

func (f *Framework) BeforeEach() {
	ginkgo.BeforeEach(f.Before)
}

func (f *Framework) BeforeAll() {
	ginkgo.BeforeAll(f.Before)
}

func (f *Framework) AfterEach() {
	ginkgo.AfterEach(f.After)
}

func (f *Framework) AfterAll() {
	ginkgo.AfterAll(f.After)
}

func (f *Framework) Before() {
	ginkgo.GinkgoHelper()
	if !f.skipNsCreation {
		ns, err := f.CreateNamespace(f.namespacePrefix, nil)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ginkgo.By(fmt.Sprintf("Create namespace %s", ns.Name))
		f.namespace = ns
		f.DeferNamespaceDelete(ns.Name)
	}
}

func (f *Framework) After() {
	ginkgo.GinkgoHelper()

	for _, obj := range f.objectsToDelete {
		ginkgo.By(fmt.Sprintf("Delete object %s", obj.GetName()))
		err := f.GenericClient().Delete(context.Background(), obj)
		if err != nil && !k8serrors.IsNotFound(err) {
			ginkgo.Fail(fmt.Sprintf("Failed to delete object %s: %s", obj.GetName(), err.Error()))
		}

		f.mu.Lock()
		delete(f.objectsToDelete, string(obj.GetUID()))
		f.mu.Unlock()
	}

	for ns := range f.namespacesToDelete {
		ginkgo.By(fmt.Sprintf("Delete namespace %s", ns))
		err := f.KubeClient().CoreV1().Namespaces().Delete(context.Background(), ns, metav1.DeleteOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			ginkgo.Fail(fmt.Sprintf("Failed to delete namespace %s: %s", ns, err.Error()))
		}

		f.mu.Lock()
		delete(f.namespacesToDelete, ns)
		f.mu.Unlock()
	}
}

func (f *Framework) CreateNamespace(prefix string, labels map[string]string) (*corev1.Namespace, error) {
	ginkgo.GinkgoHelper()
	nsLabels := map[string]string{
		NamespaceLabel: prefix,
	}
	maps.Copy(nsLabels, labels)
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-%s-", NamespaceBasePrefix, prefix),
			Labels:       nsLabels,
		},
	}

	ns, err := f.KubeClient().CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	f.DeferNamespaceDelete(ns.Name)

	return ns, nil
}

func (f *Framework) Namespace() *corev1.Namespace {
	return f.namespace
}

func (f *Framework) DeferNamespaceDelete(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.namespacesToDelete[name] = struct{}{}
}

func (f *Framework) DeferDelete(objs ...client.Object) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, obj := range objs {
		f.objectsToDelete[string(obj.GetUID())] = obj
	}
}

func (f *Framework) BatchCreate(ctx context.Context, objs ...client.Object) error {
	for _, obj := range objs {
		err := f.client.Create(ctx, obj)
		if err != nil {
			return err
		}
	}

	return nil
}
