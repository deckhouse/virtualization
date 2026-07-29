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

package step

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = Describe("CreateImporterStep", func() {
	newHTTPDataSource := func() *v1alpha2.VirtualDiskDataSource {
		return &v1alpha2.VirtualDiskDataSource{
			Type: v1alpha2.DataSourceTypeHTTP,
			HTTP: &v1alpha2.DataSourceHTTP{URL: "https://example.com/image.qcow2"},
		}
	}

	settingsBuilder := func(vd *v1alpha2.VirtualDisk, _ supplements.Generator) *importer.Settings {
		return &importer.Settings{DestinationEndpoint: "registry.example.com/vd/" + vd.Namespace + "/" + vd.Name}
	}

	takeStep := func(objects ...client.Object) (*v1alpha2.VirtualDisk, client.Client) {
		vd := newTestVD(newHTTPDataSource(), "vm")
		fakeClient := fake.NewClientBuilder().WithScheme(newStepScheme()).WithObjects(objects...).Build()

		importerService := service.NewImporterService(
			&dvcr.Settings{RegistryURL: "registry.example.com", TokenSigner: newTestSigner()},
			fakeClient,
			"importer-image",
			corev1.ResourceRequirements{},
			string(corev1.PullIfNotPresent),
			"1",
			"vd-controller",
			service.NewProtectionService(fakeClient, "virtualization.deckhouse.io/vd-protection"),
		)

		cb := conditions.NewConditionBuilder(vdcondition.ReadyType)
		result, err := NewCreateImporterStep(nil, nil, settingsBuilder, importerService, fakeClient, newTestRecorder(), cb, "import started").
			Take(context.Background(), vd)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())

		return vd, fakeClient
	}

	getImporterPod := func(fakeClient client.Client) *corev1.Pod {
		pods := &corev1.PodList{}
		Expect(fakeClient.List(context.Background(), pods)).To(Succeed())
		Expect(pods.Items).To(HaveLen(1))
		return &pods.Items[0]
	}

	It("creates the importer pod with vm, vm class and system tolerations", func() {
		_, fakeClient := takeStep(newTestVM(testVMToleration), newTestVMClass(testVMClassToleration))

		pod := getImporterPod(fakeClient)
		Expect(pod.Spec.Tolerations).To(ContainElements(testVMToleration, testVMClassToleration, testSystemToleration))
		Expect(pod.Annotations).To(HaveKey(annotations.AnnTolerationsHash))
	})

	It("creates the importer pod with only the system toleration when the vm is missing", func() {
		_, fakeClient := takeStep()

		pod := getImporterPod(fakeClient)
		Expect(pod.Spec.Tolerations).To(ContainElement(testSystemToleration))
		Expect(pod.Spec.Tolerations).ToNot(ContainElement(testVMToleration))
	})
})
