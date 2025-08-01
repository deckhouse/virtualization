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

func IndexVMMACByVM() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &virtv2.VirtualMachineMACAddress{}, IndexFieldVMMACByVM, func(object client.Object) []string {
		vmmac, ok := object.(*virtv2.VirtualMachineMACAddress)
		if !ok || vmmac == nil {
			return nil
		}

		var vmNames []string
		if vmmac.Status.VirtualMachine != "" {
			vmNames = append(vmNames, vmmac.Status.VirtualMachine)
		}

		for _, ownerRef := range vmmac.OwnerReferences {
			if ownerRef.Kind != virtv2.VirtualMachineKind {
				continue
			}

			if ownerRef.Name == "" || ownerRef.Name == vmmac.Status.VirtualMachine {
				continue
			}

			vmNames = append(vmNames, ownerRef.Name)
		}

		return vmNames
	}
}

func IndexVMMACByAddress() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &virtv2.VirtualMachineMACAddress{}, IndexFieldVMMACByAddress, func(object client.Object) []string {
		vmmac, ok := object.(*virtv2.VirtualMachineMACAddress)
		if !ok || vmmac == nil {
			return nil
		}

		var addresses []string
		if vmmac.Spec.Address != "" {
			addresses = append(addresses, vmmac.Spec.Address)
		}

		if vmmac.Status.Address != "" {
			addresses = append(addresses, vmmac.Status.Address)
		}

		return addresses
	}
}
