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
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonvm "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// vmRequiresProvisioningSecretProtection reports whether the provisioning secret of the virtual
// machine may still be needed. The secret is not consumed once at the first boot and forgotten:
// every virt-launcher pod mounts it and renders the cloud-init or sysprep image from it anew, so
// it is read by every start and by the target pod of every migration.
//
// A stopped machine releases it: deleting the secret then only prevents the next start, which the
// ProvisioningReady condition reports on its own. A machine being deleted releases it as well —
// it starts no new pods, and the pods it still runs have already mounted the secret. Without that
// exemption nobody would ever release the finalizer of the last machine, and the deletion of the
// whole namespace would stall on it forever.
func vmRequiresProvisioningSecretProtection(vm *v1alpha2.VirtualMachine) bool {
	if vm == nil {
		return false
	}

	if !vm.GetDeletionTimestamp().IsZero() {
		return false
	}

	return vm.Status.Phase != v1alpha2.MachineStopped
}

// provisioningSecretUsers maps the name of a provisioning secret to the sorted names of the
// virtual machines in the namespace that keep it.
func provisioningSecretUsers(ctx context.Context, cl client.Client, namespace string) (map[string][]string, error) {
	var vms v1alpha2.VirtualMachineList
	if err := cl.List(ctx, &vms, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list VirtualMachines: %w", err)
	}

	users := make(map[string][]string)

	for i := range vms.Items {
		vm := &vms.Items[i]
		if !vmRequiresProvisioningSecretProtection(vm) {
			continue
		}

		for _, secretName := range commonvm.ProvisioningSecretNames(vm) {
			users[secretName] = append(users[secretName], vm.GetName())
		}
	}

	for secretName := range users {
		slices.Sort(users[secretName])
	}

	return users, nil
}

// reconcileProvisioningSecretProtection ensures the provisioning-secret-protection finalizer and
// the in-use annotation are set on the secrets that virtual machines of the namespace still need,
// and are released from every other secret. Working namespace-wide keeps a secret shared by
// several machines protected while at least one of them needs it, and releases a secret whose
// machine has stopped, dropped its spec.provisioning or gone away.
func reconcileProvisioningSecretProtection(ctx context.Context, cl client.Client, protection *service.ProtectionService, namespace string) error {
	users, err := provisioningSecretUsers(ctx, cl, namespace)
	if err != nil {
		return err
	}

	var secrets corev1.SecretList
	if err := cl.List(ctx, &secrets, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("failed to list Secrets: %w", err)
	}

	for i := range secrets.Items {
		secret := &secrets.Items[i]

		vmNames, inUse := users[secret.GetName()]
		if inUse {
			// The annotation is written first: it is the only place on a Secret that can explain
			// why its deletion is held, and it must not be missing while the finalizer is set.
			if err := setInUseByVirtualMachines(ctx, cl, secret, vmNames); err != nil {
				return err
			}

			if err := protection.AddProtection(ctx, secret); err != nil {
				return err
			}

			continue
		}

		// The finalizer goes first here for the same reason: a secret that is no longer held must
		// not keep an annotation claiming otherwise.
		if controllerutil.ContainsFinalizer(secret, v1alpha2.FinalizerProvisioningSecretProtection) {
			if err := protection.RemoveProtection(ctx, secret); err != nil {
				return err
			}
		}

		if err := setInUseByVirtualMachines(ctx, cl, secret, nil); err != nil {
			return err
		}
	}

	return nil
}

// setInUseByVirtualMachines keeps the in-use annotation of the secret in sync with vmNames and
// removes it when no machine holds the secret. The secret is patched only when the value actually
// changes: every namespace-wide pass walks all of its secrets, and a virtual machine is
// reconciled often.
func setInUseByVirtualMachines(ctx context.Context, cl client.Client, secret *corev1.Secret, vmNames []string) error {
	value := strings.Join(vmNames, ",")
	current, found := secret.GetAnnotations()[annotations.AnnInUseByVirtualMachines]

	if value == current && (found || value == "") {
		return nil
	}

	patch := client.MergeFrom(secret.DeepCopy())

	anno := secret.GetAnnotations()
	if anno == nil {
		anno = make(map[string]string)
	}

	if value == "" {
		delete(anno, annotations.AnnInUseByVirtualMachines)
	} else {
		anno[annotations.AnnInUseByVirtualMachines] = value
	}
	secret.SetAnnotations(anno)

	if err := cl.Patch(ctx, secret, patch); err != nil {
		return fmt.Errorf("failed to annotate Secret %q: %w", secret.GetName(), err)
	}

	return nil
}
