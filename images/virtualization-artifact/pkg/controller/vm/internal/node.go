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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// isNodeUnresponsive reports whether the given node stopped reporting readiness. Everything the
// platform knows about a running virtual machine comes from virt-handler on its node, so once the
// node is gone the statuses freeze at their last known values: without this check conditions keep
// claiming the virtual machine is fine.
//
// The node name must come from the running instance, not from the virtual machine status: the
// status keeps the previous node until the next successful sync, and a stale name would keep the
// conditions unknown after the instance has already moved elsewhere.
//
// A missing node counts as unresponsive too: it was removed while the instance was still on it.
func isNodeUnresponsive(ctx context.Context, c client.Client, nodeName string) (bool, string, error) {
	if nodeName == "" {
		return false, "", nil
	}

	node := &corev1.Node{}
	err := c.Get(ctx, types.NamespacedName{Name: nodeName}, node)
	switch {
	case err == nil:
	case apierrors.IsNotFound(err):
		return true, fmt.Sprintf("Node %q is gone, the actual state of the virtual machine is unknown.", nodeName), nil
	default:
		return false, "", err
	}

	for _, c := range node.Status.Conditions {
		if c.Type != corev1.NodeReady {
			continue
		}
		if c.Status == corev1.ConditionTrue {
			return false, "", nil
		}
		return true, fmt.Sprintf("Node %q stopped reporting readiness at %s, the actual state of the virtual machine is unknown.",
			nodeName, c.LastTransitionTime.UTC().Format("2006-01-02 15:04:05 UTC")), nil
	}

	// A node without the Ready condition has never reported its state.
	return true, fmt.Sprintf("Node %q does not report readiness, the actual state of the virtual machine is unknown.", nodeName), nil
}
