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

package handler

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	nodePlacementHandler = "NodePlacementHandler"
)

func NewNodePlacementHandler(client client.Client, migration OneShotMigration) *NodePlacementHandler {
	return &NodePlacementHandler{
		client:           client,
		oneShotMigration: migration,
	}
}

type NodePlacementHandler struct {
	client           client.Client
	oneShotMigration OneShotMigration
}

func (h *NodePlacementHandler) Handle(ctx context.Context, vm *v1alpha2.VirtualMachine) (reconcile.Result, error) {
	if vm == nil || !vm.GetDeletionTimestamp().IsZero() {
		return reconcile.Result{}, nil
	}

	kvvmi := &virtv1.VirtualMachineInstance{}
	if err := h.client.Get(ctx, object.NamespacedName(vm), kvvmi); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	cond, _ := conditions.GetKVVMICondition(conditions.VirtualMachineInstanceNodePlacementNotMatched, kvvmi.Status.Conditions)
	if cond.Status != corev1.ConditionTrue {
		return reconcile.Result{}, nil
	}

	sum, err := genNodePlacementSum(kvvmi)
	if err != nil {
		return reconcile.Result{}, err
	}

	log := logger.FromContext(ctx).With(logger.SlogHandler(nodePlacementHandler))
	ctx = logger.ToContext(ctx, log)

	migrate, err := h.oneShotMigration.OnceMigrate(ctx, vm, annotations.AnnVMOPWorkloadUpdateNodePlacementSum, sum)
	if migrate {
		log.Info("The virtual machine was triggered to migrate by the nodeplacement handler.")
	}

	return reconcile.Result{}, err
}

func (h *NodePlacementHandler) Name() string {
	return nodePlacementHandler
}

func genNodePlacementSum(kvvmi *virtv1.VirtualMachineInstance) (string, error) {
	if kvvmi == nil {
		return "", fmt.Errorf("kvvmi is nil")
	}
	np := &nodePlacement{
		NodeSelector: kvvmi.Spec.NodeSelector,
		Affinity:     kvvmi.Spec.Affinity,
	}
	b, err := json.Marshal(np)
	if err != nil {
		return "", err
	}
	hasher := md5.New()
	hasher.Write(b)
	hashInBytes := hasher.Sum(nil)
	return hex.EncodeToString(hashInBytes), nil
}

type nodePlacement struct {
	NodeSelector map[string]string `json:"nodeSelector"`
	Affinity     *corev1.Affinity  `json:"affinity"`
}
