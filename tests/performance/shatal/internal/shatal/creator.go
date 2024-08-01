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
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/shatal/internal/api"
)

// Creator generates the missing virtual machines until reaching the target quantity (where the target quantity equals the 'count' value).
type Creator struct {
	api            *api.Client
	namespace      string
	resourcePrefix string
	interval       time.Duration
	count          int
	logger         *slog.Logger
}

func NewCreator(api *api.Client, namespace, resourcePrefix string, interval time.Duration, count int, log *slog.Logger) *Creator {
	return &Creator{
		api:            api,
		namespace:      namespace,
		resourcePrefix: resourcePrefix,
		interval:       interval,
		count:          count,
		logger:         log.With("type", "creator"),
	}
}

func (s *Creator) Run(ctx context.Context) {
	for {
		s.createVMs(ctx)

		select {
		case <-time.After(s.interval):
		case <-ctx.Done():
			s.logger.Info("Creator stopped")
			return
		}
	}
}

func (s *Creator) createVMs(ctx context.Context) {
	vms, err := s.api.GetVMs(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}

		s.logger.Error(err.Error())
		return
	}

	names := make(map[string]struct{}, len(vms))
	for _, vm := range vms {
		names[vm.Name] = struct{}{}
	}

	for i := 0; i < s.count; i++ {
		vmName := fmt.Sprintf("%s-%d", s.resourcePrefix, i)
		_, ok := names[vmName]
		if ok {
			continue
		}

		restartApprovalMode := v1alpha2.Manual
		if i%2 == 0 {
			restartApprovalMode = v1alpha2.Automatic
		}

		s.logger.Info(fmt.Sprintf("Create: %s", vmName))

		vmdName := vmName

		vm := v1alpha2.VirtualMachine{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha2.VirtualMachineKind,
				APIVersion: v1alpha2.Version,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmName,
				Namespace: s.namespace,
				Labels: map[string]string{
					"vm": s.resourcePrefix,
				},
			},
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName:  "generic-v1",
				RunPolicy:                v1alpha2.AlwaysOnPolicy,
				EnableParavirtualization: true,
				OsType:                   v1alpha2.GenericOs,
				Bootloader:               "BIOS",
				CPU: v1alpha2.CPUSpec{
					Cores:        1,
					CoreFraction: "10%",
				},
				Memory: v1alpha2.MemorySpec{
					Size: resource.MustParse("512Mi"),
				},
				BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
					{
						Kind: v1alpha2.DiskDevice,
						Name: vmdName,
					},
				},
				Provisioning: &v1alpha2.Provisioning{
					Type: v1alpha2.ProvisioningTypeUserDataRef,
					UserDataRef: &v1alpha2.UserDataRef{
						Kind: v1alpha2.UserDataRefKindSecret,
						Name: s.resourcePrefix + "-cloud-init",
					},
				},
				Disruptions: &v1alpha2.Disruptions{
					RestartApprovalMode: restartApprovalMode,
				},
			},
		}

		err = s.api.CreateVM(ctx, vm)
		if err != nil {
			if strings.Contains(err.Error(), "VirtualMachineIPAddressClaim with the name of the virtual machine already exists") {
				continue
			}

			if errors.Is(err, context.Canceled) {
				return
			}

			s.logger.Error(err.Error())
			return
		}
	}
}
