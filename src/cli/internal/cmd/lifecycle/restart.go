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

package lifecycle

import (
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/deckhouse/virtualization/src/cli/internal/templates"
)

func NewRestartCommand(clientConfig clientcmd.ClientConfig) *cobra.Command {
	lifecycle := NewLifecycle(Restart, clientConfig)
	cmd := &cobra.Command{
		Use:     "restart (VirtualMachine)",
		Short:   "Restart a virtual machine.",
		Example: lifecycle.Usage(),
		Args:    templates.ExactArgs("restart", 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return lifecycle.Run(args)
		},
	}
	AddCommandlineArgs(cmd.Flags(), &lifecycle.opts)
	cmd.SetUsageTemplate(templates.UsageTemplate())
	return cmd
}
