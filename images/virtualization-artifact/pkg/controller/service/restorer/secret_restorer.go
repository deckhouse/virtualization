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

package restorer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/snapshot"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type SecretRestorer struct {
	client client.Client
}

func NewSecretRestorer(client client.Client) *SecretRestorer {
	return &SecretRestorer{
		client: client,
	}
}

func (r SecretRestorer) Get(ctx context.Context, vmSnapshot *v1alpha2.VirtualMachineSnapshot) (*corev1.Secret, error) {
	newSecretName := snapshot.GetVMSnapshotSecretName(vmSnapshot.Name, vmSnapshot.UID)
	secret := &corev1.Secret{}
	err := r.client.Get(ctx, types.NamespacedName{
		Name:      newSecretName,
		Namespace: vmSnapshot.Namespace,
	}, secret)
	if err == nil {
		return secret, nil
	}

	if !k8serrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get secret with new name %s: %w", newSecretName, err)
	}

	legacySecretName := snapshot.GetLegacyVMSnapshotSecretName(vmSnapshot.Name)
	err = r.client.Get(ctx, types.NamespacedName{
		Name:      legacySecretName,
		Namespace: vmSnapshot.Namespace,
	}, secret)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, fmt.Errorf("snapshot secret not found (tried %s and %s)", newSecretName, legacySecretName)
		}
		return nil, fmt.Errorf("failed to get secret with legacy name %s: %w", legacySecretName, err)
	}

	return secret, nil
}

func (r SecretRestorer) Store(ctx context.Context, vm *v1alpha2.VirtualMachine, vmSnapshot *v1alpha2.VirtualMachineSnapshot) (*corev1.Secret, error) {
	secret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      snapshot.GetVMSnapshotSecretName(vmSnapshot.Name, vmSnapshot.UID),
			Namespace: vmSnapshot.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				service.MakeOwnerReference(vmSnapshot),
			},
		},
		Data: make(map[string][]byte),
		Type: "virtualmachine.virtualization.deckhouse.io/snapshot",
	}

	err := r.setVirtualMachine(&secret, vm)
	if err != nil {
		return nil, err
	}

	err = r.setVirtualMachineIPAddress(ctx, &secret, vm, vmSnapshot.Spec.KeepIPAddress)
	if err != nil {
		return nil, err
	}

	err = r.setVirtualMachineMACAddresses(ctx, &secret, vm)
	if err != nil {
		return nil, err
	}

	err = r.setProvisioning(ctx, &secret, vm)
	if err != nil {
		return nil, err
	}

	err = r.setVirtualMachineBlockDeviceAttachments(ctx, &secret, vm)
	if err != nil {
		return nil, err
	}

	err = r.client.Create(ctx, &secret)
	if err != nil {
		return nil, fmt.Errorf("failed to create restorer secret: %w", err)
	}

	return &secret, nil
}

func (r SecretRestorer) RestoreVirtualMachine(_ context.Context, secret *corev1.Secret) (*v1alpha2.VirtualMachine, error) {
	return get[*v1alpha2.VirtualMachine](secret, virtualMachineKey)
}

func (r SecretRestorer) RestoreProvisioner(_ context.Context, secret *corev1.Secret) (*corev1.Secret, error) {
	return get[*corev1.Secret](secret, provisionerKey)
}

func (r SecretRestorer) RestoreVirtualMachineIPAddress(_ context.Context, secret *corev1.Secret) (*v1alpha2.VirtualMachineIPAddress, error) {
	return get[*v1alpha2.VirtualMachineIPAddress](secret, virtualMachineIPAddressKey)
}

func (r SecretRestorer) RestoreVirtualMachineMACAddresses(_ context.Context, secret *corev1.Secret) ([]*v1alpha2.VirtualMachineMACAddress, error) {
	return get[[]*v1alpha2.VirtualMachineMACAddress](secret, virtualMachineMACAddressesKey)
}

func (r SecretRestorer) RestoreMACAddressOrder(_ context.Context, secret *corev1.Secret) ([]string, error) {
	vm, err := get[*v1alpha2.VirtualMachine](secret, virtualMachineKey)
	if err != nil {
		return nil, err
	}

	var macAddressOrder []string
	for _, ns := range vm.Status.Networks {
		if ns.Type == v1alpha2.NetworksTypeMain {
			continue
		}
		macAddressOrder = append(macAddressOrder, ns.MAC)
	}
	return macAddressOrder, nil
}

