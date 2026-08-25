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

package failfast

import (
	"context"
	"regexp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// internalVMIGVR is the GroupVersionResource of the internal (KubeVirt) VMI.
var internalVMIGVR = schema.GroupVersionResource{
	Group:    "internal.virtualization.deckhouse.io",
	Version:  "v1",
	Resource: "internalvirtualizationvirtualmachineinstances",
}

// KVVMIMigrationFailed fails the spec when an internal VMI of the namespace
// records a terminally failed live migration: the failure often never
// propagates to the object a spec is waiting on (the VM or a VMOP wedged
// InProgress), so without this rule the wait burns its whole timeout.
//
// The grace period doubles as an escape hatch for two legitimate flows: a
// promptly retried migration replaces status.migrationState before the
// re-check, and a wait on the failing VMOP classifies known KubeVirt flakes
// into a Skip (ending the spec and the rule with it) before the rule fires.
func KVVMIMigrationFailed(dyn dynamic.Interface, namespace string) FailFast {
	kvvmis := dynamicClient{dyn.Resource(internalVMIGVR).Namespace(namespace)}
	return New("InternalVirtualMachineInstance "+namespace+"/", kvvmis, func(kvvmi *unstructured.Unstructured) *Finding {
		failed, _, _ := unstructured.NestedBool(kvvmi.Object, "status", "migrationState", "failed")
		if !failed {
			return nil
		}
		// A cancelled migration also settles with failed=true, but the abort
		// marks record that the termination was requested, not suffered: a
		// spec cancelling its migration is making progress, not wedging.
		abortRequested, _, _ := unstructured.NestedBool(kvvmi.Object, "status", "migrationState", "abortRequested")
		abortStatus, _, _ := unstructured.NestedString(kvvmi.Object, "status", "migrationState", "abortStatus")
		if abortRequested || abortStatus != "" {
			return nil
		}
		reason, _, _ := unstructured.NestedString(kvvmi.Object, "status", "migrationState", "failureReason")
		if IsKnownDRBDDualPrimaryDeniedFailureReason(reason) {
			return &Finding{
				Message: "hit the known linstor-csi allow-two-primaries race (migration target got EROFS on a hotplugged DRBD disk): " + reason,
				Grace:   defaultGrace,
				Skip:    true,
			}
		}
		return &Finding{
			Message: "reports a terminally failed live migration: " + reason,
			Grace:   defaultGrace,
		}
	})
}

// knownDRBDDualPrimaryDeniedRe matches the known linstor-csi race: a stale
// attachment's ControllerUnpublish drops the DRBD allow-two-primaries property
// mid-migration, so the target qemu cannot auto-promote the volume and dies
// with EROFS on the hotplug disk path.
// TODO: remove after the migration to the new sds-replicated.
var knownDRBDDualPrimaryDeniedRe = regexp.MustCompile(`(?i)/hotplug-disks/[^']*':\s*read-only file system`)

func IsKnownDRBDDualPrimaryDeniedFailureReason(reason string) bool {
	return knownDRBDDualPrimaryDeniedRe.MatchString(reason)
}

// VirtualDiskMigrationReverted fails the spec when a volume migration of a
// namespace VirtualDisk terminally fails (e.g. "Migration reverted"): the
// disk-side failure does not always surface on the VM or VMOP the spec waits
// on.
func VirtualDiskMigrationReverted(vds Client[*v1alpha2.VirtualDisk], namespace string) FailFast {
	return New("VirtualDisk "+namespace+"/", vds, func(vd *v1alpha2.VirtualDisk) *Finding {
		if vd.Status.MigrationState.Result != v1alpha2.VirtualDiskMigrationResultFailed {
			return nil
		}
		return &Finding{
			Message: "volume migration failed: " + vd.Status.MigrationState.Message,
			Grace:   defaultGrace,
		}
	})
}

// dynamicClient adapts a dynamic ResourceInterface to [Client]: the dynamic
// Get takes variadic subresources, which keeps it from satisfying the
// interface directly.
type dynamicClient struct {
	dynamic.ResourceInterface
}

func (c dynamicClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*unstructured.Unstructured, error) {
	return c.ResourceInterface.Get(ctx, name, opts)
}
