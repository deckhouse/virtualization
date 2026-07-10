/*
Copyright 2026 Flant JSC

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

package storageprofile

import (
	"fmt"

	storagev1alpha1 "github.com/deckhouse/virtualization-controller/pkg/apis/storage/v1alpha1"
)

// BeReady reports that the StorageProfile is populated for storageClassName.
func BeReady(storageClassName string) Predicate {
	return func(sp *storagev1alpha1.StorageProfile) (bool, error) {
		if sp == nil {
			return false, nil
		}
		if sp.Status.StorageClass == nil {
			return false, nil
		}
		if *sp.Status.StorageClass != storageClassName {
			return false, fmt.Errorf(
				"StorageProfile %q references StorageClass %q, expected %q",
				sp.Name, *sp.Status.StorageClass, storageClassName,
			)
		}
		if sp.Status.Provisioner == nil {
			return false, nil
		}
		return true, nil
	}
}
