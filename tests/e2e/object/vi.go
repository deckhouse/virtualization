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

package object

import (
	"github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewHTTPVIUbuntu(name string) *virtv2.VirtualImage {
	return vi.New(
		vi.WithName(name),
		vi.WithDataSourceHTTP(
			UbuntuHTTP,
			nil,
			nil,
		),
	)
}

func NewGeneratedHTTPVIUbuntu(prefix string) *virtv2.VirtualImage {
	return vi.New(
		vi.WithGenerateName(prefix),
		vi.WithDataSourceHTTP(
			UbuntuHTTP,
			nil,
			nil,
		),
		vi.WithStorage(virtv2.StorageContainerRegistry),
	)
}
