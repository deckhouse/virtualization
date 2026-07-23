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

package vm

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// cloudInitVolumeName is the name of the cloud-init volume and disk in the
// underlying KubeVirt VMI (see kvbuilder.CloudInitDiskName).
const cloudInitVolumeName = "cloudinit"

// sysprepVolumeName is the name of the sysprep volume and disk in the
// underlying KubeVirt VMI (see kvbuilder.SysprepDiskName).
const sysprepVolumeName = "sysprep"

var _ = Describe("VirtualMachineProvisioningRemoval", Label(precheck.NoPrecheck), func() {
	It("should remove cloud-init provisioning from a running VM without a restart", func() {
		ctx := context.Background()
		f := framework.NewFramework("vm-provisioning-removal")
		DeferCleanup(f.After)
		f.Before()

		t := newProvisioningRemovalTest(f)

		By("Environment preparation")
		t.GenerateResources()
		err := f.CreateWithDeferredDeletion(ctx, t.VM, t.VDRoot)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting until VM agent is ready")
		util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)

		By("Checking the running VMI carries the cloud-init disk")
		kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, t.VM)
		Expect(err).NotTo(HaveOccurred())
		Expect(kvvmi).NotTo(BeNil())
		Expect(vmiHasVolume(kvvmi, cloudInitVolumeName)).To(BeTrue(), "cloud-init volume should be present before removal")
		Expect(vmiHasDisk(kvvmi, cloudInitVolumeName)).To(BeTrue(), "cloud-init disk should be present before removal")
		vmiUID := kvvmi.UID

		initialNode, err := util.GetVMNode(ctx, f, t.VM)
		Expect(err).NotTo(HaveOccurred())

		By("Removing provisioning from the VM spec")
		patchset := patch.NewJSONPatch(patch.WithRemove("/spec/provisioning"))
		patchBytes, err := patchset.Bytes()
		Expect(err).NotTo(HaveOccurred())
		t.VM, err = f.VirtClient().VirtualMachines(t.VM.Namespace).Patch(ctx, t.VM.Name, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("Waiting until the cloud-init disk is hot-detached from the running VMI")
		Eventually(func(g Gomega) {
			kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, t.VM)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(kvvmi).NotTo(BeNil())
			g.Expect(vmiHasVolume(kvvmi, cloudInitVolumeName)).To(BeFalse(), "cloud-init volume should be detached")
			g.Expect(vmiHasDisk(kvvmi, cloudInitVolumeName)).To(BeFalse(), "cloud-init disk should be detached")
			g.Expect(kvvmi.UID).To(Equal(vmiUID), "VMI must not be recreated (no restart)")
		}).WithTimeout(framework.MiddleTimeout).WithPolling(time.Second).Should(Succeed())

		By("Checking that no restart is required and the VM was not restarted")
		Consistently(func(g Gomega) {
			err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
			g.Expect(err).NotTo(HaveOccurred())
			_, exists := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, t.VM.Status.Conditions)
			g.Expect(exists).To(BeFalse(), "removing cloud-init must not require a restart")
			g.Expect(t.VM.Status.RestartAwaitingChanges).To(BeNil())

			kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, t.VM)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(kvvmi).NotTo(BeNil())
			g.Expect(kvvmi.UID).To(Equal(vmiUID), "VMI must not be recreated (no restart)")
		}).WithTimeout(framework.ShortTimeout).WithPolling(time.Second).Should(Succeed())

		util.ExpectNoVMOperationsForVirtualMachine(ctx, f, t.VM)
		util.ExpectVMOnNode(ctx, f, t.VM, initialNode)

		By("Checking the VM is still reachable over SSH")
		util.UntilSSHReady(f, t.VM, framework.ShortTimeout)
	})

	It("should remove a UserDataRef provisioning block and allow the referenced Secret to be deleted", func() {
		ctx := context.Background()
		f := framework.NewFramework("vm-provisioning-userdataref-removal")
		DeferCleanup(f.After)
		f.Before()

		t := newProvisioningRemovalTest(f)

		By("Environment preparation: VM with UserDataRef provisioning referencing a Secret")
		t.GenerateResourcesWithUserDataRef()
		// The Secret must exist before the VM controller reconciles, otherwise
		// the VMI is created without a cloud-init volume.
		err := f.CreateWithDeferredDeletion(ctx, t.UserDataSecret, t.VDRoot, t.VM)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting until VM agent is ready")
		util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)

		By("Checking the running VMI carries the cloud-init disk backed by the Secret")
		kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, t.VM)
		Expect(err).NotTo(HaveOccurred())
		Expect(kvvmi).NotTo(BeNil())
		Expect(vmiHasVolume(kvvmi, cloudInitVolumeName)).To(BeTrue(), "cloud-init volume should be present before removal")
		Expect(vmiHasDisk(kvvmi, cloudInitVolumeName)).To(BeTrue(), "cloud-init disk should be present before removal")
		vmiUID := kvvmi.UID

		By("Removing provisioning from the VM spec")
		patchset := patch.NewJSONPatch(patch.WithRemove("/spec/provisioning"))
		patchBytes, err := patchset.Bytes()
		Expect(err).NotTo(HaveOccurred())
		t.VM, err = f.VirtClient().VirtualMachines(t.VM.Namespace).Patch(ctx, t.VM.Name, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("Waiting until the cloud-init disk is hot-detached from the running VMI")
		Eventually(func(g Gomega) {
			kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, t.VM)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(kvvmi).NotTo(BeNil())
			g.Expect(vmiHasVolume(kvvmi, cloudInitVolumeName)).To(BeFalse(), "cloud-init volume should be detached")
			g.Expect(vmiHasDisk(kvvmi, cloudInitVolumeName)).To(BeFalse(), "cloud-init disk should be detached")
			g.Expect(kvvmi.UID).To(Equal(vmiUID), "VMI must not be recreated (no restart)")
		}).WithTimeout(framework.MiddleTimeout).WithPolling(time.Second).Should(Succeed())

		By("Checking that no restart is required")
		Consistently(func(g Gomega) {
			err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
			g.Expect(err).NotTo(HaveOccurred())
			_, exists := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, t.VM.Status.Conditions)
			g.Expect(exists).To(BeFalse(), "removing cloud-init must not require a restart")
			g.Expect(t.VM.Status.RestartAwaitingChanges).To(BeNil())
		}).WithTimeout(framework.ShortTimeout).WithPolling(time.Second).Should(Succeed())

		By("Deleting the referenced Secret after the cloud-init disk has been detached")
		// Once the cloud-init volume is gone from the running VMI, the Secret
		// is no longer referenced and must be removable without blocking the
		// VM (no dangling finalizer, no reconciliation error).
		err = f.Delete(ctx, t.UserDataSecret)
		Expect(err).NotTo(HaveOccurred())

		By("Checking the VM stays Running and is not restarted after the Secret is gone")
		Consistently(func(g Gomega) {
			kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, t.VM)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(kvvmi).NotTo(BeNil())
			g.Expect(kvvmi.UID).To(Equal(vmiUID), "VMI must not be recreated (no restart)")
		}).WithTimeout(framework.ShortTimeout).WithPolling(time.Second).Should(Succeed())

		util.ExpectNoVMOperationsForVirtualMachine(ctx, f, t.VM)

		By("Checking the VM is still reachable over SSH")
		util.UntilSSHReady(f, t.VM, framework.ShortTimeout)
	})

	It("should require a restart and keep the disk when a Sysprep provisioning block is removed", func() {
		ctx := context.Background()
		f := framework.NewFramework("vm-provisioning-sysprep-removal")
		DeferCleanup(f.After)
		f.Before()

		t := newProvisioningRemovalTest(f)

		By("Environment preparation: VM with SysprepRef provisioning referencing a Secret")
		t.GenerateResourcesWithSysprepRef()
		err := f.CreateWithDeferredDeletion(ctx, t.SysprepSecret, t.VDRoot, t.VM)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting until the VM is Running")
		util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, t.VM)

		By("Checking the running VMI carries the sysprep disk")
		kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, t.VM)
		Expect(err).NotTo(HaveOccurred())
		Expect(kvvmi).NotTo(BeNil())
		Expect(vmiHasVolume(kvvmi, sysprepVolumeName)).To(BeTrue(), "sysprep volume should be present before removal")
		Expect(vmiHasDisk(kvvmi, sysprepVolumeName)).To(BeTrue(), "sysprep disk should be present before removal")
		vmiUID := kvvmi.UID

		By("Removing provisioning from the VM spec")
		patchset := patch.NewJSONPatch(patch.WithRemove("/spec/provisioning"))
		patchBytes, err := patchset.Bytes()
		Expect(err).NotTo(HaveOccurred())
		t.VM, err = f.VirtClient().VirtualMachines(t.VM.Namespace).Patch(ctx, t.VM.Name, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("Checking that AwaitingRestartToApplyConfiguration appears and the VM is not restarted")
		Consistently(func(g Gomega) {
			err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
			g.Expect(err).NotTo(HaveOccurred())
			cond, exists := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, t.VM.Status.Conditions)
			g.Expect(exists).To(BeTrue(), "removing sysprep must require a restart")
			g.Expect(cond.Status).To(Equal(metav1.ConditionTrue), "AwaitingRestartToApplyConfiguration must be True for sysprep removal")
			g.Expect(t.VM.Status.RestartAwaitingChanges).NotTo(BeNil())

			// The VM is in Manual restart-approval mode, so it must not be
			// restarted automatically and the VMI must stay the same.
			kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, t.VM)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(kvvmi).NotTo(BeNil())
			g.Expect(kvvmi.UID).To(Equal(vmiUID), "VMI must not be recreated (manual restart approval)")
		}).WithTimeout(framework.ShortTimeout).WithPolling(time.Second).Should(Succeed())

		By("Checking the sysprep disk is NOT detached from the running VMI")
		Consistently(func(g Gomega) {
			kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, t.VM)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(kvvmi).NotTo(BeNil())
			g.Expect(vmiHasVolume(kvvmi, sysprepVolumeName)).To(BeTrue(), "sysprep volume must not be detached live")
			g.Expect(vmiHasDisk(kvvmi, sysprepVolumeName)).To(BeTrue(), "sysprep disk must not be detached live")
			g.Expect(kvvmi.UID).To(Equal(vmiUID), "VMI must not be recreated (no restart)")
		}).WithTimeout(framework.ShortTimeout).WithPolling(time.Second).Should(Succeed())

		util.ExpectNoVMOperationsForVirtualMachine(ctx, f, t.VM)
	})
})

