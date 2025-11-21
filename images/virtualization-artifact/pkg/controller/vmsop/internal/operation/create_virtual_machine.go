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

package operation

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewCreateVirtualMachineOperation(client client.Client) *CreateVirtualMachineOperation {
	return &CreateVirtualMachineOperation{
		client: client,
	}
}

type CreateVirtualMachineOperation struct {
	client client.Client
}

func (o CreateVirtualMachineOperation) Execute(ctx context.Context, vmsop *v1alpha2.VirtualMachineSnapshotOperation, vms *v1alpha2.VirtualMachineSnapshot, secret *corev1.Secret) error {
	snapshotResources := restorer.NewSnapshotResources(o.client, v1alpha2.VMOPTypeClone, vmsop.Spec.CreateVirtualMachine.Mode, secret, vms, string(vmsop.UID))

	err := snapshotResources.Prepare(ctx)
	if err != nil {
		return err
	}

	snapshotResources.Override(vmsop.Spec.CreateVirtualMachine.NameReplacement)

	if vmsop.Spec.CreateVirtualMachine.Customization != nil {
		snapshotResources.Customize(
			vmsop.Spec.CreateVirtualMachine.Customization.NamePrefix,
			vmsop.Spec.CreateVirtualMachine.Customization.NameSuffix,
		)
	}

	statuses, err := snapshotResources.Validate(ctx)
	vmsop.Status.Resources = statuses
	if err != nil {
		return err
	}

	if vmsop.Spec.CreateVirtualMachine.Mode == v1alpha2.SnapshotOperationModeDryRun {
		return nil
	}

	statuses, err = snapshotResources.Process(ctx)
	vmsop.Status.Resources = statuses
	if err != nil {
		return err
	}

	return nil
}
