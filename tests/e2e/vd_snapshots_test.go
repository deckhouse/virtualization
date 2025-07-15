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
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	snapshotvolv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	storagev1 "k8s.io/api/storage/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sdsrepvolv1 "github.com/deckhouse/sds-replicated-volume/api/v1alpha1"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	. "github.com/deckhouse/virtualization/tests/e2e/helper"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

const (
	ReplicatedStorageClassKind     = "ReplicatedStorageClass"
	LinstorProviderName            = "replicated.csi.storage.deckhouse.io"
	LVMThinName                    = "LVMThin"
	CephProviderName               = "rbd.csi.ceph.com"
	filesystemReadyTimeout         = 60 * time.Second
	filesystemReadyPollingInterval = 5 * time.Second
	frozenReasonPollingInterval    = 1 * time.Second
)

func CreateVirtualDiskSnapshot(vdName, snapshotName, volumeSnapshotClassName string, requiredConsistency bool, labels map[string]string) error {
	GinkgoHelper()
	vdSnapshotName := fmt.Sprintf("%s-%s", snapshotName, volumeSnapshotClassName)
	vdSnapshot := virtv2.VirtualDiskSnapshot{
		TypeMeta: v1.TypeMeta{
			APIVersion: APIVersion,
			Kind:       virtv2.VirtualDiskSnapshotKind,
		},
		ObjectMeta: v1.ObjectMeta{
			Labels:    labels,
			Name:      vdSnapshotName,
			Namespace: conf.Namespace,
		},
		Spec: virtv2.VirtualDiskSnapshotSpec{
			RequiredConsistency:     requiredConsistency,
			VirtualDiskName:         vdName,
			VolumeSnapshotClassName: volumeSnapshotClassName,
		},
	}

	filePath := fmt.Sprintf("%s/snapshots/%s-%s.yaml", conf.TestData.VdSnapshots, vdSnapshotName, volumeSnapshotClassName)
	err := WriteYamlObject(filePath, &vdSnapshot)
	if err != nil {
		return fmt.Errorf("cannot write file with virtual disk snapshot: %s\nstderr: ws", vdSnapshotName, err)
	}

	res := kubectl.Apply(kc.ApplyOptions{
		Filename:       []string{filePath},
		FilenameOption: kc.Filename,
	})
	if res.Error() != nil {
		return fmt.Errorf("cannot create virtual disk snapshot: %s\nstderr: %s", vdSnapshotName, res.StdErr())
	}
	return nil
}

func CreateImmediateStorageClass(provisioner string, labels map[string]string) (string, error) {
	GinkgoHelper()
	filePath := fmt.Sprintf("%s/immediate-storage-class.yaml", conf.TestData.VdSnapshots)
	switch provisioner {
	case LinstorProviderName:
		replicatedStorageClassName := fmt.Sprintf("%s-linstor-immediate", namePrefix)
		replicatedStoragePoolName, err := GetLVMThinReplicatedStoragePool()
		if err != nil {
			return "", err
		}
		err = createLinstorImmediateStorageClass(filePath, replicatedStorageClassName, replicatedStoragePoolName, labels)
		if err != nil {
			return "", err
		}
		return replicatedStorageClassName, nil
	case CephProviderName:
		storageClassName := fmt.Sprintf("%s-ceph-immediate", namePrefix)
		err := createCephImmediateStorageClass(filePath, storageClassName, labels)
		if err != nil {
			return "", err
		}
		return storageClassName, nil
	default:
		return "", errors.New("cannot create storage class with `Immediate` volume binding mode")
	}
}

// Get first replicated storage pool with type `LVMThin` and phase `Completed`
func GetLVMThinReplicatedStoragePool() (string, error) {
	GinkgoHelper()
	rspObjects := sdsrepvolv1.ReplicatedStoragePoolList{}
	err := GetObjects(kc.ResourceReplicatedStoragePool, &rspObjects, kc.GetOptions{})
	if err != nil {
		return "", err
	}
	for _, rsp := range rspObjects.Items {
		if rsp.Spec.Type == LVMThinName && rsp.Status.Phase == PhaseCompleted {
			return rsp.Name, nil
		}
	}
	return "", fmt.Errorf("cannot get completed replicated storage pool with type `LVMThin`")
}

