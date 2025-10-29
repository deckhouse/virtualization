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
	"fmt"
	"log/slog"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// Nothing - use it to do nothing with virtual machines compared other operations.
// Maybe you want 10% of virtual machines to be updated, 10% deleted, and 80% to continue working as usual.
type Nothing struct {
	logger *slog.Logger
}

func NewNothing(log *slog.Logger) *Nothing {
	return &Nothing{
		logger: log.With("type", "nothing"),
	}
}

func (s *Nothing) Do(_ context.Context, vm v1alpha2.VirtualMachine) {
	s.logger.With("node", vm.Status.Node).Debug(fmt.Sprintf("Nothing: %s", vm.Name))
}
