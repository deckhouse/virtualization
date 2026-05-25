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
	"strings"

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
		_, _ = GinkgoWriter.Write([]byte("VMC precheck is disabled.\n"))
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

	if e2eClass != nil && defaultClass != nil && getVMClassName(defaultClass) == defaultVMClassName {
		// Everything is OK: correct default VMClass is present in the cluster.
		return nil
	}

	issues := make([]string, 0)
	cmds := make([]string, 0)

	if defaultClass != nil {
		issues = append(issues, fmt.Sprintf("cluster has wrong default vmclass %q", getVMClassName(defaultClass)))
		cmds = append(cmds, cmdRemoveDefaultAnnotation(getVMClassName(defaultClass)))
	} else {
		issues = append(issues, "cluster has no default vmclass")
	}

	if e2eClass != nil {
		issues = append(issues, fmt.Sprintf("e2e tests require vmclass %q to be default", defaultVMClassName))
		cmds = append(cmds, cmdSetDefaultAnnotation(defaultVMClassName))
	} else {
		issues = append(issues, fmt.Sprintf("e2e tests require vmclass %q to present and be default", defaultVMClassName))
		cmds = append(cmds, cmdCopyGenericAsDefaultClass(defaultVMClassName))
	}

	return fmt.Errorf("%s=no to disable this precheck: %s. Run command to fix cluster: %s",
		vmcModuleCheckEnvName,
		strings.Join(issues, "; "),
		strings.Join(cmds, " && "),
	)
}

func cmdCopyGenericAsDefaultClass(targetVMClassName string) string {
	return fmt.Sprintf(`kubectl get vmclass/generic -o json | jq 'del(.status) | del(.metadata) | .metadata = {"name":"%s","annotations":{"virtualmachineclass.virtualization.deckhouse.io/is-default-class":"true"}} ' | kubectl create -f -`, targetVMClassName)
}

func cmdRemoveDefaultAnnotation(targetVMClassName string) string {
	return fmt.Sprintf(`kubectl annotate vmclass/%s virtualmachineclass.virtualization.deckhouse.io/is-default-class=-`, targetVMClassName)
}

func cmdSetDefaultAnnotation(targetVMClassName string) string {
	return fmt.Sprintf(`kubectl annotate vmclass/%s virtualmachineclass.virtualization.deckhouse.io/is-default-class=true`, targetVMClassName)
}

// Register VMC precheck as common (runs for all tests).
func init() {
	RegisterPrecheck(&vmcPrecheck{}, true)
}
