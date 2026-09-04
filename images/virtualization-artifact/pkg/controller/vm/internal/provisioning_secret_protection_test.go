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

package internal

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("Provisioning secret protection", func() {
	const ns = "default"

	newScheme := func() *apiruntime.Scheme {
		scheme := apiruntime.NewScheme()
		for _, f := range []func(*apiruntime.Scheme) error{
			v1alpha2.AddToScheme,
			corev1.AddToScheme,
		} {
			Expect(f(scheme)).To(Succeed())
		}
		return scheme
	}

	userDataRef := func(secretName string) *v1alpha2.Provisioning {
		return &v1alpha2.Provisioning{
			Type: v1alpha2.ProvisioningTypeUserDataRef,
			UserDataRef: &v1alpha2.UserDataRef{
				Kind: v1alpha2.UserDataRefKindSecret,
				Name: secretName,
			},
		}
	}

	sysprepRef := func(secretName string) *v1alpha2.Provisioning {
		return &v1alpha2.Provisioning{
			Type: v1alpha2.ProvisioningTypeSysprepRef,
			SysprepRef: &v1alpha2.SysprepRef{
				Kind: v1alpha2.SysprepRefKindSecret,
				Name: secretName,
			},
		}
	}

	newVM := func(name string, phase v1alpha2.MachinePhase, provisioning *v1alpha2.Provisioning) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       v1alpha2.VirtualMachineSpec{Provisioning: provisioning},
			Status:     v1alpha2.VirtualMachineStatus{Phase: phase},
		}
	}

	newDeletingVM := func(name string, phase v1alpha2.MachinePhase, provisioning *v1alpha2.Provisioning) *v1alpha2.VirtualMachine {
		vm := newVM(name, phase, provisioning)
		vm.DeletionTimestamp = &metav1.Time{Time: metav1.Now().Time}
		vm.Finalizers = []string{v1alpha2.FinalizerVMCleanup}
		return vm
	}

	newSecret := func(name string, finalizers ...string) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Finalizers: finalizers},
		}
	}

	getSecret := func(cl client.Client, name string) *corev1.Secret {
		secret := &corev1.Secret{}
		Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, secret)).To(Succeed())
		return secret
	}

	reconcile := func(objs ...client.Object) client.Client {
		fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(objs...).Build()
		protection := service.NewProtectionService(fakeClient, v1alpha2.FinalizerProvisioningSecretProtection)
		Expect(reconcileProvisioningSecretProtection(context.Background(), fakeClient, protection, ns)).To(Succeed())
		return fakeClient
	}

	hasProtection := func(cl client.Client, secretName string) bool {
		return controllerutil.ContainsFinalizer(getSecret(cl, secretName), v1alpha2.FinalizerProvisioningSecretProtection)
	}

	inUseBy := func(cl client.Client, secretName string) (string, bool) {
		value, found := getSecret(cl, secretName).GetAnnotations()[annotations.AnnInUseByVirtualMachines]
		return value, found
	}

	DescribeTable("vmRequiresProvisioningSecretProtection",
		func(vm *v1alpha2.VirtualMachine, expected bool) {
			Expect(vmRequiresProvisioningSecretProtection(vm)).To(Equal(expected))
		},
		Entry("nil VM", nil, false),
		Entry("stopped VM", newVM("vm", v1alpha2.MachineStopped, nil), false),
		// A pending machine is already on its way up: its pod is being created, and losing the
		// secret now stalls that pod exactly as it stalls a migration target.
		Entry("pending VM", newVM("vm", v1alpha2.MachinePending, nil), true),
		Entry("starting VM", newVM("vm", v1alpha2.MachineStarting, nil), true),
		Entry("running VM", newVM("vm", v1alpha2.MachineRunning, nil), true),
		Entry("stopping VM", newVM("vm", v1alpha2.MachineStopping, nil), true),
		Entry("migrating VM", newVM("vm", v1alpha2.MachineMigrating, nil), true),
		Entry("paused VM", newVM("vm", v1alpha2.MachinePause, nil), true),
		Entry("degraded VM", newVM("vm", v1alpha2.MachineDegraded, nil), true),
		// A machine being deleted starts no new pods, so it must let the secret go: otherwise
		// nothing would ever release the finalizer once the machine itself is gone.
		Entry("deleting running VM", newDeletingVM("vm", v1alpha2.MachineRunning, nil), false),
		Entry("terminating VM being deleted", newDeletingVM("vm", v1alpha2.MachineTerminating, nil), false),
	)

	It("collects the secrets of both provisioning kinds and sorts the machines", func() {
		fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(
			newVM("vm-b", v1alpha2.MachineRunning, userDataRef("cloud-init")),
			newVM("vm-a", v1alpha2.MachineRunning, userDataRef("cloud-init")),
			newVM("vm-windows", v1alpha2.MachineRunning, sysprepRef("sysprep")),
			newVM("vm-stopped", v1alpha2.MachineStopped, userDataRef("stopped-only")),
			newVM("vm-no-provisioning", v1alpha2.MachineRunning, nil),
		).Build()

		users, err := provisioningSecretUsers(context.Background(), fakeClient, ns)
		Expect(err).ToNot(HaveOccurred())
		Expect(users).To(HaveLen(2))
		Expect(users["cloud-init"]).To(Equal([]string{"vm-a", "vm-b"}))
		Expect(users["sysprep"]).To(Equal([]string{"vm-windows"}))
		Expect(users).ToNot(HaveKey("stopped-only"))
	})

	It("protects the secret of a running VM and annotates it, leaving unrelated secrets alone", func() {
		cl := reconcile(
			newVM("vm-running", v1alpha2.MachineRunning, userDataRef("cloud-init")),
			newSecret("cloud-init"),
			newSecret("registry-credentials"),
		)

		Expect(hasProtection(cl, "cloud-init")).To(BeTrue())
		value, found := inUseBy(cl, "cloud-init")
		Expect(found).To(BeTrue())
		Expect(value).To(Equal("vm-running"))

		Expect(hasProtection(cl, "registry-credentials")).To(BeFalse())
		_, found = inUseBy(cl, "registry-credentials")
		Expect(found).To(BeFalse())
	})

	It("lists every machine holding a shared secret in a stable order", func() {
		cl := reconcile(
			newVM("vm-b", v1alpha2.MachineRunning, userDataRef("shared")),
			newVM("vm-a", v1alpha2.MachineMigrating, userDataRef("shared")),
			newSecret("shared"),
		)

		value, found := inUseBy(cl, "shared")
		Expect(found).To(BeTrue())
		Expect(value).To(Equal("vm-a,vm-b"))
	})

	It("keeps a shared secret while at least one machine still needs it", func() {
		cl := reconcile(
			newVM("vm-running", v1alpha2.MachineRunning, userDataRef("shared")),
			newVM("vm-stopped", v1alpha2.MachineStopped, userDataRef("shared")),
			newSecret("shared"),
		)

		Expect(hasProtection(cl, "shared")).To(BeTrue())
		value, _ := inUseBy(cl, "shared")
		Expect(value).To(Equal("vm-running"))
	})

	It("releases the secret of a stopped VM", func() {
		cl := reconcile(
			newVM("vm-stopped", v1alpha2.MachineStopped, userDataRef("cloud-init")),
			newSecret("cloud-init", v1alpha2.FinalizerProvisioningSecretProtection),
		)

		Expect(hasProtection(cl, "cloud-init")).To(BeFalse())
		_, found := inUseBy(cl, "cloud-init")
		Expect(found).To(BeFalse())
	})

	It("releases the secret of a VM being deleted, so namespace deletion cannot stall", func() {
		cl := reconcile(
			newDeletingVM("vm-deleting", v1alpha2.MachineRunning, userDataRef("cloud-init")),
			newSecret("cloud-init", v1alpha2.FinalizerProvisioningSecretProtection),
		)

		Expect(hasProtection(cl, "cloud-init")).To(BeFalse())
		_, found := inUseBy(cl, "cloud-init")
		Expect(found).To(BeFalse())
	})

	It("releases the secret a VM no longer refers to", func() {
		cl := reconcile(
			newVM("vm-running", v1alpha2.MachineRunning, nil),
			newSecret("former-cloud-init", v1alpha2.FinalizerProvisioningSecretProtection),
		)

		Expect(hasProtection(cl, "former-cloud-init")).To(BeFalse())
	})

	It("does not patch a secret whose protection is already in place", func() {
		secret := newSecret("cloud-init")
		cl := reconcile(newVM("vm-running", v1alpha2.MachineRunning, userDataRef("cloud-init")), secret)

		settled := getSecret(cl, "cloud-init").GetResourceVersion()

		protection := service.NewProtectionService(cl, v1alpha2.FinalizerProvisioningSecretProtection)
		Expect(reconcileProvisioningSecretProtection(context.Background(), cl, protection, ns)).To(Succeed())

		Expect(getSecret(cl, "cloud-init").GetResourceVersion()).To(Equal(settled))
	})
})