type provisioningRemovalTest struct {
	Framework *framework.Framework

	VM     *v1alpha2.VirtualMachine
	VDRoot *v1alpha2.VirtualDisk

	UserDataSecret *corev1.Secret
	SysprepSecret  *corev1.Secret
}

func newProvisioningRemovalTest(f *framework.Framework) *provisioningRemovalTest {
	return &provisioningRemovalTest{Framework: f}
}

func (t *provisioningRemovalTest) GenerateResources() {
	t.VDRoot = object.NewVDFromCVI("vd-root", t.Framework.Namespace().Name, object.PrecreatedCVIAlpineBIOS)

	// NewMinimalVM sets cloud-init provisioning (UserData) by default. Manual
	// restart approval makes a wrongly-required restart observable as the
	// AwaitingRestartToApplyConfiguration condition instead of an auto-restart.
	t.VM = object.NewMinimalVM("vm", t.Framework.Namespace().Name,
		vmbuilder.WithDisks(t.VDRoot),
		vmbuilder.WithRestartApprovalMode(v1alpha2.Manual),
	)
}

func (t *provisioningRemovalTest) GenerateResourcesWithUserDataRef() {
	t.VDRoot = object.NewVDFromCVI("vd-root", t.Framework.Namespace().Name, object.PrecreatedCVIAlpineBIOS)

	t.UserDataSecret = newCloudInitSecret("cloud-init-secret", t.Framework.Namespace().Name, object.AlpineCloudInit)

	t.VM = vmbuilder.New(
		vmbuilder.WithGenerateName("vm"),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmbuilder.WithCPU(1, ptr.To("20%")),
		vmbuilder.WithMemory(resource.MustParse("512Mi")),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithDisks(t.VDRoot),
		vmbuilder.WithProvisioning(&v1alpha2.Provisioning{
			Type: v1alpha2.ProvisioningTypeUserDataRef,
			UserDataRef: &v1alpha2.UserDataRef{
				Kind: v1alpha2.UserDataRefKindSecret,
				Name: t.UserDataSecret.Name,
			},
		}),
		vmbuilder.WithRestartApprovalMode(v1alpha2.Manual),
	)
}

