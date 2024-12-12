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

package shatal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/shatal/internal/api"
)

// Modifier updates virtual machines (core fraction from 10% to 25% and vice versa).
type Modifier struct {
	api       *api.Client
	namespace string
	logger    *slog.Logger
}

func NewModifier(api *api.Client, namespace string, log *slog.Logger) *Modifier {
	return &Modifier{
		api:       api,
		namespace: namespace,
		logger:    log.With("type", "modifier"),
	}
}

const (
	bigCoreFraction   = "25%"
	smallCoreFraction = "10%"
)

func (s *Modifier) Do(ctx context.Context, vm v1alpha2.VirtualMachine) {
	if vm.Spec.CPU.CoreFraction == smallCoreFraction {
		vm.Spec.CPU.CoreFraction = bigCoreFraction
	} else {
		vm.Spec.CPU.CoreFraction = smallCoreFraction
	}

	s.logger.With("node", vm.Status.Node).
		With("core-fraction", vm.Spec.CPU.CoreFraction).
		Info(fmt.Sprintf("Modify: %s", vm.Name))

	err := s.api.PatchCoreFraction(ctx, vm)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}

		s.logger.Error(err.Error())
		return
	}

	if vm.Spec.Disruptions.RestartApprovalMode == v1alpha2.Automatic {
		return
	}

	vmop := v1alpha2.VirtualMachineOperation{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha2.VirtualMachineOperationKind,
			APIVersion: v1alpha2.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.New().String(),
			Namespace: s.namespace,
		},
		Spec: v1alpha2.VirtualMachineOperationSpec{
			Type:           v1alpha2.VMOPTypeRestart,
			VirtualMachine: vm.Name,
		},
	}

	err = s.api.ApplyVMOP(ctx, vmop)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}

		s.logger.Error(err.Error())
	}
}
