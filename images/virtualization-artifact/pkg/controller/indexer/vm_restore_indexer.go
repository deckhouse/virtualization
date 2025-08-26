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

package indexer

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func IndexVMRestoreByVMSnapshot() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &v1alpha2.VirtualMachineRestore{}, IndexFieldVMRestoreByVMSnapshot, func(object client.Object) []string {
		vmRestore, ok := object.(*v1alpha2.VirtualMachineRestore)
		if !ok || vmRestore == nil {
			return nil
		}

		return []string{vmRestore.Spec.VirtualMachineSnapshotName}
	}
}
