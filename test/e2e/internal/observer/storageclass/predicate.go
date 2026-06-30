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

package storageclass

import storagev1 "k8s.io/api/storage/v1"

// BeAvailable reports that the StorageClass exists and has not been deleted.
func BeAvailable() Predicate {
	return func(sc *storagev1.StorageClass) (bool, error) {
		return sc.UID != "" && sc.DeletionTimestamp == nil, nil
	}
}
