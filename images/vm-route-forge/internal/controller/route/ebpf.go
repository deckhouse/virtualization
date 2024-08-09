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

package route

import (
	"context"
	"fmt"
	"net"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"

	vmipcache "vm-route-forge/internal/cache"
	"vm-route-forge/internal/netlinkwrap"
)

func NewEbpfWatcher(cidrs []*net.IPNet, cache vmipcache.Cache, nlWrapper *netlinkwrap.Funcs, log logr.Logger) (*EbpfWatcher, error) {
	return &EbpfWatcher{}, fmt.Errorf("not implemented")
}

type EbpfWatcher struct {
}

func (w *EbpfWatcher) Watch(ctx context.Context) (<-chan types.NamespacedName, error) {
	return nil, nil
}
