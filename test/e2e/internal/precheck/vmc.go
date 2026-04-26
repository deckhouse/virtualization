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

package precheck

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const (
	vmcModuleCheckEnvName = "VMC_PRECHECK"
	defaultVMClassName    = "generic-for-e2e"

	vmClassVersion = "v1alpha3"
)

// vmcPrecheck implements Precheck interface for VMC/VMClass.
// This is a common precheck that runs for all tests.
type vmcPrecheck struct{}

func (c *vmcPrecheck) Label() string {
	return PrecheckVMC
}

func (c *vmcPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(vmcModuleCheckEnvName) {
		GinkgoWriter.Write([]byte("VMC precheck is disabled.\n"))
		return nil
	}

	// Use DynamicClient with v1alpha3 to avoid deprecation warning
	gvr := schema.GroupVersionResource{
		Group:    "virtualization.deckhouse.io",
		Version:  vmClassVersion,
		Resource: "virtualmachineclasses",
	}

	vmClasses, err := f.DynamicClient().Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: list VirtualMachineClasses: %w", vmcModuleCheckEnvName, err)
	}

	var e2eClass map[string]interface{}
	var defaultClass map[string]interface{}

	for i := range vmClasses.Items {
		vmClass := vmClasses.Items[i].Object
		name, ok := vmClass["metadata"].(map[string]interface{})["name"].(string)
		if !ok {
			continue
		}

		if name == defaultVMClassName {
			e2eClass = vmClass
		}

		// Check for default annotation
		metadata, ok := vmClass["metadata"].(map[string]interface{})
		if !ok {
			continue
		}
		annotations, ok := metadata["annotations"].(map[string]interface{})
		if !ok {
			continue
		}
		if _, ok := annotations["virtualmachineclass.virtualization.deckhouse.io/is-default-class"]; ok {
			defaultClass = vmClass
		}
	}

	// Helper to get name from vmClass
	getVMClassName := func(m map[string]interface{}) string {
		if m == nil {
			return ""
		}
		metadata, ok := m["metadata"].(map[string]interface{})
		if !ok {
			return ""
		}
		name, _ := metadata["name"].(string)
		return name
	}

	// Check if default VMClass exists and is correct
	switch {
	case e2eClass != nil && defaultClass != nil && getVMClassName(defaultClass) == defaultVMClassName:
		// OK
	case e2eClass != nil && defaultClass != nil:
		return fmt.Errorf("%s=no to disable this precheck: cluster has wrong default class %q, e2e tests require %q to be default",
			vmcModuleCheckEnvName, getVMClassName(defaultClass), defaultVMClassName)
	case e2eClass == nil && defaultClass != nil:
		return fmt.Errorf("%s=no to disable this precheck: cluster has wrong default class %q, e2e tests require %q to be default",
			vmcModuleCheckEnvName, getVMClassName(defaultClass), defaultVMClassName)
	case e2eClass != nil && defaultClass == nil:
		return fmt.Errorf("%s=no to disable this precheck: cluster has no default class, e2e tests require %q to be default. Run: kubectl annotate vmclass/%s virtualmachineclass.virtualization.deckhouse.io/is-default-class=true",
			vmcModuleCheckEnvName, defaultVMClassName, defaultVMClassName)
	case e2eClass == nil && defaultClass == nil:
		return fmt.Errorf("%s=no to disable this precheck: cluster has no default class and no %q class. Run: kubectl get vmclass/generic -o json | jq 'del(.status) | .metadata.annotations = {\"virtualmachineclass.virtualization.deckhouse.io/is-default-class\":\"true\"}' | kubectl create -f -",
			vmcModuleCheckEnvName, defaultVMClassName)
	}

	return nil
}

// Register VMC precheck as common (runs for all tests).
func init() {
	RegisterPrecheck(&vmcPrecheck{}, true)
}
