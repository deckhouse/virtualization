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

package kubeclient

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/client-go/rest"

	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

type vmpool struct {
	virtualizationv1alpha2.VirtualMachinePoolInterface
	restClient *rest.RESTClient
	namespace  string
	resource   string
}

// ScaleDownWith removes the named pool members through the aggregated-apiserver
// scaleDownWith subresource, which deletes them and decrements the pool's
// replicas atomically, bypassing the anonymous /scale guard.
func (v vmpool) ScaleDownWith(ctx context.Context, name string, opts subv1alpha2.VirtualMachinePoolScaleDownWith) error {
	path := fmt.Sprintf(subresourceURLTpl, v.namespace, v.resource, name, "scaledownwith")
	body, err := json.Marshal(&opts)
	if err != nil {
		return err
	}
	return v.restClient.Post().AbsPath(path).Body(body).SetHeader("Content-Type", "application/json").Do(ctx).Error()
}
