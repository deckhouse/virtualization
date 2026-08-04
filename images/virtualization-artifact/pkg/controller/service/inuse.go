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

package service

import (
	"fmt"
	"strings"
)

// InUseByVirtualMachinesMessage builds the message reported on the InUse condition when
// a disk or an image is held by one or more VirtualMachines. It names the VirtualMachines
// and how to release the resource: this is also the answer to "why is my resource stuck in
// Terminating", because a resource in use is protected from deletion by a finalizer.
func InUseByVirtualMachinesMessage(resourceKind string, vmNames []string) string {
	// The InUse condition is set from a live VirtualMachine list, so an empty list normally
	// means the resource is not in use at all. Keep a message for the race where the caller
	// knows it is used but the names are already gone.
	if len(vmNames) == 0 {
		return truncateMessage(fmt.Sprintf("The %s is in use by a VirtualMachine.", resourceKind))
	}

	if len(vmNames) == 1 {
		return truncateMessage(fmt.Sprintf("The %s is in use by the VirtualMachine %q; detach it or stop the VirtualMachine to release the %s.",
			resourceKind, vmNames[0], resourceKind))
	}

	quoted := make([]string, 0, len(vmNames))
	for _, name := range vmNames {
		quoted = append(quoted, fmt.Sprintf("%q", name))
	}

	// The list of names goes last: it is the only unbounded part of the message, so a
	// message truncated to maxConditionMessageLength still states the count and the way
	// to release the resource.
	return truncateMessage(fmt.Sprintf("The %s is in use by %d VirtualMachines; detach it or stop them to release the %s. In use by: %s.",
		resourceKind, len(vmNames), resourceKind, strings.Join(quoted, ", ")))
}
