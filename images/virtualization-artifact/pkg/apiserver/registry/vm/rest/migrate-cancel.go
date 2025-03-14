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

package rest

import (
	"context"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	genericreq "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/subresources"
)

func NewMigrateCancelREST(kubevirtClient client.Client) *MigrateCancelREST {
	return &MigrateCancelREST{
		kubevirtClient: kubevirtClient,
	}
}

var (
	_ rest.Storage      = &MigrateCancelREST{}
	_ rest.NamedCreater = &MigrateCancelREST{}
)

type MigrateCancelREST struct {
	kubevirtClient client.Client
}

func (r MigrateCancelREST) Create(ctx context.Context, name string, opts runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	_, ok := opts.(*subresources.VirtualMachineMigrateCancel)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}
	namespace := genericreq.NamespaceValue(ctx)

	// We are using kube-api-rewriter, so the final label for the search will look like this:
	// kubevirt.internal.virtualization.deckhouse.io/vmi-name: linux-vm-01
	migrations := &virtv1.VirtualMachineInstanceMigrationList{}
	if err := r.kubevirtClient.List(ctx, migrations,
		client.InNamespace(namespace),
		client.MatchingLabelsSelector{Selector: labels.SelectorFromSet(map[string]string{
			virtv1.MigrationSelectorLabel: name,
		})},
	); err != nil {
		return nil, k8serrors.NewInternalError(err)
	}

	done := false
	for _, mig := range migrations.Items {
		if !mig.IsFinal() {
			if err := r.kubevirtClient.Delete(ctx, &mig); err != nil {
				return nil, k8serrors.NewInternalError(err)
			}
			done = true
			break
		}
	}
	if !done {
		return nil, k8serrors.NewInternalError(fmt.Errorf("not found any migration to cancel for %s", name))
	}

	return &metav1.Status{Status: "Success"}, nil
}

func (r MigrateCancelREST) Destroy() {}

func (r MigrateCancelREST) New() runtime.Object {
	return &subresources.VirtualMachineMigrateCancel{}
}
