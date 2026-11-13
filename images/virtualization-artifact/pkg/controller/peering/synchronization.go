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

package peering

import (
	"context"
	"fmt"
	"maps"

	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	intvirt "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

const (
	migrationPeerSyncPinAnnotation     = "kubevirt.io/migration-peer-sync-pin"
	migrationPeerSyncAddressAnnotation = "kubevirt.io/migration-peer-sync-address"
)

func MigrationSynchronizationAnnotations(endpoint intvirt.MigrationSynchronizationEndpoint) map[string]string {
	return map[string]string{
		migrationPeerSyncPinAnnotation:     endpoint.SyncPin,
		migrationPeerSyncAddressAnnotation: endpoint.SyncAddress,
	}
}

func SetMigrationSynchronization(ctx context.Context, cl client.Client, key types.NamespacedName, endpoint intvirt.MigrationSynchronizationEndpoint) error {
	if endpoint.SyncPin == "" {
		return nil
	}

	migration := &virtv1.VirtualMachineInstanceMigration{}
	if err := cl.Get(ctx, key, migration); err != nil {
		return client.IgnoreNotFound(err)
	}

	if migration.Spec.SendTo == nil && migration.Spec.Receive == nil {
		return fmt.Errorf("migration %s has no peer", key)
	}

	if migration.Annotations[migrationPeerSyncPinAnnotation] == endpoint.SyncPin &&
		migration.Annotations[migrationPeerSyncAddressAnnotation] == endpoint.SyncAddress {
		return nil
	}

	before := migration.DeepCopy()
	if migration.Annotations == nil {
		migration.Annotations = make(map[string]string)
	}

	maps.Copy(migration.Annotations, MigrationSynchronizationAnnotations(endpoint))

	if err := cl.Patch(ctx, migration, client.MergeFrom(before)); err != nil {
		return fmt.Errorf("update the migration peer synchronization endpoint: %w", err)
	}

	return nil
}
