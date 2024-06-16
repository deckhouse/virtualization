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

package datasource

import virtv2alpha1 "github.com/deckhouse/virtualization/api/core/v1alpha2"

func ShouldCopyImagePullSecret(ctrImg *virtv2alpha1.DataSourceContainerRegistry, targetNS string) bool {
	if ctrImg == nil || ctrImg.ImagePullSecret.Name == "" {
		return false
	}

	imgPullNS := ctrImg.ImagePullSecret.Namespace

	// Should copy imagePullSecret if namespace differs from the specified namespace.
	return imgPullNS != "" && imgPullNS != targetNS
}
