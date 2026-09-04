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

package handler

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonvm "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const nameRestartApprovalHandler = "RestartApprovalHandler"

func NewRestartApprovalHandler(client client.Client) *RestartApprovalHandler {
	return &RestartApprovalHandler{client: client}
}

// RestartApprovalHandler keeps the dialogue in step with the node: it asks while the node is held
// and clears the answer once the node is free.
type RestartApprovalHandler struct {
	client client.Client
}

func (h *RestartApprovalHandler) Handle(ctx context.Context, node *corev1.Node) (reconcile.Result, error) {
	if node == nil {
		return reconcile.Result{}, nil
	}

	held, err := h.heldByMachines(ctx, node.GetName())
	if err != nil {
		return reconcile.Result{}, err
	}

	_, requested := node.GetAnnotations()[annotations.AnnNodeVMRestartRequired]
	_, approved := node.GetAnnotations()[annotations.AnnNodeVMRestartApproved]

	switch {
	case held && !requested:
		logger.FromContext(ctx).Info("Ask an administrator to allow restarting the machines that hold the node")
		return reconcile.Result{}, h.patchAnnotations(ctx, node, map[string]interface{}{
			annotations.AnnNodeVMRestartRequired: "",
		})
	case !held && requested:
		// The node is free, so the request is answered and the permission it spent goes with it: an
		// approval left behind would silently cover every maintenance that follows.
		toRemove := map[string]interface{}{annotations.AnnNodeVMRestartRequired: nil}
		if approved {
			toRemove[annotations.AnnNodeVMRestartApproved] = nil
			logger.FromContext(ctx).Info("The node is released, the restart approval is spent")
		}
		return reconcile.Result{}, h.patchAnnotations(ctx, node, toRemove)
	}

	return reconcile.Result{}, nil
}

func (h *RestartApprovalHandler) Name() string {
	return nameRestartApprovalHandler
}

// heldByMachines reports whether the node is held by a virtual machine that has been asked to leave
// it and cannot: such a machine either waits for a person or is about to be restarted, and either
// way the node is not released until it is dealt with.
func (h *RestartApprovalHandler) heldByMachines(ctx context.Context, nodeName string) (bool, error) {
	var vms v1alpha2.VirtualMachineList
	if err := h.client.List(ctx, &vms, client.MatchingFields{indexer.IndexFieldVMByNode: nodeName}); err != nil {
		return false, fmt.Errorf("failed to list virtual machines on node %s: %w", nodeName, err)
	}

	for _, vm := range vms.Items {
		if commonvm.HoldsNodeUnderMaintenance(&vm) {
			return true, nil
		}
	}

	return false, nil
}

// patchAnnotations sets the given annotations on the node; a nil value removes one. A merge patch
// is used rather than a JSON one because it creates the annotations map on a node that has none.
func (h *RestartApprovalHandler) patchAnnotations(ctx context.Context, node *corev1.Node, values map[string]interface{}) error {
	body, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{"annotations": values},
	})
	if err != nil {
		return fmt.Errorf("failed to build patch for node %s: %w", node.GetName(), err)
	}

	err = h.client.Patch(ctx, node, client.RawPatch(types.MergePatchType, body))
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to patch node %s: %w", node.GetName(), err)
	}

	return nil
}
