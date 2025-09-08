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

package e2e

import (
	"context"
	"fmt"
	"os"
	"path"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/d8"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

var _ = Describe("VirtualMachineVolumeMigration", SIGMigration(), ginkgoutil.FailureBehaviourEnvSwitcherDecorator(), func() {

	BeforeEach(func() {
		sc := conf.StorageClass.TemplateStorageClass
		if sc == nil || (sc.VolumeBindingMode == nil || *sc.VolumeBindingMode != storagev1.VolumeBindingWaitForFirstConsumer) {
			Skip("Default StorageClass is not set to WaitForFirstConsumer volume binding mode.")
		}
	})

	beforeEach := func(namespace *string, testCaseLabel map[string]string, testdata string) {
		Expect(namespace).NotTo(BeNil())
		BeforeEach(func() {
			kustomization := path.Join(testdata, "kustomization.yaml")
			ns, err := kustomize.GetNamespace(kustomization)
			*namespace = ns
			Expect(err).NotTo(HaveOccurred(), "%w", err)

			// Wait until namespace is deleted
			Eventually(func() error {
				_, err := kubeClient.CoreV1().Namespaces().Get(context.Background(), *namespace, metav1.GetOptions{})
				if err != nil {
					if k8serrors.IsNotFound(err) {
						return nil
					}
					return err
				}
				return fmt.Errorf("namespace %s is not deleted", *namespace)
			}).WithTimeout(LongWaitDuration).WithPolling(time.Second).ShouldNot(HaveOccurred())

			CreateNamespace(ns)
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{testdata},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})
	}

	afterEach := func(namespace *string, testCaseLabel map[string]string, testdata string) {
		Expect(namespace).NotTo(BeNil())
		AfterEach(func() {
			if CurrentSpecReport().Failed() {
				SaveTestResources(testCaseLabel, CurrentSpecReport().LeafNodeText)
			}
			resourcesToDelete := ResourcesToDelete{
				KustomizationDir: testdata,
				AdditionalResources: []AdditionalResource{
					{
						Resource: kc.ResourceVMOP,
						Labels:   testCaseLabel,
					},
				},
			}

			DeleteTestCaseResources(*namespace, resourcesToDelete)

			err := kubeClient.CoreV1().Namespaces().Delete(context.Background(), *namespace, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
		})
	}

	When("LocalDiskBlank", SIGMigration(), ginkgoutil.CommonE2ETestDecorators(), func() {
		var (
			testCaseLabel = map[string]string{"testcase": "vm-volume-migration-local-disk-blank"}
			testdata      = conf.TestData.VMVolumeMigrationLocalDiskBlank
			namespace     = ptr.To("")
		)

		beforeEach(namespace, testCaseLabel, testdata)
		afterEach(namespace, testCaseLabel, testdata)

		lvmShouldBeSuccessful(namespace, testCaseLabel)

		lvmShouldBeReverted(namespace, testCaseLabel)

		// lvmShouldBeRevertedLiveMigrationNotStarted(namespace, testCaseLabel)

		lvmStorageClassChangedContext(namespace, testCaseLabel)
	})

	When("LocalDiskRoot", SIGMigration(), ginkgoutil.CommonE2ETestDecorators(), func() {
		var (
			testCaseLabel = map[string]string{"testcase": "vm-volume-migration-local-disk-root"}
			testdata      = conf.TestData.VMVolumeMigrationLocalDiskRoot
			namespace     = ptr.To("")
		)

		beforeEach(namespace, testCaseLabel, testdata)
		afterEach(namespace, testCaseLabel, testdata)

		lvmShouldBeSuccessful(namespace, testCaseLabel)

		lvmShouldBeReverted(namespace, testCaseLabel)

		// lvmShouldBeRevertedLiveMigrationNotStarted(namespace, testCaseLabel)

		lvmStorageClassChangedContext(namespace, testCaseLabel)
	})

	When("LocalDisks", SIGMigration(), ginkgoutil.CommonE2ETestDecorators(), func() {
		var (
			testCaseLabel = map[string]string{"testcase": "vm-volume-migration-local-disks"}
			testdata      = conf.TestData.VMVolumeMigrationLocalDisks
			namespace     = ptr.To("")
		)

		beforeEach(namespace, testCaseLabel, testdata)
		afterEach(namespace, testCaseLabel, testdata)

		lvmShouldBeSuccessful(namespace, testCaseLabel)

		lvmShouldBeReverted(namespace, testCaseLabel)

		// lvmShouldBeRevertedLiveMigrationNotStarted(namespace, testCaseLabel)

		lvmStorageClassChangedContext(namespace, testCaseLabel)
	})
})

func lvmShouldBeSuccessful(namespace *string, labels map[string]string) {
	GinkgoHelper()

	// namespace should be dereferencing only in It() block

	It("should volume migration successfully", func() {

		Expect(namespace).NotTo(BeNil())
		ns := *namespace
		Expect(ns).NotTo(BeEmpty())

		listOptions := metav1.ListOptions{
			LabelSelector: metav1.FormatLabelSelector(&metav1.LabelSelector{
				MatchLabels: labels,
			}),
		}

		By("Virtual machine agents should be ready")
		WaitVMAgentReady(kc.WaitOptions{
			Labels:    labels,
			Namespace: ns,
			Timeout:   LongWaitDuration,
		})

		vms, err := virtClient.VirtualMachines(ns).List(context.Background(), listOptions)
		Expect(err).NotTo(HaveOccurred())
		Expect(vms.Items).NotTo(BeEmpty())

		vmNames := make([]string, len(vms.Items))
		for i, vm := range vms.Items {
			vmNames[i] = vm.GetName()
		}

		By("Starting migrations for virtual machines")
		MigrateVirtualMachines(labels, ns, vmNames...)

		By(fmt.Sprintf("VMOPs should be in %s phases", v1alpha2.VMOPPhaseCompleted))
		WaitPhaseByLabel(kc.ResourceVMOP, string(v1alpha2.VMOPPhaseCompleted), kc.WaitOptions{
			Labels:    labels,
			Namespace: ns,
			Timeout:   LongWaitDuration,
		})

		By("Virtual machines should be migrated")
		WaitByLabel(kc.ResourceVM, kc.WaitOptions{
			Labels:    labels,
			Namespace: ns,
			Timeout:   LongWaitDuration,
			For:       "'jsonpath={.status.migrationState.result}=Succeeded'",
		})

		waitVirtualDisksMigrationsSucceeded(ns, listOptions)
	})
}

func lvmShouldBeReverted(namespace *string, labels map[string]string) {
	GinkgoHelper()

	// namespace should be dereferencing only in It() block

	It("should be reverted", func() {

		Expect(namespace).NotTo(BeNil())
		ns := *namespace
		Expect(ns).NotTo(BeEmpty())

		listOptions := metav1.ListOptions{
			LabelSelector: metav1.FormatLabelSelector(&metav1.LabelSelector{
				MatchLabels: labels,
			}),
		}

		By("Virtual machine agents should be ready")
		WaitVMAgentReady(kc.WaitOptions{
			Labels:    labels,
			Namespace: ns,
			Timeout:   LongWaitDuration,
		})

		vms := &v1alpha2.VirtualMachineList{}
		err := GetObjects(kc.ResourceVM, vms, kc.GetOptions{Labels: labels, Namespace: ns})
		Expect(err).NotTo(HaveOccurred())
		Expect(vms.Items).NotTo(BeEmpty())

		vmNames := make([]string, len(vms.Items))
		for i, vm := range vms.Items {
			vmNames[i] = vm.GetName()
		}

		execStressNG(vmNames, ns)

		By("Starting migrations for virtual machines")
		MigrateVirtualMachines(labels, ns, vmNames...)

		waitVirtualMachinesWillBeStartMigratingAndCancelImmediately(ns, listOptions)

		waitVirtualDisksMigrationsFailed(ns, listOptions)
	})
}

func lvmShouldBeRevertedLiveMigrationNotStarted(namespace *string, labels map[string]string) {
	GinkgoHelper()

	// namespace should be dereferencing only in It() block

	It("should be reverted live migration not started", func() {

		ns := *namespace
		Expect(ns).NotTo(BeEmpty())

		listOptions := metav1.ListOptions{
			LabelSelector: metav1.FormatLabelSelector(&metav1.LabelSelector{
				MatchLabels: labels,
			}),
		}

		By("Virtual machine agents should be ready")
		WaitVMAgentReady(kc.WaitOptions{
			Labels:    labels,
			Namespace: ns,
			Timeout:   LongWaitDuration,
		})

		vms, err := virtClient.VirtualMachines(ns).List(context.Background(), listOptions)
		Expect(err).NotTo(HaveOccurred())
		Expect(vms.Items).NotTo(BeEmpty())

		vmNames := make([]string, len(vms.Items))
		for i, vm := range vms.Items {
			vmNames[i] = vm.GetName()
		}

		By("Patch VMs with doesn't exist nodeSelector")
		for _, vm := range vms.Items {
			mergePatch := []byte(`{"spec":{"nodeSelector":{"notExistNodeSelector":"true"}}}`)
			_, err = virtClient.VirtualMachines(vm.GetNamespace()).Patch(context.Background(), vm.GetName(), types.MergePatchType, mergePatch, metav1.PatchOptions{})
			Expect(err).NotTo(HaveOccurred())
		}

		By("Wait node selector sync")
		Eventually(func() error {
			kvvmis := &virtv1.VirtualMachineInstanceList{}
			err = GetObjects(kc.ResourceVM, kvvmis, kc.GetOptions{Labels: labels, Namespace: ns})
			if err != nil {
				return err
			}

			if len(kvvmis.Items) != len(vms.Items) {
				return fmt.Errorf("unexpected number of kvvmis, expected %d, got %d", len(vms.Items), len(kvvmis.Items))
			}

			for _, kvvmi := range kvvmis.Items {
				if kvvmi.Spec.NodeSelector["notExistNodeSelector"] != "true" {
					return fmt.Errorf("unexpected node selector value, expected %s, got %s", "true", kvvmi.Spec.NodeSelector["notExistNodeSelector"])
				}
			}

			return nil
		})

		By("Migrate virtual machines")
		MigrateVirtualMachines(labels, ns, vmNames...)

		vmops, err := virtClient.VirtualMachineOperations(ns).List(context.Background(), listOptions)
		Expect(err).NotTo(HaveOccurred())
		vmopsByName := make(map[string]*v1alpha2.VirtualMachineOperation, len(vmops.Items))
		for _, vmop := range vmops.Items {
			vmopsByName[vmop.Name] = &vmop
		}

		By("Wait when migrations will be created")
		Eventually(func() error {
			migList, err := listMigrations(ns, metav1.ListOptions{})
			if err != nil {
				return err
			}

			migrationCount := 0
			for _, mig := range migList.Items {
				owner := metav1.GetControllerOf(&mig)
				if _, ok := vmopsByName[owner.Name]; ok {
					migrationCount++
				}
			}

			if migrationCount != len(vmops.Items) {
				return fmt.Errorf("unexpected number of migrations, expected %d, got %d", len(vmops.Items), len(migList.Items))
			}

			return nil

		}).WithPolling(time.Second).WithTimeout(LongWaitDuration).Should(Succeed())

		By("Cancel migrations immediately")
		Eventually(func() error {
			migList, err := listMigrations(ns, metav1.ListOptions{})
			if err != nil {
				return err
			}

			if len(migList.Items) == 0 {
				// all migrations were canceled
				return nil
			}

			for _, mig := range migList.Items {
				owner := metav1.GetControllerOf(&mig)
				if _, ok := vmopsByName[owner.Name]; !ok {
					continue
				}

				err = virtClient.VirtualMachineOperations(ns).Delete(context.Background(), owner.Name, metav1.DeleteOptions{})
				if err != nil {
					return err
				}
			}

			return fmt.Errorf("all migrations should be cancelled")

		}).WithPolling(time.Second).WithTimeout(ShortWaitDuration).Should(Succeed())

		waitVirtualDisksMigrationsFailed(ns, listOptions)
	})
}

func lvmStorageClassChangedContext(namespace *string, labels map[string]string) {
	GinkgoHelper()

	// namespace should be dereferencing only in It() block

	Context("StorageClassChanged", func() {
		var (
			usedStorageClass = conf.StorageClass.TemplateStorageClass.Name
			nextStorageClass string
		)
		BeforeEach(func() {
			if env, ok := os.LookupEnv("E2E_VOLUME_MIGRATION_NEXT_STORAGE_CLASS"); ok {
				nextStorageClass = env
			} else {
				scList, err := kubeClient.StorageV1().StorageClasses().List(context.Background(), metav1.ListOptions{})
				Expect(err).NotTo(HaveOccurred())

				for _, sc := range scList.Items {
					if sc.Name == usedStorageClass {
						continue
					}
					if sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer {
						nextStorageClass = sc.Name
						break
					}
				}
			}

			if nextStorageClass == "" {
				Skip("No available storage class for test")
			}
		})

		It("should be successful", func(ctx context.Context) {

			ns := *namespace
			Expect(ns).NotTo(BeEmpty())

			listOptions := metav1.ListOptions{
				LabelSelector: metav1.FormatLabelSelector(&metav1.LabelSelector{
					MatchLabels: labels,
				}),
			}

			By("Virtual machine agents should be ready")
			WaitVMAgentReady(kc.WaitOptions{
				Labels:    labels,
				Namespace: ns,
				Timeout:   LongWaitDuration,
			})

			vms, err := virtClient.VirtualMachines(ns).List(ctx, listOptions)
			Expect(err).NotTo(HaveOccurred())
			Expect(vms.Items).NotTo(BeEmpty())

			vmNames := make([]string, len(vms.Items))
			for i, vm := range vms.Items {
				vmNames[i] = vm.GetName()
			}

			vds, err := virtClient.VirtualDisks(ns).List(ctx, listOptions)
			Expect(err).NotTo(HaveOccurred())
			Expect(vds.Items).NotTo(BeEmpty())

			vdForPatch := vds.Items[0]

			By("Patch VD with new storage class")
			patchBytes, err := patch.NewJSONPatch(patch.WithReplace("/spec/persistentVolumeClaim/storageClassName", nextStorageClass)).Bytes()
			Expect(err).NotTo(HaveOccurred())

			_, err = virtClient.VirtualDisks(vdForPatch.GetNamespace()).Patch(ctx, vdForPatch.GetName(), types.JSONPatchType, patchBytes, metav1.PatchOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("Virtual machines should be migrated")
			WaitByLabel(kc.ResourceVM, kc.WaitOptions{
				Labels:    labels,
				Namespace: ns,
				Timeout:   LongWaitDuration,
				For:       "'jsonpath={.status.migrationState.result}=Succeeded'",
			})

			waitVirtualDisksMigrationsSucceeded(ns, listOptions)

			newVdForPatch, err := virtClient.VirtualDisks(ns).Get(ctx, vdForPatch.GetName(), metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			pvc, err := kubeClient.CoreV1().PersistentVolumeClaims(ns).Get(ctx, newVdForPatch.Status.Target.PersistentVolumeClaim, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(pvc.Spec.StorageClassName).NotTo(BeNil())
			Expect(*pvc.Spec.StorageClassName).To(Equal(nextStorageClass))
			Expect(pvc.Status.Phase).To(Equal(corev1.ClaimBound))
		})

		It("should be reverted", func(ctx context.Context) {

			ns := *namespace
			Expect(ns).NotTo(BeEmpty())

			listOptions := metav1.ListOptions{
				LabelSelector: metav1.FormatLabelSelector(&metav1.LabelSelector{
					MatchLabels: labels,
				}),
			}

			By("Virtual machine agents should be ready")
			WaitVMAgentReady(kc.WaitOptions{
				Labels:    labels,
				Namespace: ns,
				Timeout:   LongWaitDuration,
			})

			vms, err := virtClient.VirtualMachines(ns).List(ctx, listOptions)
			Expect(err).NotTo(HaveOccurred())
			Expect(vms.Items).NotTo(BeEmpty())

			vmNames := make([]string, len(vms.Items))
			for i, vm := range vms.Items {
				vmNames[i] = vm.GetName()
			}

			execStressNG(vmNames, ns)

			vds, err := virtClient.VirtualDisks(ns).List(ctx, listOptions)
			Expect(err).NotTo(HaveOccurred())
			Expect(vds.Items).NotTo(BeEmpty())

			vdForPatch := vds.Items[0]

			By("Patch VD with new storage class")
			patchBytes, err := patch.NewJSONPatch(patch.WithReplace("/spec/persistentVolumeClaim/storageClassName", nextStorageClass)).Bytes()
			Expect(err).NotTo(HaveOccurred())

			_, err = virtClient.VirtualDisks(vdForPatch.GetNamespace()).Patch(ctx, vdForPatch.GetName(), types.JSONPatchType, patchBytes, metav1.PatchOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitVirtualMachinesWillBeStartMigratingAndCancelImmediately(ns, listOptions)

			waitVirtualDisksMigrationsFailed(ns, listOptions)
		})
	})
}

func listMigrations(namespace string, listOptions metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return dynamicClient.Resource(schema.GroupVersionResource{
		Group:    "internal.virtualization.deckhouse.io",
		Version:  "v1",
		Resource: "internalvirtualizationvirtualmachineinstancemigrations",
	}).Namespace(namespace).List(context.Background(), listOptions)
}

func waitVirtualMachinesWillBeStartMigratingAndCancelImmediately(namespace string, listOptions metav1.ListOptions) {
	GinkgoHelper()

	someCompleted := false

	By("wait when migrations will be start migrating")
	Eventually(func() error {
		vmops, err := virtClient.VirtualMachineOperations(namespace).List(context.Background(), listOptions)
		if err != nil {
			return err
		}

		if len(vmops.Items) == 0 {
			// All migrations were be canceled
			return nil
		}

		vms, err := virtClient.VirtualMachines(namespace).List(context.Background(), listOptions)
		if err != nil {
			return err
		}

		vmsByName := make(map[string]*v1alpha2.VirtualMachine, len(vms.Items))
		for _, vm := range vms.Items {
			vmsByName[vm.Name] = &vm
		}

		migrationReady := make(map[string]struct{})
		for _, vmop := range vmops.Items {
			if vm := vmsByName[vmop.Spec.VirtualMachine]; vm != nil {
				if vm.Status.MigrationState != nil && !vm.Status.MigrationState.StartTimestamp.IsZero() && vm.Status.MigrationState.EndTimestamp.IsZero() {
					migrationReady[vmop.Name] = struct{}{}
				}
			}
		}

		for _, vmop := range vmops.Items {
			switch vmop.Status.Phase {
			case v1alpha2.VMOPPhaseInProgress:
				_, readyToDelete := migrationReady[vmop.Name]

				if readyToDelete && vmop.GetDeletionTimestamp().IsZero() {
					err = virtClient.VirtualMachineOperations(vmop.GetNamespace()).Delete(context.Background(), vmop.GetName(), metav1.DeleteOptions{})
					if err != nil {
						return err
					}
				}
			case v1alpha2.VMOPPhaseFailed, v1alpha2.VMOPPhaseCompleted:
				someCompleted = true
				return nil
			}
		}
		return fmt.Errorf("retry because not all vmops canceled")
	}).WithTimeout(LongWaitDuration).WithPolling(time.Second).ShouldNot(HaveOccurred())

	Expect(someCompleted).Should(BeFalse())
}

func waitVirtualDisksMigrationsFailed(namespace string, listOptions metav1.ListOptions) {
	GinkgoHelper()

	var (
		vds *v1alpha2.VirtualDiskList
		err error
	)

	By("Wait until VirtualDisks migrations failed")
	Eventually(func() error {
		vds, err = virtClient.VirtualDisks(namespace).List(context.Background(), listOptions)
		if err != nil {
			return err
		}

		for _, vd := range vds.Items {
			if vd.Status.MigrationState.EndTimestamp.IsZero() {
				return fmt.Errorf("migration is not completed for vd %s", vd.Name)
			}
		}
		return nil
	}).WithTimeout(LongWaitDuration).WithPolling(time.Second).ShouldNot(HaveOccurred())

	Expect(vds).ShouldNot(BeNil())
	Expect(vds.Items).ShouldNot(BeEmpty())

	for _, vd := range vds.Items {
		Expect(vd.Status.MigrationState.EndTimestamp.IsZero()).Should(BeFalse())
		Expect(vd.Status.MigrationState.SourcePVC).Should(Equal(vd.Status.Target.PersistentVolumeClaim))
		Expect(vd.Status.MigrationState.TargetPVC).ShouldNot(BeEmpty())
		Expect(vd.Status.MigrationState.Result).Should(Equal(v1alpha2.VirtualDiskMigrationResultFailed))
	}
}

func waitVirtualDisksMigrationsSucceeded(namespace string, listOptions metav1.ListOptions) {
	GinkgoHelper()

	By("Wait until VirtualDisks migrations succeeded")
	Eventually(func() error {
		vds, err := virtClient.VirtualDisks(namespace).List(context.Background(), listOptions)
		if err != nil {
			return err
		}
		for _, vd := range vds.Items {
			if vd.Status.Phase != v1alpha2.DiskReady {
				return fmt.Errorf("vd %s is not ready", vd.Name)
			}
			if vd.Status.Target.PersistentVolumeClaim == "" {
				return fmt.Errorf("vd %s has no target PVC", vd.Name)
			}
			if vd.Status.MigrationState.StartTimestamp.IsZero() {
				// Skip the disks that are not migrated
				continue
			}
			if vd.Status.Target.PersistentVolumeClaim != vd.Status.MigrationState.TargetPVC {
				return fmt.Errorf("vd %s target PVC is not equal to migration target PVC", vd.Name)
			}
			if vd.Status.MigrationState.Result != v1alpha2.VirtualDiskMigrationResultSucceeded {
				return fmt.Errorf("vd %s migration failed", vd.Name)
			}
		}
		return nil
	}).WithTimeout(LongWaitDuration).WithPolling(time.Second).ShouldNot(HaveOccurred())
}

func execStressNG(vmNames []string, namespace string) {
	GinkgoHelper()

	for _, name := range vmNames {
		By(fmt.Sprintf("Exec SSHCommand for virtualmachine %s/%s", namespace, name))
		res := d8Virtualization.SSHCommand(name, "sudo nohup stress-ng --vm 1 --vm-bytes 100% --timeout 300s &>/dev/null &", d8.SSHOptions{
			Namespace:   namespace,
			Username:    conf.TestData.SSHUser,
			IdenityFile: conf.TestData.Sshkey,
			Timeout:     ShortTimeout,
		})
		Expect(res.WasSuccess()).To(BeTrue(), res.StdErr())
	}

	By("Wait until stress-ng loads the memory more heavily")
	time.Sleep(20 * time.Second)
}
