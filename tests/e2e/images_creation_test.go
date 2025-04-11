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
	storagev1 "k8s.io/api/storage/v1"

	sdsrepvolv1 "github.com/deckhouse/sds-replicated-volume/api/v1alpha1"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	. "github.com/deckhouse/virtualization/tests/e2e/helper"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

var _ = Describe("Virtual images creation", ginkgoutil.CommonE2ETestDecorators(), func() {
	var (
		immediateStorageClassName string // require for unattached virtual disk snapshots
		testCaseLabel             = map[string]string{"testcase": "images-creation"}
	)

	BeforeEach(func() {
		if config.IsReusable() {
			Skip("Test not available in REUSABLE mode: not supported yet.")
		}
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestResources(testCaseLabel, CurrentSpecReport().LeafNodeText)
		}
	})

	Context("Preparing the environment", func() {
		It("sets the namespace", func() {
			kustomization := fmt.Sprintf("%s/%s", conf.TestData.ImagesCreation, "kustomization.yaml")
			ns, err := kustomize.GetNamespace(kustomization)
			Expect(err).NotTo(HaveOccurred(), "%w", err)
			conf.SetNamespace(ns)
		})

		It("prepares `Immediate` storage class and virtual disk that use it", func() {
			sc, err := GetDefaultStorageClass()
			Expect(err).NotTo(HaveOccurred(), "cannot get default storage class\nstderr: %s", err)

			defaultVolumeSnapshotClassName, err := GetVolumeSnapshotClassName(sc)
			Expect(err).NotTo(HaveOccurred(), "cannot define default `VolumeSnapshotClass`\nstderr: %s", err)

			if sc.Provisioner == LinstorProviderName {
				storagePoolName := sc.Parameters["replicated.csi.storage.deckhouse.io/storagePool"]
				storagePoolObj := sdsrepvolv1.ReplicatedStoragePool{}
				err := GetObject(kc.ResourceReplicatedStoragePool, storagePoolName, &storagePoolObj, kc.GetOptions{})
				Expect(err).NotTo(HaveOccurred(), "cannot get `storagePoolObj`: %s\nstderr: %s", storagePoolName, err)
				Expect(storagePoolObj.Spec.Type).To(Equal(LVMThinName), "type of replicated storage pool should be `LVMThin`")
			}

			if *sc.VolumeBindingMode != storagev1.VolumeBindingImmediate {
				immediateStorageClassName, err = CreateImmediateStorageClass(sc.Provisioner, testCaseLabel)
				Expect(err).NotTo(HaveOccurred(), "%s", err)

				virtualDisk := virtv2.VirtualDisk{}
				vdFilePath := fmt.Sprintf("%s/vd/vd-alpine-http.yaml", conf.TestData.ImagesCreation)
				err = UnmarshalResource(vdFilePath, &virtualDisk)
				Expect(err).NotTo(HaveOccurred(), "cannot get object from file: %s\nstderr: %s", vdFilePath, err)

				virtualDisk.Spec.PersistentVolumeClaim.StorageClass = &immediateStorageClassName
				err = WriteYamlObject(vdFilePath, &virtualDisk)
				Expect(err).NotTo(HaveOccurred(), "cannot update virtual disk with custom storage class: %s\nstderr: %s", vdFilePath, err)
			} else {
				immediateStorageClassName = sc.Name
			}

			virtualDiskSnapshot := virtv2.VirtualDiskSnapshot{}
			vdSnapshotFilePath := fmt.Sprintf("%s/vdsnapshot/vdsnapshot.yaml", conf.TestData.ImagesCreation)
			err = UnmarshalResource(vdSnapshotFilePath, &virtualDiskSnapshot)
			Expect(err).NotTo(HaveOccurred(), "cannot get object from file: %s\nstderr: %s", vdSnapshotFilePath, err)

			virtualDiskSnapshot.Spec.VolumeSnapshotClassName = defaultVolumeSnapshotClassName
			err = WriteYamlObject(vdSnapshotFilePath, &virtualDiskSnapshot)
			Expect(err).NotTo(HaveOccurred(), "cannot update virtual disk with custom storage class: %s\nstderr: %s", vdSnapshotFilePath, err)
		})
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
			By(fmt.Sprintf("VD should be in %s phase", virtv2.DiskReady))
			WaitPhaseByLabel(kc.ResourceVD, string(virtv2.DiskReady), kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})

		It("checks VDSnapshot phase", func() {
			By(fmt.Sprintf("VDSnapshot should be in %s phase", virtv2.VirtualDiskSnapshotPhaseReady))
			WaitPhaseByLabel(kc.ResourceVDSnapshot, string(virtv2.VirtualDiskSnapshotPhaseReady), kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual images are applied", func() {
		It("checks VIs phases", func() {
			By(fmt.Sprintf("VIs should be in %s phases", virtv2.ImageReady))
			WaitPhaseByLabel(kc.ResourceVI, string(virtv2.ImageReady), kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})

		It("checks CVIs phases", func() {
			By(fmt.Sprintf("CVIs should be in %s phases", virtv2.ImageReady))
			WaitPhaseByLabel(kc.ResourceCVI, string(virtv2.ImageReady), kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			DeleteTestCaseResources(ResourcesToDelete{
				KustomizationDir: conf.TestData.ImagesCreation,
				AdditionalResources: []AdditionalResource{
					{
						Resource: kc.ResourceReplicatedStorageClass,
						Labels:   testCaseLabel,
					},
					{
						Resource: kc.ResourceStorageClass,
						Labels:   testCaseLabel,
					},
				},
			})
		})
	})
})