func (t *provisioningRemovalTest) GenerateResourcesWithSysprepRef() {
	t.VDRoot = object.NewVDFromCVI("vd-root", t.Framework.Namespace().Name, object.PrecreatedCVIAlpineBIOS)

	t.SysprepSecret = newSysprepSecret("sysprep-secret", t.Framework.Namespace().Name)

	// osType Windows selects the Windows device set (TPM, USB tablet). A real
	// Windows guest image is not available in the e2e environment, so the
	// guest stays Linux; this test validates the platform behaviour (condition
	// + no live detach) declared in the PR, not the guest-side automation.
	t.VM = vmbuilder.New(
		vmbuilder.WithGenerateName("vm"),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmbuilder.WithCPU(1, ptr.To("20%")),
		vmbuilder.WithMemory(resource.MustParse("512Mi")),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithDisks(t.VDRoot),
		vmbuilder.WithOsType(v1alpha2.Windows),
		vmbuilder.WithBootloader(v1alpha2.BIOS),
		vmbuilder.WithProvisioning(&v1alpha2.Provisioning{
			Type: v1alpha2.ProvisioningTypeSysprepRef,
			SysprepRef: &v1alpha2.SysprepRef{
				Kind: v1alpha2.SysprepRefKindSecret,
				Name: t.SysprepSecret.Name,
			},
		}),
		vmbuilder.WithRestartApprovalMode(v1alpha2.Manual),
	)
}

