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

package supplements

import (
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualDiskGenerator struct {
	*supplements.Generator
	claimName string
}

func NewGenerator(vd *virtv2.VirtualDisk) *VirtualDiskGenerator {
	return &VirtualDiskGenerator{
		Generator: supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID),
		claimName: vd.Status.Target.PersistentVolumeClaim,
	}
}

func (g *VirtualDiskGenerator) SetClaimName(name string) {
	g.claimName = name
}

func (g *VirtualDiskGenerator) DataVolume() types.NamespacedName {
	return g.PersistentVolumeClaim()
}

func (g *VirtualDiskGenerator) PersistentVolumeClaim() types.NamespacedName {
	return types.NamespacedName{
		Namespace: g.Namespace,
		Name:      g.claimName,
	}
}
