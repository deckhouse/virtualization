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

package main

import (
	"context"
	"strings"

	"k8s.io/client-go/rest"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// vmipDeprecationPrefixes start the deprecationWarning of the VirtualMachineIPAddress
// and VirtualMachineIPAddressLease CRDs and the vmipDeprecationWarning of the VM
// validators: a change of those texts has to come here too.
var vmipDeprecationPrefixes = []string{
	"VirtualMachineIPAddress is deprecated",
	"VirtualMachineIPAddressLease is deprecated",
	"spec.virtualMachineIPAddressName is deprecated",
}

// withoutVMIPDeprecationWarnings drops the vmip deprecation warnings: the controller
// still serves vmip, and every request would log them.
func withoutVMIPDeprecationWarnings() rest.WarningHandlerWithContext {
	return vmipDeprecationFilter{next: ctrllog.NewKubeAPIWarningLogger(ctrllog.KubeAPIWarningLoggerOptions{})}
}

type vmipDeprecationFilter struct {
	next rest.WarningHandlerWithContext
}

func (f vmipDeprecationFilter) HandleWarningHeaderWithContext(ctx context.Context, code int, agent, text string) {
	for _, prefix := range vmipDeprecationPrefixes {
		if strings.HasPrefix(text, prefix) {
			return
		}
	}
	f.next.HandleWarningHeaderWithContext(ctx, code, agent, text)
}
