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

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	dvcrcondition "github.com/deckhouse/virtualization-controller/pkg/controller/dvcr-garbage-collection/condition"
	dvcrtypes "github.com/deckhouse/virtualization-controller/pkg/controller/dvcr-garbage-collection/types"
	dvcrdeploymentcondition "github.com/deckhouse/virtualization/api/core/v1alpha2/dvcr-deployment-condition"
)

type LifeCycleHandler struct {
	client             client.Client
	dvcrService        dvcrtypes.DVCRService
	provisioningLister dvcrtypes.ProvisioningLister
}

var (
	gcStatusPollingInterval    = time.Second * 20
	resultPersistRetryInterval = time.Second
)

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
		dvcrcondition.UpdateGarbageCollectionCondition(deploy,
			dvcrdeploymentcondition.InProgress,
			"Garbage collection initiated.",
		)
		return reconcile.Result{}, h.dvcrService.InitiateGarbageCollectionMode(ctx)
	}

	if req.Name == dvcrtypes.DVCRGarbageCollectionSecretName {
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
		secret, err := h.dvcrService.GetGarbageCollectionSecret(ctx)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("fetch garbage collection secret: %w", err)
		}
		if secret == nil || secret.GetDeletionTimestamp() != nil {
			// Secret is gone, nothing to clean up:
			// - Keep deployment conditions for informational purposes.
			// - Postponed CVI/VI/VD will start importers themselves.
			return reconcile.Result{}, nil
		}

		if h.dvcrService.IsGarbageCollectionDone(secret) {
			if h.dvcrService.IsGarbageCollectionResultPersisted(secret, deploy) {
				return reconcile.Result{}, h.dvcrService.DeleteGarbageCollectionSecret(ctx)
			}

			// Extract error or success message from the result.
			reason, msg, err := h.dvcrService.ParseGarbageCollectionResult(secret)
			if err != nil {
				return reconcile.Result{}, err
			}
			dvcrcondition.UpdateGarbageCollectionCondition(deploy, reason, "%s", msg)
			// Put full result JSON into annotation on deployment.
			annotations.AddAnnotation(deploy, annotations.AnnDVCRGarbageCollectionResult, h.dvcrService.GetGarbageCollectionResult(secret))
			// Requeue to delete secret only after deployment update succeeds.
			return reconcile.Result{RequeueAfter: resultPersistRetryInterval}, nil
		}

		if h.dvcrService.IsGarbageCollectionStarted(secret) {
			hasCreationTimestamp := !secret.GetCreationTimestamp().Time.IsZero()
			waitDuration := time.Since(secret.GetCreationTimestamp().Time)
			if hasCreationTimestamp && waitDuration > dvcrtypes.GarbageCollectionTimeout {
				if h.dvcrService.IsGarbageCollectionResultPersisted(secret, deploy) {
					return reconcile.Result{}, h.dvcrService.DeleteGarbageCollectionSecret(ctx)
				}

				dvcrcondition.UpdateGarbageCollectionCondition(deploy,
					dvcrdeploymentcondition.Error,
					"Wait for garbage collection more than %s timeout: %s elapsed, garbage collection canceled",
					dvcrtypes.GarbageCollectionTimeout.String(),
					waitDuration.Truncate(time.Second).String(),
				)
				annotations.AddAnnotation(deploy, annotations.AnnDVCRGarbageCollectionResult, "")
				return reconcile.Result{RequeueAfter: resultPersistRetryInterval}, nil
			}

			dvcrcondition.UpdateGarbageCollectionCondition(deploy,
				dvcrdeploymentcondition.InProgress,
				"Wait for garbage collection to finish.",
			)
			// Wait for done annotation appears on secret.
			return reconcile.Result{RequeueAfter: gcStatusPollingInterval}, nil
		}

		// No special annotation, check for provisioners to finish.
		resourcesInProvisioning, err := h.provisioningLister.ListAllInProvisioning(ctx)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("list resources in provisioning: %w", err)
		}
		remainInProvisioning := len(resourcesInProvisioning)
		if remainInProvisioning == 0 {
			// All provisioners are finished, switch DVCR to garbage collection.
			err = h.dvcrService.SwitchToGarbageCollectionMode(ctx)
			if err != nil {
				return reconcile.Result{}, fmt.Errorf("switch to garbage collection mode: %w", err)
			}
			dvcrcondition.UpdateGarbageCollectionCondition(deploy,
				dvcrdeploymentcondition.InProgress,
				"Wait for garbage collection to finish.",
			)
			return reconcile.Result{RequeueAfter: gcStatusPollingInterval}, nil
		}

		// Cancel garbage collection if wait for provisioners for too long.
		hasCreationTimestamp := !secret.GetCreationTimestamp().Time.IsZero()
		waitDuration := time.Since(secret.GetCreationTimestamp().Time)
		if hasCreationTimestamp && waitDuration > dvcrtypes.WaitProvisionersTimeout {
			if h.dvcrService.IsGarbageCollectionResultPersisted(secret, deploy) {
				return reconcile.Result{}, h.dvcrService.DeleteGarbageCollectionSecret(ctx)
			}

			// Wait for provisioners timed out: report error and stop garbage collection.
			dvcrcondition.UpdateGarbageCollectionCondition(deploy,
				dvcrdeploymentcondition.Error,
				"Wait for %d resources provisioners to finish more than %s timeout: %s elapsed, garbage collection canceled",
				remainInProvisioning,
				dvcrtypes.WaitProvisionersTimeout.String(),
				waitDuration.Truncate(time.Second).String(),
			)
			annotations.AddAnnotation(deploy, annotations.AnnDVCRGarbageCollectionResult, "")
			return reconcile.Result{RequeueAfter: resultPersistRetryInterval}, nil
		}

		// Use requeue to wait for provisioners to finish.
		dvcrcondition.UpdateGarbageCollectionCondition(deploy,
			dvcrdeploymentcondition.InProgress,
			"Wait for cvi/vi/vd finish provisioning: %d resources remain.", remainInProvisioning,
		)
		return reconcile.Result{RequeueAfter: gcStatusPollingInterval}, nil
	}

	return reconcile.Result{}, nil
}
