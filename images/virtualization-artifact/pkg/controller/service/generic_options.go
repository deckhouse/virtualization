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

package service

import "github.com/deckhouse/virtualization-controller/pkg/common/provisioner"

type genericOptions struct {
	nodePlacement *provisioner.NodePlacement
}

func newGenericOptions(opts ...Option) *genericOptions {
	o := &genericOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

type Option func(o *genericOptions)

func WithNodePlacement(nodePlacement *provisioner.NodePlacement) Option {
	return func(o *genericOptions) {
		o.nodePlacement = nodePlacement
	}
}

func WithSystemNodeToleration() Option {
	return func(o *genericOptions) {
		if o.nodePlacement == nil {
			o.nodePlacement = &provisioner.NodePlacement{}
		}
		provisioner.AddTolerationForSystemNodes(o.nodePlacement)
	}
}
