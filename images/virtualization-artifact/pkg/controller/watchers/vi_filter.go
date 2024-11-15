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

package watchers

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/deckhouse/deckhouse/pkg/log"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualImageFilter struct {
	logger *log.Logger
}

func NewVirtualImageFilter() *VirtualImageFilter {
	return &VirtualImageFilter{
		logger: log.Default().With("filter", "vi"),
	}
}

func (f VirtualImageFilter) FilterUpdateEvents(e event.UpdateEvent) bool {
	oldVI, ok := e.ObjectOld.(*virtv2.VirtualImage)
	if !ok {
		f.logger.Error(fmt.Sprintf("expected an old VirtualImage but got a %T", e.ObjectOld))
		return false
	}

	newVI, ok := e.ObjectNew.(*virtv2.VirtualImage)
	if !ok {
		f.logger.Error(fmt.Sprintf("expected a new VirtualImage but got a %T", e.ObjectNew))
		return false
	}

	if newVI.Generation != newVI.Status.ObservedGeneration {
		return false
	}

	return oldVI.Status.Phase != newVI.Status.Phase
}