func (r SecretRestorer) RestoreVirtualMachineBlockDeviceAttachments(_ context.Context, secret *corev1.Secret) ([]*v1alpha2.VirtualMachineBlockDeviceAttachment, error) {
	return get[[]*v1alpha2.VirtualMachineBlockDeviceAttachment](secret, virtualMachineBlockDeviceAttachmentKey)
}

func (r SecretRestorer) setVirtualMachine(secret *corev1.Secret, vm *v1alpha2.VirtualMachine) error {
	JSON, err := json.Marshal(vm)
	if err != nil {
		return err
	}

	secret.Data[virtualMachineKey] = JSON
	return nil
}

func (r SecretRestorer) setVirtualMachineBlockDeviceAttachments(ctx context.Context, secret *corev1.Secret, vm *v1alpha2.VirtualMachine) error {
	var vmbdas []*v1alpha2.VirtualMachineBlockDeviceAttachment

	for _, bdr := range vm.Status.BlockDeviceRefs {
		if !bdr.Hotplugged {
			continue
		}

		vmbda, err := object.FetchObject(ctx, types.NamespacedName{
			Name:      bdr.VirtualMachineBlockDeviceAttachmentName,
			Namespace: vm.Namespace,
		}, r.client, &v1alpha2.VirtualMachineBlockDeviceAttachment{})
		if err != nil {
			return err
		}

		if vmbda == nil {
			return fmt.Errorf("the virtual machine block device attachment %q not found", bdr.VirtualMachineBlockDeviceAttachmentName)
		}

		vmbdas = append(vmbdas, vmbda)
	}

	if len(vmbdas) == 0 {
		return nil
	}

	JSON, err := json.Marshal(vmbdas)
	if err != nil {
		return err
	}

	secret.Data[virtualMachineBlockDeviceAttachmentKey] = JSON
	return nil
}

func (r SecretRestorer) setVirtualMachineIPAddress(ctx context.Context, secret *corev1.Secret, vm *v1alpha2.VirtualMachine, keepIPAddress v1alpha2.KeepIPAddress) error {
	vmip, err := object.FetchObject(ctx, types.NamespacedName{
		Namespace: vm.Namespace,
		Name:      vm.Status.VirtualMachineIPAddress,
	}, r.client, &v1alpha2.VirtualMachineIPAddress{})
	if err != nil {
		return err
	}

	if vmip == nil {
		return fmt.Errorf("the virtual machine ip address %q not found", vm.Status.VirtualMachineIPAddress)
	}

	/*
		1. Never/Always (Keep/Convert)
		2. Static/Auto
		3. Empty/Set

		Always == convert Auto to Static
		Static == keep old IP address
		Set == with old name
		-----------------------------------------------------------------------------------------
		| KEEP   | IP-TYPE | VM-IP | BEHAVIOUR                                      | IN-SECRET |
		| Never  | Static  | Empty | not possible                                   |     -     |
		| Never  | Static  | Set   | keep old IP address with old name              |     Y     |
		| Never  | Auto    | Empty | allocate new random IP address with any name   |     N     |
		| Never  | Auto    | Set   | allocate new random IP address with old name   |     Y     |
		| Always | Static  | Empty | not possible                                   |     -     |
		| Always | Static  | Set   | keep old IP address with old name              |     Y     |
		| Always | Auto    | Empty | convert and keep old IP address with any name  |     Y     |
		| Always | Auto    | Set   | convert and keep old IP address with old name  |     Y     |
		-----------------------------------------------------------------------------------------
	*/

	switch keepIPAddress {
	case v1alpha2.KeepIPAddressNever:
		switch vmip.Spec.Type {
		case v1alpha2.VirtualMachineIPAddressTypeStatic:
			if vm.Spec.VirtualMachineIPAddress == "" {
				return errors.New("not possible to use static ip address with omitted .spec.VirtualMachineIPAddress, please report a bug")
			}
		case v1alpha2.VirtualMachineIPAddressTypeAuto:
			if vm.Spec.VirtualMachineIPAddress == "" {
				return nil
			}
		}

		// Put to secret.
	case v1alpha2.KeepIPAddressAlways:
		switch vmip.Spec.Type {
		case v1alpha2.VirtualMachineIPAddressTypeStatic:
			if vm.Spec.VirtualMachineIPAddress == "" {
				return errors.New("not possible to use static ip address with omitted .spec.VirtualMachineIPAddress, please report a bug")
			}

			// Put to secret.
		case v1alpha2.VirtualMachineIPAddressTypeAuto:
			vmip.Spec.Type = v1alpha2.VirtualMachineIPAddressTypeStatic
			vmip.Spec.StaticIP = vmip.Status.Address
			// Put to secret.
		}
	}

	JSON, err := json.Marshal(vmip)
	if err != nil {
		return err
	}

	secret.Data[virtualMachineIPAddressKey] = JSON
	return nil
}

