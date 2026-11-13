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
	"fmt"
	"reflect"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/peering"
	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

// ownControllerUser is the controller of this module: it may create requests too, for stands without the
// peering module where the source reaches this cluster with its token.
const ownControllerUser = "system:serviceaccount:d8-virtualization:virtualization-controller"

// MigrationReceiverValidator lets only a peer cluster ask this cluster to receive a migration: the VM runs
// here with the domain of the request.
type MigrationReceiverValidator struct{}

func NewMigrationReceiverValidator() *MigrationReceiverValidator {
	return &MigrationReceiverValidator{}
}

func (v *MigrationReceiverValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	r, ok := obj.(*intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver)
	if !ok {
		return nil, fmt.Errorf("expected a InternalVirtualizationVirtualMachineReceiver but got a %T", obj)
	}

	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, err
	}

	if !mayCreateReceiver(req.UserInfo.Username, req.UserInfo.Groups) {
		return nil, fmt.Errorf("InternalVirtualizationVirtualMachineReceiver can only be created by a peer cluster")
	}

	return nil, validateReceiver(r)
}

func (v *MigrationReceiverValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldR, ok := oldObj.(*intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver)
	if !ok {
		return nil, fmt.Errorf("expected an old InternalVirtualizationVirtualMachineReceiver but got a %T", oldObj)
	}

	newR, ok := newObj.(*intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver)
	if !ok {
		return nil, fmt.Errorf("expected a new InternalVirtualizationVirtualMachineReceiver but got a %T", newObj)
	}

	oldSpec, newSpec := oldR.Spec, newR.Spec
	oldSpec.SourceSynchronization = intvirtv1alpha1.MigrationSynchronizationEndpoint{}

	newSpec.SourceSynchronization = intvirtv1alpha1.MigrationSynchronizationEndpoint{}
	if !reflect.DeepEqual(oldSpec, newSpec) {
		return nil, fmt.Errorf("the spec of InternalVirtualizationVirtualMachineReceiver cannot be changed")
	}
	// The finalizer keeps the request until the objects created for it no longer refer to it: a peer that
	// removed it could have the garbage collector delete the VM received here.
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, err
	}

	if isPeerGroup(req.UserInfo.Groups) &&
		(!slices.Equal(oldR.Finalizers, newR.Finalizers) || !reflect.DeepEqual(oldR.OwnerReferences, newR.OwnerReferences)) {
		return nil, fmt.Errorf("the finalizers and the owner references of InternalVirtualizationVirtualMachineReceiver cannot be changed by a peer cluster")
	}

	return nil, nil
}

func (v *MigrationReceiverValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// MigrationReceiverDefaulter names in the request the peer that asks for it, as this cluster knows the peer:
// the VM received here shows the cluster it comes from. The name comes from the identity of the peer on
// every write of the peer, so a peer cannot give another one.
type MigrationReceiverDefaulter struct{}

func NewMigrationReceiverDefaulter() *MigrationReceiverDefaulter {
	return &MigrationReceiverDefaulter{}
}

func (d *MigrationReceiverDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	r, ok := obj.(*intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver)
	if !ok {
		return fmt.Errorf("expected a InternalVirtualizationVirtualMachineReceiver but got a %T", obj)
	}

	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return err
	}

	peer := peerName(req.UserInfo.Groups)
	if peer == "" {
		return nil
	}

	r.Spec.SourcePeer = peer

	return nil
}

// peerName returns the name this cluster gives the peer of the virtualization capability in its group,
// peering:peer:<peer>:cap:virtualization, or nothing for another identity.
func peerName(groups []string) string {
	for _, g := range groups {
		peer, ok := strings.CutPrefix(g, "peering:peer:")
		if !ok {
			continue
		}

		if name, ok := strings.CutSuffix(peer, ":cap:virtualization"); ok && name != "" {
			return name
		}
	}

	return ""
}

// mayCreateReceiver allows this module's controller and authenticated peers with the virtualization capability.
func mayCreateReceiver(name string, groups []string) bool {
	return name == ownControllerUser || isPeerGroup(groups)
}

// isPeerGroup tells a peer cluster by the group of its capability, unlike the controller of this module.
func isPeerGroup(groups []string) bool {
	return peerName(groups) != ""
}

// validateReceiver checks that the request is readable and that the domain of the source asks for no
// devices of the nodes here: the VirtualMachine, the disks and the volumes are this cluster's own and pass
// its own webhooks, but the domain runs as is.
func validateReceiver(r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) error {
	rcv, err := peering.ParseReceiver(r)
	if err != nil {
		return err
	}

	if rcv.VirtualMachine.Name != "" && rcv.VirtualMachine.Name != r.Name {
		return fmt.Errorf("spec.virtualMachineTemplate is named %q, not %q", rcv.VirtualMachine.Name, r.Name)
	}

	devices := rcv.Instance.Spec.Domain.Devices
	if len(devices.HostDevices) > 0 || len(devices.GPUs) > 0 {
		return fmt.Errorf("spec.internalVirtualizationVirtualMachineInstance: devices of the nodes cannot be received")
	}

	return nil
}
