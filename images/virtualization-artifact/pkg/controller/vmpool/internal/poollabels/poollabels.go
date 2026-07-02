//go:build EE
// +build EE

/*
Copyright 2026 Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license. See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/

// Package poollabels defines the controller-managed labels that mark a
// VirtualMachine as a member of a VirtualMachinePool and the selectors used to
// list members.
package poollabels

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	// PoolUID marks a replica with the metadata.uid of its pool. It is unique
	// per pool instance, so a manually created VirtualMachine can never match it
	// — membership cannot be hijacked. The controller lists members by this label
	// and publishes it in status.selector for the scale subresource. Analogous to
	// batch.kubernetes.io/controller-uid on Job pods.
	PoolUID = "vmpool.virtualization.deckhouse.io/pool-uid"

	// Pool is a human-readable label with the pool name, predictable from the
	// pool and handy for kubectl/observability. Analogous to job-name on Job pods.
	Pool = "vmpool.virtualization.deckhouse.io/pool"

	// TemplateHash marks the template revision a replica is effectively on (cf.
	// pod-template-hash / currentRevision). It is NOT part of the member selector,
	// so changing the template does not orphan existing replicas.
	TemplateHash = "vmpool.virtualization.deckhouse.io/template-hash"

	// PatchedTemplateHash (annotation) records the revision a replica's spec was
	// last patched to. It is distinct from TemplateHash so a re-patch is avoided
	// even while the disruptive part of the change waits for a restart, and it
	// does not depend on comparing specs (which the apiserver mutates by
	// defaulting/allocation).
	PatchedTemplateHash = "vmpool.virtualization.deckhouse.io/patched-template-hash"
)

// ListMembers returns the VirtualMachines controlled by the pool. The pool-uid
// label scopes the list; the controllerRef check is the authoritative guard.
func ListMembers(ctx context.Context, c client.Client, pool *v1alpha2.VirtualMachinePool) ([]v1alpha2.VirtualMachine, error) {
	var list v1alpha2.VirtualMachineList
	if err := c.List(ctx, &list, client.InNamespace(pool.GetNamespace()), MemberSelector(pool)); err != nil {
		return nil, err
	}
	members := make([]v1alpha2.VirtualMachine, 0, len(list.Items))
	for i := range list.Items {
		if ref := metav1.GetControllerOf(&list.Items[i]); ref != nil && ref.UID == pool.GetUID() {
			members = append(members, list.Items[i])
		}
	}
	return members, nil
}

// ComputeTemplateHash returns a stable short hash of the pool's
// virtualMachineTemplate — the desired revision replicas converge to.
func ComputeTemplateHash(pool *v1alpha2.VirtualMachinePool) string {
	// encoding/json sorts map keys, so the marshalling is deterministic.
	data, _ := json.Marshal(pool.Spec.VirtualMachineTemplate)
	h := fnv.New32a()
	_, _ = h.Write(data)
	return fmt.Sprintf("%x", h.Sum32())
}

// Member returns the managed labels stamped on every replica of the pool.
func Member(pool *v1alpha2.VirtualMachinePool) map[string]string {
	return map[string]string{
		PoolUID: string(pool.GetUID()),
		Pool:    pool.GetName(),
	}
}

// MemberSelector is the label selector the controller uses to list the members
// it owns. It contains only the hash-independent pool-uid, so it stays stable
// across template changes.
func MemberSelector(pool *v1alpha2.VirtualMachinePool) client.MatchingLabels {
	return client.MatchingLabels{PoolUID: string(pool.GetUID())}
}

// StatusSelector is the string form published in status.selector for the scale
// subresource (HPA/KEDA read it themselves).
func StatusSelector(pool *v1alpha2.VirtualMachinePool) string {
	return metav1.FormatLabelSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{PoolUID: string(pool.GetUID())},
	})
}
