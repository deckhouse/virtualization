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

package evacuation

import (
	"context"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
)

func newCanceler(virtClient kubeclient.Client) *canceler {
	return &canceler{
		virtClient: virtClient,
	}
}

type canceler struct {
	virtClient kubeclient.Client
}

func (c canceler) Cancel(ctx context.Context, name, namespace string) error {
	err := c.virtClient.VirtualMachines(namespace).CancelEvacuation(ctx, name, nil)
	if err != nil {
		if !strings.Contains(err.Error(), "not evacuated") && !k8serrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}
