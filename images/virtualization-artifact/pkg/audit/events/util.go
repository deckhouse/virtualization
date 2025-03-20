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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	virtv1 "kubevirt.io/api/core/v1"

	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
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

func getVMFromInformer(cache ttlCache, vmInformer indexer, vmName string) (*v1alpha2.VirtualMachine, error) {
	vmObj, exist, err := vmInformer.GetByKey(vmName)
	if err != nil {
		return nil, fmt.Errorf("fail to get node from informer: %w", err)
	}
	if !exist {
		vmObj, exist = cache.Get("virtualmachines/" + vmName)
		if !exist {
			return nil, errors.New("vmObj not exist")
		}
	}

	vm, ok := vmObj.(*v1alpha2.VirtualMachine)
	if !ok {
		return nil, errors.New("fail to convert vmObj to vm")
	}

	return vm, nil
}

func getVDFromInformer(cache ttlCache, vdInformer indexer, vdName string) (*v1alpha2.VirtualDisk, error) {
	vdObj, exist, err := vdInformer.GetByKey(vdName)
	if err != nil {
		return nil, fmt.Errorf("fail to get node from informer: %w", err)
	}
	if !exist {
		vdObj, exist = cache.Get("virtualdisks/" + vdName)
		if !exist {
			return nil, errors.New("vdObj not exist")
		}
	}

	vd, ok := vdObj.(*v1alpha2.VirtualDisk)
	if !ok {
		return nil, errors.New("fail to convert vdObj to vd")
	}

	return vd, nil
}

func getNodeFromInformer(nodeInformer indexer, nodeName string) (*corev1.Node, error) {
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

func getPodFromInformer(cache ttlCache, podInformer indexer, podName string) (*corev1.Pod, error) {
	podObj, exist, err := podInformer.GetByKey(podName)
	if err != nil {
		return nil, fmt.Errorf("fail to get pod from informer: %w", err)
	}
	if !exist {
		podObj, exist = cache.Get("pods/" + podName)
		if !exist {
			return nil, errors.New("podObj not exist")
		}
	}

	pod, ok := podObj.(*corev1.Pod)
	if !ok {
		return nil, errors.New("fail to convert podObj to pod")
	}

	return pod, nil
}

func getVMOPFromInformer(vmopInformer indexer, vmopName string) (*v1alpha2.VirtualMachineOperation, error) {
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

func getInternalVMIFromInformer(internalVMIInformer indexer, internalVMIName string) (*virtv1.VirtualMachineInstance, error) {
	vmopObj, exist, err := internalVMIInformer.GetByKey(internalVMIName)
	if err != nil {
		return nil, fmt.Errorf("fail to get intVMI from informer: %w", err)
	}
	if !exist {
		return nil, errors.New("intVMI not exist")
	}

	vm, ok := vmopObj.(*virtv1.VirtualMachineInstance)
	if !ok {
		return nil, errors.New("fail to convert intVMIObj to vmop")
	}

	return vm, nil
}

func getModuleFromInformer(moduleInformer indexer, moduleName string) (*module, error) {
	moduleObj, exist, err := moduleInformer.GetByKey(moduleName)
	if err != nil {
		return nil, fmt.Errorf("fail to get module from informer: %w", err)
	}
	if !exist {
		return nil, errors.New("module not exist")
	}

	unstructuredObj, ok := moduleObj.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf("moduleObj is not of type *unstructured.Unstructured")
	}

	module := &module{}
	err = unstructuredToTypedObject(unstructuredObj, module)
	if err != nil {
		return nil, fmt.Errorf("failed to convert unstructuredObj to Module: %w", err)
	}

	return module, nil
}

func getModuleConfigFromInformer(moduleConfigInformer indexer, moduleConfigName string) (*mcapi.ModuleConfig, error) {
	mcObj, exist, err := moduleConfigInformer.GetByKey(moduleConfigName)
	if err != nil {
		return nil, fmt.Errorf("fail to get module config from informer: %w", err)
	}
	if !exist {
		return nil, errors.New("module config not exist")
	}

	unstructuredObj, ok := mcObj.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf("mcObj is not of type *unstructured.Unstructured")
	}

	moduleConfig := &mcapi.ModuleConfig{}
	err = unstructuredToTypedObject(unstructuredObj, moduleConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to convert unstructuredObj to ModuleConfig: %w", err)
	}

	return moduleConfig, nil
}

func unstructuredToTypedObject(unstructuredObj *unstructured.Unstructured, obj runtime.Object) error {
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, obj)
	if err != nil {
		return fmt.Errorf("failed to convert map to typed object: %w", err)
	}

	return nil
}