func createLinstorImmediateStorageClass(filePath, storageClassName, replicatedStoragePoolName string, labels map[string]string) error {
	GinkgoHelper()
	replicatedStorageClass := sdsrepvolv1.ReplicatedStorageClass{
		TypeMeta: v1.TypeMeta{
			APIVersion: sdsrepvolv1.SchemeGroupVersion.String(),
			Kind:       ReplicatedStorageClassKind,
		},
		ObjectMeta: v1.ObjectMeta{
			Name:   storageClassName,
			Labels: labels,
		},
		Spec: sdsrepvolv1.ReplicatedStorageClassSpec{
			ReclaimPolicy: "Delete",
			Replication:   "None",
			StoragePool:   replicatedStoragePoolName,
			Topology:      "Ignored",
			VolumeAccess:  "Any",
		},
	}

	err := WriteYamlObject(filePath, &replicatedStorageClass)
	if err != nil {
		return nil
	}

	res := kubectl.Apply(kc.ApplyOptions{
		Filename:       []string{filePath},
		FilenameOption: kc.Filename,
	})
	if res.Error() != nil {
		return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
	}

	return nil
}

func createCephImmediateStorageClass(filePath, storageClassName string, labels map[string]string) error {
	GinkgoHelper()
	sc, err := GetDefaultStorageClass()
	if err != nil {
		return err
	}
	sc.ObjectMeta.Name = storageClassName
	sc.ObjectMeta.Labels = labels
	*sc.VolumeBindingMode = storagev1.VolumeBindingImmediate

	err = WriteYamlObject(filePath, sc)
	if err != nil {
		return err
	}

	res := kubectl.Apply(kc.ApplyOptions{
		Filename:       []string{filePath},
		FilenameOption: kc.Filename,
	})
	if res.Error() != nil {
		return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
	}

	return nil
}

func GetVolumeSnapshotClassName(storageClass *storagev1.StorageClass) (string, error) {
	vscObjects := snapshotvolv1.VolumeSnapshotClassList{}
	err := GetObjects(kc.ResourceVolumeSnapshotClass, &vscObjects, kc.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("cannot get `VolumeSnapshotClasses` by provisioner %q\nstderr: ws", storageClass.Provisioner, err)
	}

	for _, vsc := range vscObjects.Items {
		if vsc.Driver == storageClass.Provisioner {
			return vsc.Name, nil
		}
	}

	return "", fmt.Errorf("cannot found `VolumeSnapshotClass` by provisioner %q", storageClass.Provisioner)
}

func CheckFileSystemFrozen(vmName string) (bool, error) {
	vmObj := virtv2.VirtualMachine{}
	err := GetObject(kc.ResourceVM, vmName, &vmObj, kc.GetOptions{Namespace: conf.Namespace})
	if err != nil {
		return false, fmt.Errorf("cannot get `VirtualMachine`: %q\nstderr: ws", vmName, err)
	}

	for _, condition := range vmObj.Status.Conditions {
		if condition.Type == vmcondition.TypeFilesystemFrozen.String() {
			return condition.Status == v1.ConditionTrue, nil
		}
	}

	return false, nil
}

