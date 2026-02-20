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

package plugin

import (
	"context"
	"log/slog"

	"github.com/containerd/nri/pkg/api"
)

// Synchronize is called by the NRI to synchronize the state of the driver during bootstrap.
func (d *Driver) Synchronize(ctx context.Context, pods []*api.PodSandbox, containers []*api.Container) ([]*api.ContainerUpdate, error) {
	d.log.Info("Synchronizing state with the runtime...", slog.Int("pods", len(pods)), slog.Int("containers", len(containers)))
	return d.allocator.Synchronize(ctx, pods, containers)
}
