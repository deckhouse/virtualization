/*
Copyright 2024 Flant JSC

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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/powerstate"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const namePodHandler = "PodHandler "

func NewPodHandler(client client.Client) *PodHandler {
	return &PodHandler{
		client:     client,
		protection: service.NewProtectionService(client, virtv2.FinalizerPodProtection),
	}
}

type PodHandler struct {
	client     client.Client
	protection *service.ProtectionService
}

func (h *PodHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}
	current := s.VirtualMachine().Current()
	pods, err := s.Pods(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if isDeletion(current) {
		objs := make([]client.Object, len(pods.Items))
		for i, p := range pods.Items {
			objs[i] = p.DeepCopy()
		}
		return reconcile.Result{}, h.protection.RemoveProtection(ctx, objs...)
	}
	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	info := powerstate.ShutdownReason(kvvmi, pods)
	if info.PodCompeted {
		s.Shared(func(s *state.Shared) {
			s.ShutdownInfo = info
		})
		return reconcile.Result{}, h.protection.RemoveProtection(ctx, &info.Pod)
	}

	for _, p := range pods.Items {
		if podFinal(p) {
			continue
		}
		if err := h.protection.AddProtection(ctx, &p); err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func (h *PodHandler) Name() string {
	return namePodHandler
}
