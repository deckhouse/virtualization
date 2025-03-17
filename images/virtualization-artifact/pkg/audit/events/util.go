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

package events

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/client-go/tools/cache"
)

// removeAllQueryParams removes all query parameters from the given URI.
//
// @param uri The URI string from which query parameters need to be removed.
//
// @return A string representing the URI without query parameters, or an error if the URI parsing fails.
func removeAllQueryParams(uri string) (string, error) {
	parsedURL, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("failed to parse URI: %w", err)
	}

	parsedURL.RawQuery = ""

	return parsedURL.String(), nil
}

func getVMFromInformer(vmInformer cache.Indexer, event *audit.Event) (*v1alpha2.VirtualMachine, error) {
	vmObj, exist, err := vmInformer.GetByKey(event.ObjectRef.Namespace + "/" + event.ObjectRef.Name)
	if err != nil {
		return nil, fmt.Errorf("fail to get node from informer: %w", err)
	}
	if !exist {
		return nil, errors.New("vmObj not exist")
	}

	vm, ok := vmObj.(*v1alpha2.VirtualMachine)
	if !ok {
		return nil, errors.New("fail to convert vmObj to vm")
	}

	return vm, nil
}

func fillVDInfo(vdInformer cache.Indexer, response map[string]string, vm *v1alpha2.VirtualMachine) error {
	storageClasses := []string{}

	for _, bd := range vm.Spec.BlockDeviceRefs {
		if bd.Kind != v1alpha2.VirtualDiskKind {
			continue
		}

		vdObj, exist, err := vdInformer.GetByKey(vm.Namespace + "/" + bd.Name)
		if err != nil {
			return fmt.Errorf("fail to get virtual disk from informer: %w", err)
		}
		if !exist {
			continue
		}

		vd, ok := vdObj.(*v1alpha2.VirtualDisk)
		if !ok {
			return errors.New("fail to convert vdObj to vd")
		}

		storageClasses = append(storageClasses, vd.Status.StorageClassName)
	}

	if len(storageClasses) != 0 {
		response["storageclasses"] = strings.Join(slices.Compact(storageClasses), ",")
	}

	return nil
}

func fillNodeInfo(nodeInformer cache.Indexer, response map[string]string, vm *v1alpha2.VirtualMachine) error {
	nodeObj, exist, err := nodeInformer.GetByKey(vm.Status.Node)
	if err != nil {
		return fmt.Errorf("fail to get node from informer: %w", err)
	}
	if !exist {
		return nil
	}
	node, ok := nodeObj.(*corev1.Node)
	if !ok {
		return errors.New("fail to convert nodeObj to node")
	}

	addresses := []string{}
	for _, addr := range node.Status.Addresses {
		if addr.Type != corev1.NodeHostName && addr.Address != "" {
			addresses = append(addresses, addr.Address)
		}
	}

	if len(addresses) != 0 {
		response["node-network-address"] = strings.Join(slices.Compact(addresses), ",")
	}

	return nil
}
