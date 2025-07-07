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

package restorer

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualDiskOverrideValidator struct {
	vd     *virtv2.VirtualDisk
	client client.Client
}

func NewVirtualDiskOverrideValidator(vdTmpl *virtv2.VirtualDisk, client client.Client) *VirtualDiskOverrideValidator {
	return &VirtualDiskOverrideValidator{
		vd: &virtv2.VirtualDisk{
			TypeMeta: metav1.TypeMeta{
				Kind:       vdTmpl.Kind,
				APIVersion: vdTmpl.APIVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        vdTmpl.Name,
				Namespace:   vdTmpl.Namespace,
				Annotations: vdTmpl.Annotations,
				Labels:      vdTmpl.Labels,
			},
			Spec: vdTmpl.Spec,
		},
		client: client,
	}
}

func (v VirtualDiskOverrideValidator) Override(rules []virtv2.NameReplacement) {
	v.vd.Name = overrideName(v.vd.Kind, v.vd.Name, rules)
}

func (v VirtualDiskOverrideValidator) Validate(ctx context.Context) error {
	vdKey := types.NamespacedName{Namespace: v.vd.Namespace, Name: v.vd.Name}
	existed, err := object.FetchObject(ctx, vdKey, v.client, &virtv2.VirtualDisk{})
	if err != nil {
		return err
	}

	if existed != nil {
		return fmt.Errorf("the virtual disk %q %w", vdKey.Name, ErrAlreadyExists)
	}

	return nil
}

func (v VirtualDiskOverrideValidator) Object() client.Object {
	return v.vd
}
