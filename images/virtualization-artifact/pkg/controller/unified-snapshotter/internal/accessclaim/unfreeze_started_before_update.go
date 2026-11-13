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

package accessclaim

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/vmaccess"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

// Unfreezer sends an unfreeze to the guest of a virtual machine on behalf of the claim with claimUID.
type Unfreezer interface {
	Unfreeze(ctx context.Context, namespace, name string, claimUID types.UID) error
}

// UnfreezeStartedBeforeUpdate unfreezes the guest of the running instance kvvmi, which a snapshot started
// before the module update may have left frozen. A guest under a coordinator session is left alone: its
// freeze belongs to the session, which unfreezes it. A filesystem request that snapshot left on the
// instance is removed by the coordinator.
//
// The unfreeze is sent whatever fsFreezeStatus reads: it can read empty while the guest is frozen, and
// unfreezing a thawed guest is a no-op.
func UnfreezeStartedBeforeUpdate(ctx context.Context, reader client.Reader, unfreezer Unfreezer, kvvmi *virtv1.VirtualMachineInstance) error {
	if kvvmi == nil || kvvmi.Status.Phase != virtv1.Running {
		return nil
	}

	kvvm := &virtv1.VirtualMachine{}
	if err := reader.Get(ctx, types.NamespacedName{Namespace: kvvmi.Namespace, Name: kvvmi.Name}, kvvm); err != nil {
		return client.IgnoreNotFound(err)
	}

	lock, err := vmaccess.ReadLock(kvvm)
	if err != nil || lock != nil {
		return err
	}

	logger.FromContext(ctx).Info("unfreezing the guest a snapshot started before the module update may have left frozen")

	return unfreezer.Unfreeze(ctx, kvvmi.Namespace, kvvmi.Name, "")
}
