/*
Copyright 2024 Flant JSC

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

package e2e

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/framework"
	"github.com/deckhouse/virtualization/tests/e2e/helper"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

var _ = Describe("VirtualImageCreation", framework.CommonE2ETestDecorators(), func() {
	var (
		testCaseLabel = map[string]string{"testcase": "images-creation"}
		ns            string
		criticalError error
	)

	BeforeAll(func() {
		kustomization := fmt.Sprintf("%s/%s", conf.TestData.ImagesCreation, "kustomization.yaml")
		var err error
		ns, err = kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)

		CreateNamespace(ns)

		Expect(conf.StorageClass.ImmediateStorageClass).NotTo(BeNil(), "immediate storage class cannot be nil; please set up the immediate storage class in the cluster")

		virtualDisk := v1alpha2.VirtualDisk{}
		vdFilePath := fmt.Sprintf("%s/vd/vd-alpine-http.yaml", conf.TestData.ImagesCreation)
		err = helper.UnmarshalResource(vdFilePath, &virtualDisk)
		Expect(err).NotTo(HaveOccurred(), "cannot get object from file: %s\nstderr: %s", vdFilePath, err)

		virtualDisk.Spec.PersistentVolumeClaim.StorageClass = &conf.StorageClass.ImmediateStorageClass.Name
		err = helper.WriteYamlObject(vdFilePath, &virtualDisk)
		Expect(err).NotTo(HaveOccurred(), "cannot update virtual disk with custom storage class: %s\nstderr: %s", vdFilePath, err)

		virtualDiskSnapshot := v1alpha2.VirtualDiskSnapshot{}
		vdSnapshotFilePath := fmt.Sprintf("%s/vdsnapshot/vdsnapshot.yaml", conf.TestData.ImagesCreation)
		err = helper.UnmarshalResource(vdSnapshotFilePath, &virtualDiskSnapshot)
		Expect(err).NotTo(HaveOccurred(), "cannot get object from file: %s\nstderr: %s", vdSnapshotFilePath, err)

		err = helper.WriteYamlObject(vdSnapshotFilePath, &virtualDiskSnapshot)
		Expect(err).NotTo(HaveOccurred(), "cannot update virtual disk with custom storage class: %s\nstderr: %s", vdSnapshotFilePath, err)
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestCaseDump(testCaseLabel, CurrentSpecReport().LeafNodeText, ns)
		}
	})

	BeforeEach(func() {
		if criticalError != nil {
			Skip(fmt.Sprintf("Skip because blinking error: %s", criticalError.Error()))
		}
	})

	Context("When resources are applied", func() {
		It("result should be succeeded", func() {
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.ImagesCreation},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
		})
	})

	Context("When base virtual resources are ready", func() {
		It("checks VD phase", func() {
			By(fmt.Sprintf("VD should be in %s phase", v1alpha2.DiskReady))
			WaitPhaseByLabel(kc.ResourceVD, string(v1alpha2.DiskReady), kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})

		It("checks VDSnapshot phase", func() {
			By(fmt.Sprintf("VDSnapshot should be in %s phase", v1alpha2.VirtualDiskSnapshotPhaseReady))
			WaitPhaseByLabel(kc.ResourceVDSnapshot, string(v1alpha2.VirtualDiskSnapshotPhaseReady), kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual images are applied", func() {
		It("checks VIs phases", func() {
			By(fmt.Sprintf("VIs should be in %s phases", v1alpha2.ImageReady))
			err := InterceptGomegaFailure(func() {
				WaitPhaseByLabel(kc.ResourceVI, string(v1alpha2.ImageReady), kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
			})
			if err != nil {
				criticalError = err
			}
		})

		It("checks CVIs phases", func() {
			By(fmt.Sprintf("CVIs should be in %s phases", v1alpha2.ImageReady))
			WaitPhaseByLabel(kc.ResourceCVI, string(v1alpha2.ImageReady), kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			DeleteTestCaseResources(ns, ResourcesToDelete{
				KustomizationDir: conf.TestData.ImagesCreation,
			})
		})
	})
})
