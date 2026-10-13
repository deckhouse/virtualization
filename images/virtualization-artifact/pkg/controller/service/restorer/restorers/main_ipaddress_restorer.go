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

package restorer

import (
	"context"
	"fmt"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer/common"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// MainIPAddressHandler restores the sdn IPAddress of the Main network with the
// KeepIPAddress semantics: the resource is created, or taken over, as Static with
// the snapshot-time address, so the restored VM keeps its address.
type MainIPAddressHandler struct {
	name       string
	namespace  string
	address    string
	vmUIDs     []string
	client     client.Client
	restoreUID string
}

// NewMainIPAddressHandler takes the UIDs the restored VM had: an IPAddress labeled
// with any other UID belongs to another VM.
func NewMainIPAddressHandler(client client.Client, name, namespace, address string, vmUIDs []string, vmRestoreUID string) *MainIPAddressHandler {
	return &MainIPAddressHandler{
		name:       name,
		namespace:  namespace,
		address:    address,
		vmUIDs:     vmUIDs,
		client:     client,
		restoreUID: vmRestoreUID,
	}
}

// Name returns the current (possibly overridden) IPAddress name to be linked
// into the restored VM spec.
func (v *MainIPAddressHandler) Name() string {
	return v.name
}

func (v *MainIPAddressHandler) Override(rules []v1alpha2.NameReplacement) {
	v.name = common.OverrideName(sdnapi.IPAddressKind, v.name, rules)
}

func (v *MainIPAddressHandler) Customize(prefix, suffix string) {
	v.name = common.ApplyNameCustomization(v.name, prefix, suffix)
}

// The IPAddress is labeled with one of the UIDs the VM had, or this restore has taken
// it over.
func IsIPAddressOfRestoredVM(ipa *sdnapi.IPAddress, vmUIDs []string, restoreUID string) bool {
	return slices.Contains(vmUIDs, ipa.GetLabels()[annotations.LabelVirtualMachineUID]) ||
		ipa.GetAnnotations()[annotations.AnnVMOPRestore] == restoreUID
}

// A Static IPAddress the sdn module refused names the address but does not take it.
func GetMainIPAddressTakingAddress(ctx context.Context, c client.Client, namespace, address string) (*sdnapi.IPAddress, error) {
	ipas := &sdnapi.IPAddressList{}
	if err := c.List(ctx, ipas, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("list the IPAddresses of the namespace %s: %w", namespace, err)
	}
	for i := range ipas.Items {
		ipa := &ipas.Items[i]
		if ipa.GetDeletionTimestamp() == nil && network.IsMainNetworkIPAddress(ipa) &&
			network.AddressOfIPAddress(ipa) == address && !network.IPAddressTakenByAnother(ipa) {
			return ipa, nil
		}
	}
	return nil, nil
}

func (v *MainIPAddressHandler) ValidateRestore(ctx context.Context) error {
	existing, err := object.FetchObject(ctx, types.NamespacedName{Name: v.name, Namespace: v.namespace}, v.client, &sdnapi.IPAddress{})
	if err != nil {
		return err
	}
	return v.validate(ctx, existing)
}

// validate checks that the restore may take the IPAddress named v.name (nil: it does
// not exist) for the address.
func (v *MainIPAddressHandler) validate(ctx context.Context, existing *sdnapi.IPAddress) error {
	if existing == nil {
		ipaTakingAddress, err := GetMainIPAddressTakingAddress(ctx, v.client, v.namespace, v.address)
		if err != nil {
			return err
		}
		if ipaTakingAddress != nil {
			return fmt.Errorf("the address %s is %w by the IPAddress %q: delete that IPAddress first", v.address, common.ErrAlreadyInUse, ipaTakingAddress.GetName())
		}
		return nil
	}

	if existing.GetAnnotations()[annotations.AnnVMOPRestore] == v.restoreUID {
		return nil
	}
	if address := network.AddressOfIPAddress(existing); address != v.address {
		return fmt.Errorf("the IPAddress %q is %w: it has the address %s, and the snapshot has %s; delete it, or restore it under another name with nameReplacements", v.name, common.ErrAlreadyInUse, address, v.address)
	}
	// ProcessRestore takes the IPAddress away from its VM. The restored VM is stopped
	// by then, but the IPAddress may belong to another VM.
	if vmUID := existing.GetLabels()[annotations.LabelVirtualMachineUID]; vmUID != "" && !slices.Contains(v.vmUIDs, vmUID) {
		return fmt.Errorf("the IPAddress %q is %w by another virtual machine: free the address of that virtual machine first", v.name, common.ErrAlreadyInUse)
	}
	return nil
}

func (v *MainIPAddressHandler) ProcessRestore(ctx context.Context) error {
	existing, err := object.FetchObject(ctx, types.NamespacedName{Name: v.name, Namespace: v.namespace}, v.client, &sdnapi.IPAddress{})
	if err != nil {
		return err
	}
	if err := v.validate(ctx, existing); err != nil {
		return err
	}

	if existing == nil {
		ipa := &sdnapi.IPAddress{
			ObjectMeta: metav1.ObjectMeta{
				Name:        v.name,
				Namespace:   v.namespace,
				Annotations: map[string]string{annotations.AnnVMOPRestore: v.restoreUID},
			},
			Spec: network.MainStaticIPAddressSpec(v.address),
		}
		if err := v.client.Create(ctx, ipa); err != nil {
			return fmt.Errorf("failed to create the IPAddress: %w", err)
		}
		return nil
	}

	// A retry of this restore after the patch below has nothing to do.
	if existing.GetAnnotations()[annotations.AnnVMOPRestore] == v.restoreUID && len(existing.GetOwnerReferences()) == 0 {
		return nil
	}
	patch := client.MergeFrom(existing.DeepCopy())
	network.DetachIPAddressFromVM(existing)
	existing.Spec = network.MainStaticIPAddressSpec(v.address)
	anns := existing.GetAnnotations()
	if anns == nil {
		anns = map[string]string{}
	}
	anns[annotations.AnnVMOPRestore] = v.restoreUID
	existing.SetAnnotations(anns)
	return v.client.Patch(ctx, existing, patch)
}

func (v *MainIPAddressHandler) ValidateClone(_ context.Context) error {
	return nil
}

func (v *MainIPAddressHandler) ProcessClone(_ context.Context) error {
	return nil
}

func (v *MainIPAddressHandler) Object() client.Object {
	return &sdnapi.IPAddress{
		ObjectMeta: metav1.ObjectMeta{Name: v.name, Namespace: v.namespace},
	}
}
