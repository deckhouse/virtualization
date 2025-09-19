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

package resources

import (
	"fmt"

	cvibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/cvi"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewCVI(name, url string) *v1alpha2.ClusterVirtualImage {
	return cvibuilder.New(
		cvibuilder.WithGenerateName(fmt.Sprintf("%s-", name)),
		cvibuilder.WithDataSourceHTTPWithOnlyURL(url),
	)
}
