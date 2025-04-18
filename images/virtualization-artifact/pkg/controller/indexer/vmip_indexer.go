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

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func IndexVMIPByVM() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &virtv2.VirtualMachineIPAddress{}, IndexFieldVMIPByVM, func(object client.Object) []string {
		vmip, ok := object.(*virtv2.VirtualMachineIPAddress)
		if !ok || vmip == nil {
			return nil
		}
		return []string{vmip.Status.VirtualMachine}
	}
}

func IndexVMIPByAddress() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &virtv2.VirtualMachineIPAddress{}, IndexFieldVMIPByAddress, func(object client.Object) []string {
		vmip, ok := object.(*virtv2.VirtualMachineIPAddress)
		if !ok || vmip == nil {
			return nil
		}

		var addresses []string
		if vmip.Spec.StaticIP != "" {
			addresses = append(addresses, vmip.Spec.StaticIP)
		}

		if vmip.Status.Address != "" {
			addresses = append(addresses, vmip.Status.Address)
		}

		return addresses
	}
}
