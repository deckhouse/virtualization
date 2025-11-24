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

package legacy

import (
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
	kc "github.com/deckhouse/virtualization/test/e2e/internal/kubectl"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	filesystemReadyTimeout         = 60 * time.Second
	filesystemReadyPollingInterval = 5 * time.Second
	frozenReasonPollingInterval    = 1 * time.Second
)

var _ = Describe("VirtualDiskSnapshots", Ordered, func() {
	var (
		testCaseLabel            = map[string]string{"testcase": "vd-snapshots", "id": namePrefix}
		attachedVirtualDiskLabel = map[string]string{"attachedVirtualDisk": ""}
		hasNoConsumerLabel       = map[string]string{"hasNoConsumer": "vd-snapshots"}
		vmAutomaticWithHotplug   = map[string]string{"vm": "automatic-with-hotplug"}
		ns                       string
	)

	BeforeAll(func() {
		kustomization := fmt.Sprintf("%s/%s", conf.TestData.VdSnapshots, "kustomization.yaml")
		var err error
		ns, err = kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)

		CreateNamespace(ns)

		Expect(conf.StorageClass.ImmediateStorageClass).NotTo(BeNil(), "immediate storage class cannot be nil; please set up the immediate storage class in the cluster")

		virtualDiskWithoutConsumer := v1alpha2.VirtualDisk{}
		vdWithoutConsumerFilePath := fmt.Sprintf("%s/vd/vd-ubuntu-http.yaml", conf.TestData.VdSnapshots)
		err = util.UnmarshalResource(vdWithoutConsumerFilePath, &virtualDiskWithoutConsumer)
		Expect(err).NotTo(HaveOccurred(), "cannot get object from file: %s\nstderr: %s", vdWithoutConsumerFilePath, err)

		virtualDiskWithoutConsumer.Spec.PersistentVolumeClaim.StorageClass = &conf.StorageClass.ImmediateStorageClass.Name
		err = util.WriteYamlObject(vdWithoutConsumerFilePath, &virtualDiskWithoutConsumer)
		Expect(err).NotTo(HaveOccurred(), "cannot update virtual disk with custom storage class: %s\nstderr: %s", vdWithoutConsumerFilePath, err)
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestCaseDump(testCaseLabel, CurrentSpecReport().LeafNodeText, ns)
		}
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
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual disks are applied:", func() {
		It("checks VDs phases", func() {
			By(fmt.Sprintf("VDs should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVD, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machines are applied:", func() {
		It("checks VMs phases", func() {
			By("Virtual machine agents should be ready")
			WaitVMAgentReady(kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machine block device attachments are applied:", func() {
		It("checks VMBDAs phases", func() {
			By(fmt.Sprintf("VMBDAs should be in %s phases", PhaseAttached))
			WaitPhaseByLabel(kc.ResourceVMBDA, PhaseAttached, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context(fmt.Sprintf("When unattached VDs in phase %s:", PhaseReady), func() {
		It("creates VDs snapshots with `requiredConsistency`", func() {
			res := kubectl.List(kc.ResourceVD, kc.GetOptions{
				Labels:    hasNoConsumerLabel,
				Namespace: ns,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			Expect(res.Error()).NotTo(HaveOccurred(), "cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())

			vds := strings.Split(res.StdOut(), " ")

			for _, vdName := range vds {
				By(fmt.Sprintf("Create snapshot for %q", vdName))
				labels := make(map[string]string)
				maps.Copy(labels, hasNoConsumerLabel)
				maps.Copy(labels, testCaseLabel)
				err := CreateVirtualDiskSnapshot(vdName, vdName, ns, true, labels)
				Expect(err).NotTo(HaveOccurred(), "%s", err)
			}
		})

		It("checks snapshots of unattached VDs", func() {
			By(fmt.Sprintf("Snapshots should be in %s phase", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVDSnapshot, PhaseReady, kc.WaitOptions{
				Labels:    hasNoConsumerLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})

			By("Snapshots should be consistent", func() {
				vdSnapshots := v1alpha2.VirtualDiskSnapshotList{}
				err := GetObjects(kc.ResourceVDSnapshot, &vdSnapshots, kc.GetOptions{Namespace: ns, Labels: hasNoConsumerLabel})
				Expect(err).NotTo(HaveOccurred(), "cannot get `vdSnapshots`\nstderr: %s", err)

				for _, snapshot := range vdSnapshots.Items {
					Expect(*snapshot.Status.Consistent).To(BeTrue(), "consistent field should be `true`: %s", snapshot.Name)
				}
			})
		})
	})

	Context(fmt.Sprintf("When virtual machines in %s phase", PhaseRunning), func() {
		It("creates snapshots with `requiredConsistency` of attached VDs", func() {
			vmObjects := v1alpha2.VirtualMachineList{}
			err := GetObjects(kc.ResourceVM, &vmObjects, kc.GetOptions{Namespace: ns})
			Expect(err).NotTo(HaveOccurred(), "cannot get virtual machines\nstderr: %s", err)

			for _, vm := range vmObjects.Items {
				Eventually(func() error {
					frozen, err := CheckFileSystemFrozen(vm.Name, ns)
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
					if blockDevice.Kind == v1alpha2.VirtualDiskKind {
						By(fmt.Sprintf("Create snapshot for %q", blockDevice.Name))
						labels := make(map[string]string)
						maps.Copy(labels, attachedVirtualDiskLabel)
						maps.Copy(labels, testCaseLabel)
						err := CreateVirtualDiskSnapshot(blockDevice.Name, blockDevice.Name, ns, true, labels)
						Expect(err).NotTo(HaveOccurred(), "%s", err)

						Eventually(func() error {
							frozen, err := CheckFileSystemFrozen(vm.Name, ns)
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
			vmObjects := v1alpha2.VirtualMachineList{}
			err := GetObjects(kc.ResourceVM, &vmObjects, kc.GetOptions{
				Namespace: ns,
				Labels:    vmAutomaticWithHotplug,
			})
			Expect(err).NotTo(HaveOccurred(), "cannot get vmObject with label %q\nstderr: %s", vmAutomaticWithHotplug, err)

			for _, vm := range vmObjects.Items {
				Eventually(func() error {
					frozen, err := CheckFileSystemFrozen(vm.Name, ns)
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
					if blockDevice.Kind == v1alpha2.VirtualDiskKind {
						By(fmt.Sprintf("Create five snapshots for %q of %q", blockDevice.Name, vm.Name))
						errs := make([]error, 0, 5)
						wg := sync.WaitGroup{}
						for i := range 5 {
							wg.Add(1)
							go func(index int) {
								defer wg.Done()
								snapshotName := fmt.Sprintf("%s-%d", blockDevice.Name, index)

								labels := make(map[string]string)
								maps.Copy(labels, attachedVirtualDiskLabel)
								maps.Copy(labels, testCaseLabel)
								err := CreateVirtualDiskSnapshot(blockDevice.Name, snapshotName, ns, true, labels)
								if err != nil {
									errs = append(errs, err)
								}
							}(i)
						}
						wg.Wait()
						Expect(errs).To(BeEmpty(), "should not face concurrent snapshotting error")

						Eventually(func() error {
							frozen, err := CheckFileSystemFrozen(vm.Name, ns)
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

		It("checks snapshots", func() {
			By("Snapshots should be `Ready`")
			labels := make(map[string]string)
			maps.Copy(labels, attachedVirtualDiskLabel)
			maps.Copy(labels, testCaseLabel)

			Eventually(func() error {
				vdSnapshots := GetVirtualDiskSnapshots(ns, labels)
				for _, snapshot := range vdSnapshots.Items {
					if snapshot.Status.Phase == v1alpha2.VirtualDiskSnapshotPhaseReady || snapshot.DeletionTimestamp != nil {
						continue
					}
					return errors.New("still wait for all snapshots either in ready or in deletion state")
				}
				return nil
			}).WithTimeout(
				LongWaitDuration,
			).WithPolling(
				Interval,
			).Should(Succeed(), "all snapshots should be in ready state after creation")
		})

		It("checks snapshots of attached VDs", func() {
			By(fmt.Sprintf("Snapshots should be in %s phase", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVDSnapshot, PhaseReady, kc.WaitOptions{
				Labels:    attachedVirtualDiskLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
			By("Snapshots should be consistent", func() {
				vdSnapshots := v1alpha2.VirtualDiskSnapshotList{}
				err := GetObjects(kc.ResourceVDSnapshot, &vdSnapshots, kc.GetOptions{
					ExcludedLabels: []string{"hasNoConsumer"},
					Namespace:      ns,
					Labels:         attachedVirtualDiskLabel,
				})
				Expect(err).NotTo(HaveOccurred(), "cannot get `vdSnapshots`\nstderr: %s", err)

				for _, snapshot := range vdSnapshots.Items {
					Expect(snapshot.Status.Consistent).ToNot(BeNil())
					Expect(*snapshot.Status.Consistent).To(BeTrue(), "consistent field should be `true`: %s", snapshot.Name)
				}
			})
		})

		It("checks `FileSystemFrozen` status of VMs", func() {
			By("Status should not be `Frozen`")
			vmObjects := v1alpha2.VirtualMachineList{}
			err := GetObjects(kc.ResourceVM, &vmObjects, kc.GetOptions{Namespace: ns})
			Expect(err).NotTo(HaveOccurred(), "cannot get virtual machines\nstderr: %s", err)

			for _, vm := range vmObjects.Items {
				Eventually(func() error {
					frozen, err := CheckFileSystemFrozen(vm.Name, vm.Namespace)
					if err != nil {
						return nil
					}
					if frozen {
						return fmt.Errorf("the filesystem of the virtual machine %s/%s is still frozen", vm.Namespace, vm.Name)
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
			if config.IsCleanUpNeeded() {
				DeleteTestCaseResources(ns, ResourcesToDelete{
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
					},
				})
			}
		})
	})
})

func CreateVirtualDiskSnapshot(vdName, snapshotName, namespace string, requiredConsistency bool, labels map[string]string) error {
	GinkgoHelper()
	vdSnapshot := v1alpha2.VirtualDiskSnapshot{
		TypeMeta: metav1.TypeMeta{
			APIVersion: APIVersion,
			Kind:       v1alpha2.VirtualDiskSnapshotKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Labels:    labels,
			Name:      snapshotName,
			Namespace: namespace,
		},
		Spec: v1alpha2.VirtualDiskSnapshotSpec{
			RequiredConsistency: requiredConsistency,
			VirtualDiskName:     vdName,
		},
	}

	filePath := fmt.Sprintf("%s/snapshots/%s.yaml", conf.TestData.VdSnapshots, snapshotName)
	err := util.WriteYamlObject(filePath, &vdSnapshot)
	if err != nil {
		return fmt.Errorf("cannot write file with virtual disk snapshot: %s\nstderr: %w", snapshotName, err)
	}

	res := kubectl.Apply(kc.ApplyOptions{
		Filename:       []string{filePath},
		FilenameOption: kc.Filename,
	})
	if res.Error() != nil {
		return fmt.Errorf("cannot create virtual disk snapshot: %s\nstderr: %s", snapshotName, res.StdErr())
	}
	return nil
}

func GetVirtualDiskSnapshots(namespace string, labels map[string]string) v1alpha2.VirtualDiskSnapshotList {
	GinkgoHelper()
	vdSnapshots := v1alpha2.VirtualDiskSnapshotList{}
	err := GetObjects(kc.ResourceVDSnapshot, &vdSnapshots, kc.GetOptions{
		ExcludedLabels: []string{"hasNoConsumer"},
		Namespace:      namespace,
		Labels:         labels,
	})
	Expect(err).NotTo(HaveOccurred(), "cannot get `vdSnapshots`\nstderr: %s", err)
	return vdSnapshots
}

func CheckFileSystemFrozen(vmName, vmNamespace string) (bool, error) {
	vmObj := v1alpha2.VirtualMachine{}
	err := GetObject(kc.ResourceVM, vmName, &vmObj, kc.GetOptions{Namespace: vmNamespace})
	if err != nil {
		return false, fmt.Errorf("cannot get `VirtualMachine`: %q\nstderr: %w", vmName, err)
	}

	for _, condition := range vmObj.Status.Conditions {
		if condition.Type == vmcondition.TypeFilesystemFrozen.String() {
			return condition.Status == metav1.ConditionTrue, nil
		}
	}

	return false, nil
}
