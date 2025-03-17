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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
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

func getVMFromInformer(vmInformer cache.Indexer, vmName string) (*v1alpha2.VirtualMachine, error) {
	vmObj, exist, err := vmInformer.GetByKey(vmName)
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

func getVDFromInformer(vdInformer cache.Indexer, vdName string) (*v1alpha2.VirtualDisk, error) {
	vdObj, exist, err := vdInformer.GetByKey(vdName)
	if err != nil {
		return nil, fmt.Errorf("fail to get node from informer: %w", err)
	}
	if !exist {
		return nil, errors.New("vdObj not exist")
	}

	vd, ok := vdObj.(*v1alpha2.VirtualDisk)
	if !ok {
		return nil, errors.New("fail to convert vdObj to vd")
	}

	return vd, nil
}

func getNodeFromInformer(nodeInformer cache.Indexer, nodeName string) (*corev1.Node, error) {
	nodeObj, exist, err := nodeInformer.GetByKey(nodeName)
	if err != nil {
		return nil, fmt.Errorf("fail to get node from informer: %w", err)
	}
	if !exist {
		return nil, errors.New("nodeObj not exist")
	}

	node, ok := nodeObj.(*corev1.Node)
	if !ok {
		return nil, errors.New("fail to convert nodeObj to node")
	}

	return node, nil
}

func getPodFromInformer(podInformer cache.Indexer, podName string) (*corev1.Pod, error) {
	podObj, exist, err := podInformer.GetByKey(podName)
	if err != nil {
		return nil, fmt.Errorf("fail to get pod from informer: %w", err)
	}
	if !exist {
		return nil, errors.New("podObj not exist")
	}

	pod, ok := podObj.(*corev1.Pod)
	if !ok {
		return nil, errors.New("fail to convert podObj to pod")
	}

	return pod, nil
}

func getVMOPFromInformer(vmopInformer cache.Indexer, vmopName string) (*v1alpha2.VirtualMachineOperation, error) {
	vmopObj, exist, err := vmopInformer.GetByKey(vmopName)
	if err != nil {
		return nil, fmt.Errorf("fail to get vmop from informer: %w", err)
	}
	if !exist {
		return nil, errors.New("vmopObj not exist")
	}

	vmop, ok := vmopObj.(*v1alpha2.VirtualMachineOperation)
	if !ok {
		return nil, errors.New("fail to convert vmopObj to vmop")
	}

	return vmop, nil
}

func fillVDInfo(vdInformer cache.Indexer, response *EventLog, vm *v1alpha2.VirtualMachine) error {
	storageClasses := []string{}

	for _, bd := range vm.Spec.BlockDeviceRefs {
		if bd.Kind != v1alpha2.VirtualDiskKind {
			continue
		}

		vd, err := getVDFromInformer(vdInformer, vm.Namespace+"/"+bd.Name)
		if err != nil {
			return fmt.Errorf("fail to get virtual disk from informer: %w", err)
		}

		storageClasses = append(storageClasses, vd.Status.StorageClassName)
	}

	if len(storageClasses) != 0 {
		response.StorageClasses = strings.Join(slices.Compact(storageClasses), ",")
	}

	return nil
}

func fillNodeInfo(nodeInformer cache.Indexer, response *EventLog, vm *v1alpha2.VirtualMachine) error {
	node, err := getNodeFromInformer(nodeInformer, vm.Status.Node)
	if err != nil {
		return fmt.Errorf("fail to get node from informer: %w", err)
	}

	addresses := []string{}
	for _, addr := range node.Status.Addresses {
		if addr.Type != corev1.NodeHostName && addr.Address != "" {
			addresses = append(addresses, addr.Address)
		}
	}

	if len(addresses) != 0 {
		response.NodeNetworkAddress = strings.Join(slices.Compact(addresses), ",")
	}

	return nil
}
