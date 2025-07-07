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

package moduleconfig

import (
	"context"
	"fmt"
	"net/netip"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
)

type cidrsValidator struct {
	client client.Client
}

func newCIDRsValidator(client client.Client) *cidrsValidator {
	return &cidrsValidator{
		client: client,
	}
}

func (v cidrsValidator) ValidateUpdate(ctx context.Context, _, newMC *mcapi.ModuleConfig) (admission.Warnings, error) {
	CIDRs, err := parseCIDRs(newMC.Spec.Settings)
	if err != nil {
		return admission.Warnings{}, err
	}

	err = checkOverlapsCIDRs(CIDRs)
	if err != nil {
		return admission.Warnings{}, err
	}

	err = v.checkNodeSubnets(ctx, CIDRs)
	if err != nil {
		return admission.Warnings{}, err
	}

	return admission.Warnings{}, nil
}

func (v cidrsValidator) checkNodeSubnets(ctx context.Context, excludedPrefixes []netip.Prefix) error {
	nodes := &corev1.NodeList{}
	err := v.client.List(ctx, nodes)
	if err != nil {
		return fmt.Errorf("internal error: unable to retrieve nodes at the moment, please try again later. Details: %w", err)
	}
	return checkNodeAddressesOverlap(nodes.Items, excludedPrefixes)
}