// newCloudInitSecret builds a Secret with a cloud-init userData script, typed
// so the DVP controller accepts it as a UserDataRef source.
func newCloudInitSecret(name, namespace, userData string) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: v1alpha2.SecretTypeCloudInit,
		Data: map[string][]byte{
			"userData": []byte(userData),
		},
	}
}

// newSysprepSecret builds a Secret with a minimal autounattend.xml, typed so
// the DVP controller accepts it as a SysprepRef source. The content is never
// consumed by a Windows guest in this test; it only needs to be a valid key.
func newSysprepSecret(name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: v1alpha2.SecretTypeSysprep,
		Data: map[string][]byte{
			"autounattend.xml": []byte(`<?xml version="1.0" encoding="utf-8"?>
<unattend xmlns="urn:schemas-microsoft-com:unattend">
</unattend>`),
		},
	}
}

func vmiHasVolume(vmi *virtv1.VirtualMachineInstance, name string) bool {
	for _, v := range vmi.Spec.Volumes {
		if v.Name == name {
			return true
		}
	}
	return false
}

func vmiHasDisk(vmi *virtv1.VirtualMachineInstance, name string) bool {
	for _, d := range vmi.Spec.Domain.Devices.Disks {
		if d.Name == name {
			return true
		}
	}
	return false
}