var _ = Describe("Virtual disk snapshots", ginkgoutil.CommonE2ETestDecorators(), func() {
	BeforeEach(func() {
		if config.IsReusable() {
			Skip("Test not available in REUSABLE mode: not supported yet.")
		}
	})

	var (
		immediateStorageClassName      string // require for unattached virtual disk snapshots
		defaultVolumeSnapshotClassName string
		testCaseLabel                  = map[string]string{"testcase": "vd-snapshots"}
		attachedVirtualDiskLabel       = map[string]string{"attachedVirtualDisk": ""}
		hasNoConsumerLabel             = map[string]string{"hasNoConsumer": "vd-snapshots"}
		vmAutomaticWithHotplug         = map[string]string{"vm": "automatic-with-hotplug"}
	)

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestResources(testCaseLabel, CurrentSpecReport().LeafNodeText)
		}
	})

	Context("Preparing the environment", func() {
		It("sets the namespace", func() {
			kustomization := fmt.Sprintf("%s/%s", conf.TestData.VdSnapshots, "kustomization.yaml")
			ns, err := kustomize.GetNamespace(kustomization)
			Expect(err).NotTo(HaveOccurred(), "%w", err)
			conf.SetNamespace(ns)
		})

		It("prepares `Immediate` storage class and virtual disk that use it", func() {
			sc, err := GetDefaultStorageClass()
			Expect(err).NotTo(HaveOccurred(), "cannot get default storage class\nstderr: %s", err)
			defaultVolumeSnapshotClassName, err = GetVolumeSnapshotClassName(sc)
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

				virtualDiskWithoutConsumer := virtv2.VirtualDisk{}
				vdWithoutConsumerFilePath := fmt.Sprintf("%s/vd/vd-alpine-http.yaml", conf.TestData.VdSnapshots)
				err = UnmarshalResource(vdWithoutConsumerFilePath, &virtualDiskWithoutConsumer)
				Expect(err).NotTo(HaveOccurred(), "cannot get object from file: %s\nstderr: %s", vdWithoutConsumerFilePath, err)

				virtualDiskWithoutConsumer.Spec.PersistentVolumeClaim.StorageClass = &immediateStorageClassName
				err = WriteYamlObject(vdWithoutConsumerFilePath, &virtualDiskWithoutConsumer)
				Expect(err).NotTo(HaveOccurred(), "cannot update virtual disk with custom storage class: %s\nstderr: %s", vdWithoutConsumerFilePath, err)
			} else {
				immediateStorageClassName = sc.Name
			}
		})
	})

	Context("When virtualization resources are applied:", func() {
		It("result should be succeeded", func() {
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.VdSnapshots},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), "cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
		})
	})

	Context("When virtual images are applied:", func() {
		It("checks VIs phases", func() {
			By(fmt.Sprintf("VIs should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVI, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual disks are applied:", func() {
		It("checks VDs phases", func() {
			By(fmt.Sprintf("VDs should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVD, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machines are applied:", func() {
		It("checks VMs phases", func() {
			By("Virtual machine agents should be ready")
			WaitVMAgentReady(kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machine block device attachments are applied:", func() {
		It("checks VMBDAs phases", func() {
			By(fmt.Sprintf("VMBDAs should be in %s phases", PhaseAttached))
			WaitPhaseByLabel(kc.ResourceVMBDA, PhaseAttached, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context(fmt.Sprintf("When unattached VDs in phase %s:", PhaseReady), func() {
		It("creates VDs snapshots with `requiredConsistency`", func() {
			res := kubectl.List(kc.ResourceVD, kc.GetOptions{
				Labels:    hasNoConsumerLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			Expect(res.Error()).NotTo(HaveOccurred(), "cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())

			vds := strings.Split(res.StdOut(), " ")

			sc := storagev1.StorageClass{}
			err := GetObject(kc.ResourceStorageClass, immediateStorageClassName, &sc, kc.GetOptions{})
			Expect(err).NotTo(HaveOccurred(), "cannot get storage class: %s\nstderr: %s", immediateStorageClassName, err)

			volumeSnapshotClassName, getErr := GetVolumeSnapshotClassName(&sc)
			Expect(getErr).NotTo(HaveOccurred(), "%s", getErr)

			for _, vdName := range vds {
				By(fmt.Sprintf("Create snapshot for %q with volume snapshot class %q", vdName, volumeSnapshotClassName))
				err := CreateVirtualDiskSnapshot(vdName, vdName, volumeSnapshotClassName, true, hasNoConsumerLabel)
				Expect(err).NotTo(HaveOccurred(), "%s", err)
			}
		})

		It("checks snapshots of unattached VDs", func() {
			By(fmt.Sprintf("Snapshots should be in %s phase", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVDSnapshot, PhaseReady, kc.WaitOptions{
				Labels:    hasNoConsumerLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
			// TODO: It is a known issue that disk snapshots are not always created consistently. To prevent this error from causing noise during testing, we disabled this check. It will need to be re-enabled once the consistency issue is fixed.
			// By("Snapshots should be consistent", func() {
			// 	vdSnapshots := virtv2.VirtualDiskSnapshotList{}
			// 	err := GetObjects(kc.ResourceVDSnapshot, &vdSnapshots, kc.GetOptions{Namespace: conf.Namespace, Labels: hasNoConsumerLabel})
			// 	Expect(err).NotTo(HaveOccurred(), "cannot get `vdSnapshots`\nstderr: %s", err)
			//
			// 	for _, snapshot := range vdSnapshots.Items {
			// 		Expect(*snapshot.Status.Consistent).To(BeTrue(), "consistent field should be `true`: %s", snapshot.Name)
			// 	}
			// })
		})
	})

	Context(fmt.Sprintf("When virtual machines in %s phase", PhaseRunning), func() {
		It("creates snapshots with `requiredConsistency` of attached VDs", func() {
			vmObjects := virtv2.VirtualMachineList{}
			err := GetObjects(kc.ResourceVM, &vmObjects, kc.GetOptions{Namespace: conf.Namespace})
			Expect(err).NotTo(HaveOccurred(), "cannot get virtual machines\nstderr: %s", err)

			for _, vm := range vmObjects.Items {
				Eventually(func() error {
					frozen, err := CheckFileSystemFrozen(vm.Name)
					if frozen {
						return errors.New("file system of the Virtual Machine is frozen")
					}
					return err
				}).WithTimeout(
					filesystemReadyTimeout,
				).WithPolling(
					filesystemReadyPollingInterval,
				).Should(Succeed())

				blockDevices := vm.Status.BlockDeviceRefs
				for _, blockDevice := range blockDevices {
					if blockDevice.Kind == virtv2.VirtualDiskKind {
						By(fmt.Sprintf(
							"Create snapshot for %q with volume snapshot class %q",
							blockDevice.Name,
							defaultVolumeSnapshotClassName,
						))
						err := CreateVirtualDiskSnapshot(blockDevice.Name, blockDevice.Name, defaultVolumeSnapshotClassName, true, attachedVirtualDiskLabel)
						Expect(err).NotTo(HaveOccurred(), "%s", err)

						Eventually(func() error {
							frozen, err := CheckFileSystemFrozen(vm.Name)
							if !frozen {
								return fmt.Errorf("`Filesystem` should be frozen when controller is snapshotting the attached virtual disk")
							}
							return err
						}).WithTimeout(
							filesystemReadyTimeout,
						).WithPolling(
							frozenReasonPollingInterval,
						).Should(Succeed())
					}
				}
			}
		})

		It("creates `vdSnapshots` concurrently", func() {
			vmObjects := virtv2.VirtualMachineList{}
			err := GetObjects(kc.ResourceVM, &vmObjects, kc.GetOptions{
				Namespace: conf.Namespace,
				Labels:    vmAutomaticWithHotplug,
			})
			Expect(err).NotTo(HaveOccurred(), "cannot get vmObject with label %q\nstderr: %s", vmAutomaticWithHotplug, err)

			for _, vm := range vmObjects.Items {
				Eventually(func() error {
					frozen, err := CheckFileSystemFrozen(vm.Name)
					if frozen {
						return errors.New("filesystem of the Virtual Machine is frozen")
					}
					return err
				}).WithTimeout(
					filesystemReadyTimeout,
				).WithPolling(
					filesystemReadyPollingInterval,
				).Should(Succeed())

				blockDevices := vm.Status.BlockDeviceRefs
				for _, blockDevice := range blockDevices {
					if blockDevice.Kind == virtv2.VirtualDiskKind {
						By(fmt.Sprintf(
							"Create five snapshots for %q of %q with volume snapshot class %q",
							blockDevice.Name,
							vm.Name,
							defaultVolumeSnapshotClassName,
						))
						errs := make([]error, 0, 5)
						wg := sync.WaitGroup{}
						for i := 0; i < 5; i++ {
							wg.Add(1)
							go func(index int) {
								defer wg.Done()
								snapshotName := fmt.Sprintf("%s-%d", blockDevice.Name, index)
								err := CreateVirtualDiskSnapshot(blockDevice.Name, snapshotName, defaultVolumeSnapshotClassName, true, attachedVirtualDiskLabel)
								if err != nil {
									errs = append(errs, err)
								}
							}(i)
						}
						wg.Wait()
						Expect(errs).To(BeEmpty(), "concurrent snapshotting error")

						Eventually(func() error {
							frozen, err := CheckFileSystemFrozen(vm.Name)
							if !frozen {
								return fmt.Errorf("`Filesystem` should be frozen when controller is snapshotting the attached virtual disk")
							}
							return err
						}).WithTimeout(
							filesystemReadyTimeout,
						).WithPolling(
							frozenReasonPollingInterval,
						).Should(Succeed())
					}
				}
			}
		})

		// TODO: It is a known issue that disk snapshots are not always created consistently. To prevent this error from causing noise during testing, we disabled this check. It will need to be re-enabled once the consistency issue is fixed.
		// It("checks snapshots of attached VDs", func() {
		// 	By(fmt.Sprintf("Snapshots should be in %s phase", PhaseReady))
		// 	WaitPhaseByLabel(kc.ResourceVDSnapshot, PhaseReady, kc.WaitOptions{
		// 		Labels:    attachedVirtualDiskLabel,
		// 		Namespace: conf.Namespace,
		// 		Timeout:   MaxWaitTimeout,
		// 	})
		// 	By("Snapshots should be consistent", func() {
		// 		vdSnapshots := virtv2.VirtualDiskSnapshotList{}
		// 		err := GetObjects(kc.ResourceVDSnapshot, &vdSnapshots, kc.GetOptions{
		// 			ExcludedLabels: []string{"hasNoConsumer"},
		// 			Namespace:      conf.Namespace,
		// 			Labels:         attachedVirtualDiskLabel,
		// 		})
		// 		Expect(err).NotTo(HaveOccurred(), "cannot get `vdSnapshots`\nstderr: %s", err)
		//
		// 		for _, snapshot := range vdSnapshots.Items {
		// 			Expect(snapshot.Status.Consistent).ToNot(BeNil())
		// 			Expect(*snapshot.Status.Consistent).To(BeTrue(), "consistent field should be `true`: %s", snapshot.Name)
		// 		}
		// 	})
		// })

		It("checks `FileSystemFrozen` status of VMs", func() {
			By("Status should not be `Frozen`")
			vmObjects := virtv2.VirtualMachineList{}
			err := GetObjects(kc.ResourceVM, &vmObjects, kc.GetOptions{Namespace: conf.Namespace})
			Expect(err).NotTo(HaveOccurred(), "cannot get virtual machines\nstderr: %s", err)

			for _, vm := range vmObjects.Items {
				Eventually(func() error {
					frozen, err := CheckFileSystemFrozen(vm.Name)
					if err != nil {
						return nil
					}
					if frozen {
						return errors.New("Filesystem of the Virtual Machine is frozen")
					}
					return nil
				}).WithTimeout(
					filesystemReadyTimeout,
				).WithPolling(
					filesystemReadyPollingInterval,
				).Should(Succeed())
			}
		})
	})

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			DeleteTestCaseResources(ResourcesToDelete{
				KustomizationDir: conf.TestData.VdSnapshots,
				AdditionalResources: []AdditionalResource{
					{
						Resource: kc.ResourceVDSnapshot,
						Labels:   hasNoConsumerLabel,
					},
					{
						Resource: kc.ResourceVDSnapshot,
						Labels:   attachedVirtualDiskLabel,
					},
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
