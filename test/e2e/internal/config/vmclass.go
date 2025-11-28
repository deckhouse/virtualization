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

package config

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	virtclient "github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const DefaultVirtualMachineClassName = "generic-for-e2e"

func CheckDefaultVMClass(virtClient virtclient.Client) error {
	vmClasses, err := virtClient.VirtualMachineClasses().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list VirtualMachineClasses in cluster: %w", err)
	}

	var e2eClass *v1alpha2.VirtualMachineClass
	var currentDefaultClass *v1alpha2.VirtualMachineClass
	for _, vmClass := range vmClasses.Items {
		if vmClass.Name == DefaultVirtualMachineClassName {
			e2eClass = &vmClass
		}
		if _, ok := vmClass.Annotations[annotations.AnnVirtualMachineClassDefault]; ok {
			currentDefaultClass = &vmClass
		}
	}

	// Expect that VMClass for e2e is also a default VMClass.
	if e2eClass != nil && currentDefaultClass != nil && currentDefaultClass.Name == DefaultVirtualMachineClassName {
		return nil
	}

	// Handle other cases.
	switch {
	case e2eClass != nil && currentDefaultClass != nil:
		return fmt.Errorf("cluster has wrong default class %s, e2e tests requires %s class to be default, run these commands to fix this issue: %s ; %s",
			currentDefaultClass.Name,
			DefaultVirtualMachineClassName,
			cmdRemoveDefaultClassAnnotation(currentDefaultClass.Name),
			cmdSetDefaultClassAnnotation(DefaultVirtualMachineClassName),
		)
	case e2eClass == nil && currentDefaultClass != nil:
		return fmt.Errorf("cluster has wrong default class %s, e2e tests requires %s class to be default, run these commands to fix this issue: %s ; %s",
			currentDefaultClass.Name,
			DefaultVirtualMachineClassName,
			cmdRemoveDefaultClassAnnotation(currentDefaultClass.Name),
			cmdCopyGenericAsDefaultClass(),
		)
	case e2eClass != nil && currentDefaultClass == nil:
		return fmt.Errorf("cluster has no default class, e2e tests requires %s class to be default, run this command to fix this issue: %s",
			DefaultVirtualMachineClassName,
			cmdSetDefaultClassAnnotation(DefaultVirtualMachineClassName),
		)
	case e2eClass == nil && currentDefaultClass == nil:
		return fmt.Errorf("cluster has no default class, e2e tests requires %s class to be default, run this command to fix this issue: %s",
			DefaultVirtualMachineClassName,
			cmdCopyGenericAsDefaultClass(),
		)
	}

	return nil
}

func cmdCopyGenericAsDefaultClass() string {
	return fmt.Sprintf(`kubectl get vmclass/generic -o json | jq 'del(.status) | del(.metadata) | .metadata = {"name":"%s","annotations":{"virtualmachineclass.virtualization.deckhouse.io/is-default-class":"true"}} ' | kubectl create -f -`, DefaultVirtualMachineClassName)
}

func cmdRemoveDefaultClassAnnotation(className string) string {
	return fmt.Sprintf(`kubectl annotate vmclass/%s virtualmachineclass.virtualization.deckhouse.io/is-default-class-`, className)
}

func cmdSetDefaultClassAnnotation(className string) string {
	return fmt.Sprintf(`kubectl annotate vmclass/%s virtualmachineclass.virtualization.deckhouse.io/is-default-class=true`, className)
}
