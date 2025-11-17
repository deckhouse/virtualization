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

package internal

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dvcrcondition "github.com/deckhouse/virtualization-controller/pkg/controller/dvcr-maintenance/condition"
	dvcrtypes "github.com/deckhouse/virtualization-controller/pkg/controller/dvcr-maintenance/types"
	dvcr_deployment_condition "github.com/deckhouse/virtualization/api/core/v1alpha2/dvcr-deployment-condition"
)

type LifeCycleHandler struct {
	client             client.Client
	dvcrService        dvcrtypes.DVCRService
	provisioningLister dvcrtypes.ProvisioningLister
}

func NewLifeCycleHandler(client client.Client, dvcrService dvcrtypes.DVCRService, provisioningLister dvcrtypes.ProvisioningLister) *LifeCycleHandler {
	return &LifeCycleHandler{
		client:             client,
		dvcrService:        dvcrService,
		provisioningLister: provisioningLister,
	}
}

func (h LifeCycleHandler) Handle(ctx context.Context, req reconcile.Request, deploy *appsv1.Deployment) (reconcile.Result, error) {
	if deploy == nil || deploy.GetDeletionTimestamp() != nil {
		return reconcile.Result{}, nil
	}

	if req.Namespace == dvcrtypes.CronSourceNamespace && req.Name == dvcrtypes.CronSourceRunGC {
		dvcrcondition.UpdateMaintenanceCondition(deploy,
			dvcr_deployment_condition.InProgress,
			"Garbage collection initiated.",
		)
		return reconcile.Result{}, h.dvcrService.InitiateMaintenanceMode(ctx)
	}

	if req.Name == dvcrtypes.DVCRMaintenanceSecretName {
		// Secret has 3 states:
		// - created, without annotations (InitiatedNotStarted)
		//   - wait for all provisioners to finish, add switch annotation.
		//   - add condition to deployment
		// - switch annotation is present. (Started)
		//   - wait for cleanup to finish, just return.
		//   - add condition to deployment
		// - done annotation is present. (Done)
		//   - cleanup is done, copy result to deployment condition.
		//   - delete secret.
		secret, err := h.dvcrService.GetMaintenanceSecret(ctx)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("fetch maintenance secret: %w", err)
		}
		if secret == nil || secret.GetDeletionTimestamp() != nil {
			// Secret is gone, no action required.
			return reconcile.Result{}, nil
		}

		if h.dvcrService.IsMaintenanceDone(secret) {
			dvcrcondition.UpdateMaintenanceCondition(deploy,
				dvcr_deployment_condition.Done,
				"%s", string(secret.Data["result"]),
			)
			return reconcile.Result{}, h.dvcrService.DeleteMaintenanceSecret(ctx)
		}

		if h.dvcrService.IsMaintenanceStarted(secret) {
			dvcrcondition.UpdateMaintenanceCondition(deploy,
				dvcr_deployment_condition.InProgress,
				"Wait for garbage collection to finish.",
			)
			// Wait for done annotation.
			return reconcile.Result{}, nil
		}

		// No special annotation, check for provisioners to finish.
		resourcesInProvisioning, err := h.provisioningLister.ListAllInProvisioning(ctx)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("list resources in provisioning: %w", err)
		}
		remainInProvisioning := len(resourcesInProvisioning)
		if remainInProvisioning > 0 {
			dvcrcondition.UpdateMaintenanceCondition(deploy,
				dvcr_deployment_condition.InProgress,
				"Wait for cvi/vi/vd finish provisioning: %d resources remain.", remainInProvisioning,
			)
			return reconcile.Result{RequeueAfter: time.Second * 20}, nil
		}
		// All provisioners are finished, switch to garbage collection.
		err = h.dvcrService.SwitchToMaintenanceMode(ctx)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("switch to maintenance mode: %w", err)
		}
		dvcrcondition.UpdateMaintenanceCondition(deploy,
			dvcr_deployment_condition.InProgress,
			"Wait for garbage collection to finish.",
		)
		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, nil
}
