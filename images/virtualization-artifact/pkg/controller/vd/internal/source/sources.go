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

package source

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type Handler interface {
	Sync(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error)
	CleanUp(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error)
	Validate(ctx context.Context, vd *virtv2.VirtualDisk) error
}

type Sources struct {
	sources map[virtv2.DataSourceType]Handler
}

func NewSources() *Sources {
	return &Sources{
		sources: make(map[virtv2.DataSourceType]Handler),
	}
}

func (s Sources) Set(dsType virtv2.DataSourceType, h Handler) {
	s.sources[dsType] = h
}

func (s Sources) Get(dsType virtv2.DataSourceType) (Handler, bool) {
	source, ok := s.sources[dsType]
	return source, ok
}

func (s Sources) Changed(_ context.Context, vd *virtv2.VirtualDisk) bool {
	if vd.Generation == 1 {
		return false
	}

	return vd.Generation != vd.Status.ObservedGeneration
}

func (s Sources) CleanUp(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	var requeue bool

	for _, source := range s.sources {
		ok, err := source.CleanUp(ctx, vd)
		if err != nil {
			return false, err
		}

		requeue = requeue || ok
	}

	return requeue, nil
}

type Cleaner interface {
	CleanUpSupplements(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error)
}

func CleanUpSupplements(ctx context.Context, vd *virtv2.VirtualDisk, c Cleaner) (bool, error) {
	if cc.ShouldCleanupSubResources(vd) {
		return c.CleanUpSupplements(ctx, vd)
	}

	return false, nil
}

func isDiskProvisioningFinished(c metav1.Condition) bool {
	return c.Reason == vdcondition.ReadyReason_Ready || c.Reason == vdcondition.ReadyReason_Lost
}
