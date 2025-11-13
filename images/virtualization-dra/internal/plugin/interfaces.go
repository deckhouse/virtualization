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

	"github.com/containerd/nri/pkg/api"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/dynamic-resource-allocation/resourceslice"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"
)

// Allocator is the backend that Driver calls. Implement this to support a resource type (e.g. USB); Driver handles kubelet protocol and ResourceSlice publishing.
type Allocator interface {
	UpdateChannel() chan resourceslice.DriverResources
	Prepare(ctx context.Context, claim *resourcev1.ResourceClaim) ([]*drapbv1.Device, error)
	Unprepare(ctx context.Context, claimUID types.UID) error
	Synchronize(ctx context.Context, pods []*api.PodSandbox, containers []*api.Container) ([]*api.ContainerUpdate, error)
}