func (r SecretRestorer) setVirtualMachineMACAddresses(ctx context.Context, secret *corev1.Secret, vm *v1alpha2.VirtualMachine) error {
	var vmmacs []v1alpha2.VirtualMachineMACAddress
	for _, ns := range vm.Status.Networks {
		if ns.Type == v1alpha2.NetworksTypeMain {
			continue
		}

		vmmac, err := object.FetchObject(ctx, types.NamespacedName{
			Namespace: vm.Namespace,
			Name:      ns.VirtualMachineMACAddressName,
		}, r.client, &v1alpha2.VirtualMachineMACAddress{})
		if err != nil {
			return err
		}

		if vmmac == nil {
			return fmt.Errorf("the virtual machine mac address %q not found", ns.VirtualMachineMACAddressName)
		}

		vmmac.Spec.Address = vmmac.Status.Address
		vmmacs = append(vmmacs, *vmmac)
	}

	if len(vmmacs) == 0 {
		return nil
	}

	JSON, err := json.Marshal(vmmacs)
	if err != nil {
		return err
	}

	secret.Data[virtualMachineMACAddressesKey] = JSON
	return nil
}

func (r SecretRestorer) setProvisioning(ctx context.Context, secret *corev1.Secret, vm *v1alpha2.VirtualMachine) error {
	var secretName string

	if vm.Spec.Provisioning == nil {
		return nil
	}

	switch vm.Spec.Provisioning.Type {
	case v1alpha2.ProvisioningTypeSysprepRef:
		if vm.Spec.Provisioning.SysprepRef == nil {
			return errors.New("the virtual machine sysprep ref provisioning is nil")
		}

		switch vm.Spec.Provisioning.SysprepRef.Kind {
		case v1alpha2.SysprepRefKindSecret:
			secretName = vm.Spec.Provisioning.SysprepRef.Name
		default:
			return fmt.Errorf("unknown sysprep ref kind %s", vm.Spec.Provisioning.SysprepRef.Kind)
		}
	case v1alpha2.ProvisioningTypeUserDataRef:
		if vm.Spec.Provisioning.UserDataRef == nil {
			return errors.New("the virtual machine user data ref provisioning is nil")
		}

		switch vm.Spec.Provisioning.UserDataRef.Kind {
		case v1alpha2.UserDataRefKindSecret:
			secretName = vm.Spec.Provisioning.UserDataRef.Name
		default:
			return fmt.Errorf("unknown user data ref kind %s", vm.Spec.Provisioning.UserDataRef.Kind)
		}
	default:
		return nil
	}

	secretKey := types.NamespacedName{Name: secretName, Namespace: vm.Namespace}
	provisioner, err := object.FetchObject(ctx, secretKey, r.client, &corev1.Secret{})
	if err != nil {
		return err
	}

	if provisioner == nil {
		return fmt.Errorf("the virtual machine provisioning secret %q not found", secretName)
	}

	JSON, err := json.Marshal(provisioner)
	if err != nil {
		return err
	}

	secret.Data[provisionerKey] = JSON
	return nil
}

func extractJSON(data []byte) []byte {
	jsonData, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return data
	}

	return jsonData
}

func get[T any](secret *corev1.Secret, key string) (T, error) {
	var t T

	data, ok := secret.Data[key]
	if !ok {
		return t, nil
	}

	jsonData := extractJSON(data)
	err := json.Unmarshal(jsonData, &t)
	if err != nil {
		return t, err
	}

	return t, nil
}
