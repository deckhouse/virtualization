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
	if !isPeerUser(req.UserInfo.Username) {
		return nil, fmt.Errorf("InternalVirtualizationVirtualMachineReceiver can only be created by a peer cluster")
	}
	return nil, validateReceiver(r)
}

func (v *MigrationReceiverValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldR, ok := oldObj.(*intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver)
	if !ok {
		return nil, fmt.Errorf("expected an old InternalVirtualizationVirtualMachineReceiver but got a %T", oldObj)
	}
	newR, ok := newObj.(*intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver)
	if !ok {
		return nil, fmt.Errorf("expected a new InternalVirtualizationVirtualMachineReceiver but got a %T", newObj)
	}
	if !reflect.DeepEqual(oldR.Spec, newR.Spec) {
		return nil, fmt.Errorf("the spec of InternalVirtualizationVirtualMachineReceiver cannot be changed")
	}
	return nil, nil
}

func (v *MigrationReceiverValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// isPeerUser tells the identity of a peer cluster acting for the virtualization capability, as the
// peering module presents it: peering:peer:<peer>:cap:virtualization.
func isPeerUser(name string) bool {
	if name == ownControllerUser {
		return true
	}
	peer, ok := strings.CutPrefix(name, "peering:peer:")
	return ok && strings.HasSuffix(peer, ":cap:virtualization") && !strings.HasPrefix(peer, ":")
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
