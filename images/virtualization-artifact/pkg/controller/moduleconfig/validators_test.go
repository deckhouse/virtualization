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

package moduleconfig

import (
	"context"
	"net/netip"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appconfig "github.com/deckhouse/virtualization-controller/pkg/config"
	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestCIDRsValidatorValidateUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}

	podSubnet := netip.MustParsePrefix("10.111.0.0/16")
	serviceSubnet := netip.MustParsePrefix("10.222.0.0/16")

	validator := newCIDRsValidator(fake.NewClientBuilder().WithScheme(scheme).Build(), &appconfig.ClusterSubnets{
		PodSubnet:     podSubnet,
		ServiceSubnet: serviceSubnet,
	})

	newMC := &mcapi.ModuleConfig{Spec: mcapi.ModuleConfigSpec{Settings: mcapi.SettingsValues{"dvcr": map[string]any{}}}}
	if _, err := validator.ValidateUpdate(context.Background(), nil, newMC); err != nil {
		t.Fatalf("expected no error without CIDRs, got: %v", err)
	}

	newMC.Spec.Settings["virtualMachineCIDRs"] = []any{"10.111.0.0/24", "10.111.0.0/25"}
	if _, err := validator.ValidateUpdate(context.Background(), nil, newMC); err == nil {
		t.Fatalf("expected overlap validation error")
	}
}

func TestRemoveCIDRsValidatorValidateUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}

	tests := []struct {
		name         string
		oldCIDRs     []any
		newCIDRs     []any
		ipLeaseNames []string
		wantErr      bool
	}{
		{
			"old none new none",
			nil,
			nil,
			nil,
			false,
		},
		{
			"old none new some",
			nil,
			[]any{"10.0.0.0/24"},
			nil,
			false,
		},
		{
			"old some new same",
			[]any{"10.0.0.0/24"},
			[]any{"10.0.0.0/24"},
			nil,
			false,
		},
		{
			"old some new grown",
			[]any{"10.0.0.0/24"},
			[]any{"10.0.0.0/24", "10.0.1.0/24"},
			nil,
			false,
		},
		{
			"old three new none (full clear is rejected)",
			[]any{"10.0.0.0/24", "10.0.1.0/24", "10.0.2.0/24"},
			nil,
			nil,
			true,
		},
		{
			"old some new replaced (without leases)",
			[]any{"10.0.0.0/24"},
			[]any{"10.0.1.0/24"},
			nil,
			false,
		},
		{
			"old two new one (without leases)",
			[]any{"10.0.0.0/24", "10.0.1.0/24"},
			[]any{"10.0.1.0/24"},
			nil,
			false,
		},
		{
			"old two new one removed with lease",
			[]any{"10.0.0.0/24", "10.0.1.0/24"},
			[]any{"10.0.1.0/24"},
			[]string{"ip-10-0-0-1"},
			true,
		},
	}

	createValidator := func(t *testing.T, ipLeaseNames []string) *removeCIDRsValidator {
		t.Helper()
		clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
		if len(ipLeaseNames) > 0 {
			leases := []client.Object{}
			for _, leaseName := range ipLeaseNames {
				leases = append(leases, &v1alpha2.VirtualMachineIPAddressLease{ObjectMeta: metav1.ObjectMeta{Name: leaseName}})
			}
			clientBuilder = clientBuilder.WithObjects(leases...)
		}
		return newRemoveCIDRsValidator(clientBuilder.Build())
	}

	createMC := func(t *testing.T, CIDRs []any) *mcapi.ModuleConfig {
		t.Helper()
		mc := &mcapi.ModuleConfig{
			Spec: mcapi.ModuleConfigSpec{
				Settings: mcapi.SettingsValues{
					"dvcr": map[string]any{},
				},
			},
		}
		if CIDRs != nil {
			mc.Spec.Settings["virtualMachineCIDRs"] = CIDRs
		}
		return mc
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := createValidator(t, tt.ipLeaseNames)

			oldMC := createMC(t, tt.oldCIDRs)
			newMC := createMC(t, tt.newCIDRs)

			_, err := validator.ValidateUpdate(context.Background(), oldMC, newMC)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got none")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
