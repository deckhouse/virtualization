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
	"os/exec"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
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
	namespacesToDelete []string
	resourcesToDelete  []client.Object
}

func NewFramework(namespacePrefix string) *Framework {
	return &Framework{
		Clients:         GetClients(),
		namespacePrefix: namespacePrefix,
		skipNsCreation:  namespacePrefix == "",
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
		ginkgo.By(fmt.Sprintf("Created namespace %s", ns.Name))
		f.namespace = ns
	}
}

func (f *Framework) After() {
	ginkgo.GinkgoHelper()

	for _, resource := range f.resourcesToDelete {
		ginkgo.By(fmt.Sprintf("Delete resource %s", resource.GetName()))
		err := f.client.Delete(context.Background(), resource)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}

	for _, ns := range f.namespacesToDelete {
		ginkgo.By(fmt.Sprintf("Delete namespace %s", ns))
		err := f.KubeClient().CoreV1().Namespaces().Delete(context.Background(), ns, metav1.DeleteOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
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
			Name:   fmt.Sprintf("%s-%s-%s", NamespaceBasePrefix, prefix, GetCommitHash()),
			Labels: nsLabels,
		},
	}

	ns, err := f.KubeClient().CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	f.AddNamespaceToDelete(ns.Name)

	return ns, nil
}

func (f *Framework) Namespace() *corev1.Namespace {
	return f.namespace
}

func (f *Framework) AddNamespaceToDelete(name string) {
	f.namespacesToDelete = append(f.namespacesToDelete, name)
}

func (f *Framework) AddResourceToDelete(obj client.Object) {
	f.resourcesToDelete = append(f.resourcesToDelete, obj)
}

// func (f *Framework) GetRunHash() string {
// 	parts := strings.Split(f.Namespace().Name, "-")
// 	if len(parts) == 0 {
// 		return ""
// 	}
// 	return parts[len(parts)-1]
// }

func GetCommitHash() string {
	ginkgo.GinkgoHelper()

	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	stdout, err := cmd.Output()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return strings.TrimSpace(string(stdout))
}
